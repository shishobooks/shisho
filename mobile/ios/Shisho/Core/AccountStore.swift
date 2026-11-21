import Foundation

/// One account on one server that this installation signed in to.
struct Account: Codable, Identifiable, Hashable {
    /// The account's id on this installation, which names its folder.
    let id: String
    var serverURL: URL
    var username: String
    var userID: Int
    var deviceID: String
    /// The Shisho release the server ran when it was last heard from.
    var serverVersion: String
    var thumbnailRenderKey: String
    var capabilities: [String]
    /// The server ended the Device's sign-in (revoked, or the user was
    /// deactivated). The account keeps its data until it signs in again.
    var signedOut: Bool

    /// The folder holding this account's database.
    var folder: URL {
        AccountStore.accountsFolder.appendingPathComponent(id, isDirectory: true)
    }
}

/// The accounts this installation signed in to, kept in Application
/// Support. Credentials stay in the Keychain, never here.
final class AccountStore {
    static var accountsFolder: URL {
        FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("Accounts", isDirectory: true)
    }

    private static var indexURL: URL {
        accountsFolder.appendingPathComponent("accounts.json")
    }

    private struct Index: Codable {
        var accounts: [Account] = []
        var currentID: String?
    }

    private var index: Index

    init() {
        if let data = try? Data(contentsOf: AccountStore.indexURL),
           let decoded = try? JSONDecoder().decode(Index.self, from: data) {
            index = decoded
        } else {
            index = Index()
        }
    }

    var accounts: [Account] { index.accounts }

    var current: Account? {
        index.accounts.first { $0.id == index.currentID }
    }

    /// The account for a user on a server, so signing in again reuses its
    /// database.
    func account(serverURL: URL, userID: Int) -> Account? {
        index.accounts.first { $0.serverURL == serverURL && $0.userID == userID }
    }

    func save(_ account: Account, makeCurrent: Bool = true) {
        if let position = index.accounts.firstIndex(where: { $0.id == account.id }) {
            index.accounts[position] = account
        } else {
            index.accounts.append(account)
        }
        if makeCurrent {
            index.currentID = account.id
        }
        write()
    }

    /// Stops using the current account, keeping its data for signing in
    /// again.
    func leaveCurrent() {
        index.currentID = nil
        write()
    }

    private func write() {
        try? FileManager.default.createDirectory(at: AccountStore.accountsFolder, withIntermediateDirectories: true)
        if let data = try? JSONEncoder().encode(index) {
            try? data.write(to: AccountStore.indexURL, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
        }
    }
}
