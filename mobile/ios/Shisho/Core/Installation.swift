import Foundation
import UIKit

/// What this installation tells a Shisho server about itself.
enum Installation {
    /// The Device surface protocol this app speaks (ADR 0020).
    static let appProtocol = 1

    /// The first Shisho release whose Device surface speaks `appProtocol`,
    /// named when a server is too old.
    static let minimumServerVersion = "0.10.0"

    /// Where to update the app. A placeholder until the app is in the store.
    static let updateURL = URL(string: "https://www.shishobooks.com/docs/shisho-app")!

    private static let installationIDKey = "installationID"

    /// The installation's own id, made the first time it's needed and kept
    /// across signing out and back in. It goes when the app is deleted.
    static var id: String {
        let defaults = UserDefaults.standard
        if let existing = defaults.string(forKey: installationIDKey) {
            return existing
        }
        let made = UUID().uuidString.lowercased()
        defaults.set(made, forKey: installationIDKey)
        return made
    }

    /// The app's version and build, such as "0.1.0 (1)".
    static var appVersion: String {
        let info = Bundle.main.infoDictionary ?? [:]
        let version = info["CFBundleShortVersionString"] as? String ?? "0"
        let build = info["CFBundleVersion"] as? String ?? "0"
        return "\(version) (\(build))"
    }

    /// The hardware's name for the Device list, such as "iPhone". iOS gives
    /// apps only the model unless they hold an entitlement for the name the
    /// user chose, so people rename the Device on the web if they want.
    @MainActor static var deviceName: String {
        UIDevice.current.name
    }
}
