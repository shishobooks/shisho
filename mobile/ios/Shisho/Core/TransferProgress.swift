import Foundation
import os

/// Reports how far one transfer in front of the user has got, from 0 to 1,
/// as opening a book that isn't downloaded shows it. The transfer's bytes
/// are read a few times a second, and only a change of a whole percent is
/// reported, so a screen showing it doesn't redraw for every packet. As the
/// transfer's own delegate, it applies the session's redirect policy too.
final class TransferProgress: NSObject, URLSessionTaskDelegate, Sendable {
    private let report: @Sendable (Double) -> Void
    private let polling = OSAllocatedUnfairLock<Task<Void, Never>?>(initialState: nil)

    init(report: @escaping @Sendable (Double) -> Void) {
        self.report = report
    }

    /// The fraction to report for bytes received of expected, or nil when
    /// the size isn't known or it hasn't moved a whole percent since last.
    static func fraction(received: Int64, expected: Int64, last: Double?) -> Double? {
        guard expected > 0 else { return nil }
        let fraction = min(1, Double(received) / Double(expected))
        guard let last else { return fraction }
        return fraction - last >= 0.01 || (fraction == 1 && last < 1) ? fraction : nil
    }

    func urlSession(_ session: URLSession, didCreateTask task: URLSessionTask) {
        let report = report
        let poll = Task {
            var last: Double?
            while !Task.isCancelled {
                if let fraction = Self.fraction(received: task.countOfBytesReceived, expected: task.countOfBytesExpectedToReceive,
                                                last: last) {
                    last = fraction
                    report(fraction)
                }
                try? await Task.sleep(for: .milliseconds(200))
            }
        }
        polling.withLock {
            $0?.cancel()
            $0 = poll
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest) async -> URLRequest? {
        guard let current = task.currentRequest else { return nil }
        return RedirectPolicy.follow(request, from: current)
    }

    /// Stops reading the transfer's bytes, once it's over.
    func stop() {
        polling.withLock {
            $0?.cancel()
            $0 = nil
        }
    }
}
