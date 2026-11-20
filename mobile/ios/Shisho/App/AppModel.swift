import Foundation
import Observation
import ShishoAPI
import UIKit

/// The signed-in account and everything that works on it: its HTTP layer,
/// its copy of the catalog, its sync, and its covers.
final class AccountSession {
    let account: Account
    let client: APIClient
    let store: CatalogStore
    let sync: SyncEngine
    let covers: CoverCache

    init(account: Account, credential: String, onCapabilities: @escaping @Sendable ([String]) -> Void) throws {
        self.account = account
        client = APIClient(baseURL: account.serverURL, credential: credential, onCapabilities: onCapabilities)
        try FileManager.default.createDirectory(at: account.folder, withIntermediateDirectories: true)
        store = try CatalogStore(path: account.folder.appendingPathComponent("catalog.sqlite").path)
        sync = SyncEngine(client: client, store: store)
        covers = CoverCache(client: client, accountID: account.id, renderKey: account.thumbnailRenderKey)
    }
}

/// What the last sync found, shown above the Library.
enum SyncStatus: Equatable {
    case idle
    case syncing
    /// The server couldn't be reached; the copy on the device still works.
    case offline
    /// The server and the app don't share a protocol. Syncing waits for an
    /// update; everything on the device keeps working (ADR 0020).
    /// appTooOld says the update is the app's.
    case versionGap(String, appTooOld: Bool)
    case failed(String)
}

@MainActor
@Observable
final class AppModel {
    private let accounts = AccountStore()

    private(set) var session: AccountSession?
    private(set) var syncStatus: SyncStatus = .idle
    /// Set when the server ended this installation's sign-in, for the
    /// sign-in screen to explain.
    private(set) var signedOutAccount: Account?

    init() {
        if let current = accounts.current {
            if current.signedOut {
                signedOutAccount = current
            } else if let credential = Keychain.credential(for: current.id) {
                session = try? openSession(account: current, credential: credential)
            }
        }
    }

    // MARK: - Signing in and out

    /// Signs in, and downloads the catalog. Throws a message to show.
    func signIn(server: String, username: String, password: String) async throws {
        let address: ServerAddress
        do {
            address = try ServerAddress(server)
        } catch let problem as ServerAddress.Problem {
            throw SignInError(message: Messages.address(problem))
        }
        let client = APIClient(baseURL: address.url, credential: nil)
        let response: Shishoapp.SignInResponse
        do {
            response = try await client.signIn(username: username, password: password, deviceName: Installation.deviceName)
        } catch let error as APIError {
            throw SignInError(message: Messages.signIn(error), offersUpdate: Messages.isAppTooOld(error))
        }

        var account = accounts.account(serverURL: address.url, userID: response.user.id) ?? Account(
            id: UUID().uuidString.lowercased(), serverURL: address.url, username: response.user.username,
            userID: response.user.id, deviceID: response.deviceId, serverVersion: response.serverVersion,
            thumbnailRenderKey: response.server.thumbnailRenderKey, capabilities: response.server.capabilities,
            signedOut: false
        )
        account.username = response.user.username
        account.deviceID = response.deviceId
        account.serverVersion = response.serverVersion
        account.thumbnailRenderKey = response.server.thumbnailRenderKey
        account.capabilities = response.server.capabilities
        account.signedOut = false
        do {
            try Keychain.setCredential(response.credential, for: account.id)
        } catch {
            // The Device exists on the server, but this installation couldn't
            // keep its credential, so it signs that Device out again.
            await APIClient(baseURL: address.url, credential: response.credential).signOut()
            throw SignInError(message: String(localized: "The app couldn't save your sign-in on this device. Try again."))
        }
        accounts.save(account)
        do {
            session = try openSession(account: account, credential: response.credential)
        } catch {
            throw SignInError(message: String(localized: "The app couldn't open its storage on this device. \(error.localizedDescription)"))
        }
        signedOutAccount = nil
        await syncNow()
    }

    /// Signs this installation out of the current account: the server
    /// forgets the Device, and the app its credential. The account's copy of
    /// the catalog stays for signing in again.
    func signOut() async {
        guard let session else { return }
        await session.client.signOut()
        Keychain.removeCredential(for: session.account.id)
        accounts.leaveCurrent()
        self.session = nil
        signedOutAccount = nil
        syncStatus = .idle
    }

    /// The server ended the sign-in. Its data stays, waiting for the same
    /// user to sign in again.
    private func handleSignedOut() {
        guard var account = session?.account else { return }
        account.signedOut = true
        Keychain.removeCredential(for: account.id)
        accounts.save(account)
        signedOutAccount = account
        session = nil
        syncStatus = .idle
    }

    // MARK: - Syncing

    /// Brings the catalog up to date. Failing to reach the server leaves the
    /// copy on the device as it is.
    func syncNow() async {
        guard let session, syncStatus != .syncing else { return }
        syncStatus = .syncing
        do {
            try await session.sync.run()
            syncStatus = .idle
        } catch APIError.signedOut {
            handleSignedOut()
        } catch APIError.unreachable {
            syncStatus = .offline
        } catch let error as APIError {
            switch error {
            case .appTooOld, .serverTooOld, .serverPredatesApp:
                syncStatus = .versionGap(Messages.signIn(error, serverVersion: session.account.serverVersion),
                                         appTooOld: Messages.isAppTooOld(error))
            default:
                syncStatus = .failed(Messages.signIn(error))
            }
        } catch {
            syncStatus = .failed(error.localizedDescription)
        }
    }

    /// The account's session, whose responses keep its capabilities current.
    private func openSession(account: Account, credential: String) throws -> AccountSession {
        let accountID = account.id
        return try AccountSession(account: account, credential: credential) { [weak self] listed in
            Task { @MainActor in self?.recordCapabilities(listed, accountID: accountID) }
        }
    }

    private func recordCapabilities(_ listed: [String], accountID: String) {
        guard var account = session?.account, account.id == accountID, account.capabilities != listed else { return }
        account.capabilities = listed
        accounts.save(account)
    }
}

struct SignInError: LocalizedError {
    let message: String
    /// The fix is updating the app, so the screen links to the update.
    var offersUpdate = false
    var errorDescription: String? { message }
}

/// The words for each problem, in one place.
enum Messages {
    static func address(_ problem: ServerAddress.Problem) -> String {
        switch problem {
        case .empty:
            return String(localized: "Enter your Shisho server's address.")
        case .invalid:
            return String(localized: "That doesn't look like a server address. Try something like https://shisho.example.com.")
        case .insecureHTTP:
            return String(localized: "Plain HTTP only works for servers on your local network. Use the server's https:// address.")
        }
    }

    static func signIn(_ error: APIError, serverVersion: String? = nil) -> String {
        let app = Installation.appVersion
        let needed = Installation.minimumServerVersion
        switch error {
        case .serverPredatesApp:
            return String(localized: "This server's Shisho is too old for this version of the app (\(app)). The app needs Shisho \(needed) or newer. Ask the server's owner to update Shisho.")
        case .serverTooOld(let version):
            let running = version ?? serverVersion ?? String(localized: "an older release")
            return String(localized: "This server runs Shisho \(running), which is too old for this version of the app (\(app)). Ask the server's owner to update Shisho.")
        case .appTooOld(let version):
            let running = version ?? serverVersion ?? String(localized: "a newer release")
            return String(localized: "This version of the app (\(app)) is too old for this server, which runs Shisho \(running). Update the Shisho app to keep syncing.")
        case .demoMode:
            return String(localized: "This is a Shisho demo server. The Shisho app doesn't connect to demo servers; sign in to your own Shisho server instead.")
        case .wrongPassword:
            return String(localized: "Wrong username or password.")
        case .passwordResetRequired:
            return String(localized: "Your password has to be changed before you can sign in. Sign in on the web to change it, then try again.")
        case .noAccess:
            return String(localized: "Your account can't read Books on this server. Ask the server's owner for access.")
        case .signedOut:
            return String(localized: "This device was signed out of your account. Sign in again.")
        case .unreachable(let detail):
            return String(localized: "Couldn't reach the server. Check the address and your connection. (\(detail))")
        case .server(_, let message):
            return message
        }
    }

    static func isAppTooOld(_ error: APIError) -> Bool {
        if case .appTooOld = error { return true }
        return false
    }
}
