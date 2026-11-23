import CoreMotion
import UIKit

/// Notices the phone being shaken while a sleep timer counts, so a listener
/// drifting off can extend it without looking. The accelerometer works with
/// the phone locked, since audio keeps the app running; the system's shake
/// gesture covers the screen being on, and the Simulator's Shake command.
@MainActor
final class ShakeDetector {
    /// How hard a jolt counts as a shake, in g, gravity included.
    private static let threshold = 2.3
    /// One shake jolts several times; it counts once in this long.
    private static let quietAfter: TimeInterval = 1.5
    /// Two jolts this close together make a shake.
    private static let joltsWithin: TimeInterval = 0.6

    private let motion = CMMotionManager()
    private var onShake: (@MainActor () -> Void)?
    private var lastShake = Date.distantPast
    private var lastJolt = Date.distantPast
    private var observer: NSObjectProtocol?

    func start(_ onShake: @escaping @MainActor () -> Void) {
        self.onShake = onShake
        if observer == nil {
            observer = NotificationCenter.default.addObserver(forName: UIWindow.shakeNotification, object: nil, queue: .main) { [weak self] _ in
                Task { @MainActor in self?.shook() }
            }
        }
        guard motion.isAccelerometerAvailable, !motion.isAccelerometerActive else { return }
        motion.accelerometerUpdateInterval = 0.1
        ShakeDetector.watch(motion, threshold: ShakeDetector.threshold) { [weak self] in
            Task { @MainActor in self?.jolted() }
        }
    }

    func stop() {
        onShake = nil
        motion.stopAccelerometerUpdates()
        if let observer {
            NotificationCenter.default.removeObserver(observer)
        }
        observer = nil
    }

    /// Lets the listener feel that the shake took.
    func confirm() {
        UINotificationFeedbackGenerator().notificationOccurred(.success)
    }

    private func shook() {
        let now = Date()
        guard let onShake, now.timeIntervalSince(lastShake) > ShakeDetector.quietAfter else { return }
        lastShake = now
        onShake()
    }

    /// A jolt from the accelerometer. One alone, as from tossing the phone
    /// onto the bed, isn't a shake; two close together are.
    private func jolted() {
        let now = Date()
        defer { lastJolt = now }
        if now.timeIntervalSince(lastJolt) < ShakeDetector.joltsWithin {
            shook()
        }
    }

    /// CoreMotion calls back on the queue it's given, so the closure is made
    /// outside the main actor and hops back to it.
    nonisolated private static func watch(_ motion: CMMotionManager, threshold: Double, jolted: @escaping @Sendable () -> Void) {
        motion.startAccelerometerUpdates(to: OperationQueue()) { data, _ in
            guard let a = data?.acceleration else { return }
            if (a.x * a.x + a.y * a.y + a.z * a.z).squareRoot() > threshold {
                jolted()
            }
        }
    }
}

extension UIWindow {
    /// Posted when the system recognizes a shake while the app is on screen.
    static let shakeNotification = Notification.Name("ShishoDeviceShaken")

    override open func motionEnded(_ motion: UIEvent.EventSubtype, with event: UIEvent?) {
        if motion == .motionShake {
            NotificationCenter.default.post(name: UIWindow.shakeNotification, object: nil)
        }
        super.motionEnded(motion, with: event)
    }
}
