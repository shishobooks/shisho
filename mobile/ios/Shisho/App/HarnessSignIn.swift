#if DEBUG
import Foundation

/// The account a debug build signs in to as it launches, when the Maestro
/// harness names one in launch arguments, so a flow that isn't about signing
/// in skips typing on the sign-in screen
/// (`subflows/launch_signed_in.yaml` in the harness's flows). It signs in
/// through the same path as the screen. Release builds leave it out.
struct HarnessSignIn: Equatable {
    let server: String
    let username: String
    let password: String

    /// The account named by `-harnessServer`, `-harnessUsername`, and
    /// `-harnessPassword`, or nil without all three.
    init?(arguments: [String: Any]) {
        guard let server = arguments["harnessServer"] as? String, !server.isEmpty,
              let username = arguments["harnessUsername"] as? String, !username.isEmpty,
              let password = arguments["harnessPassword"] as? String, !password.isEmpty
        else { return nil }
        self.init(server: server, username: username, password: password)
    }

    init(server: String, username: String, password: String) {
        self.server = server
        self.username = username
        self.password = password
    }

    /// The harness can't put the Simulator on cellular, so `-harnessNetwork
    /// cellular` says to treat any network as cellular, to check Wi-Fi only.
    static var cellular: Bool {
        UserDefaults.standard.volatileDomain(forName: UserDefaults.argumentDomain)["harnessNetwork"] as? String == "cellular"
    }

    /// The account this launch's arguments name, never one kept in the
    /// app's settings.
    static var launched: HarnessSignIn? {
        HarnessSignIn(arguments: UserDefaults.standard.volatileDomain(forName: UserDefaults.argumentDomain))
    }
}
#endif
