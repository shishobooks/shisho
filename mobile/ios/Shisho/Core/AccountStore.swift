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
    /// The server's instance id, which tells it apart from another server
    /// set up again from scratch at the same address. Nil for an account
    /// saved before servers had one, or on a server too old to send it.
    var instanceID: String?

    /// The folder holding this account's database.
    var folder: URL {
        AccountData(accountID: id).folder
    }
}

/// How a sign-in relates to the accounts already on this installation.
enum AccountMatch: Equatable {
    /// No account for this user on this server yet.
    case new
    /// The account signed in to before, to keep using with its journal.
    case same(Account)
    /// An account for the same address and user id on another server
    /// instance: the server was set up again from scratch, so this is a
    /// new account, and the old one's data is for a server that's gone.
    case replaced(Account)
}

/// Everything this installation keeps for one account, so removing the
/// account removes all of it. Something new kept per account adds its
/// location or settings key here, and removal follows.
struct AccountData {
    let accountID: String

    /// The folder holding the account's database.
    var folder: URL {
        AccountStore.accountsFolder.appendingPathComponent(accountID, isDirectory: true)
    }

    /// The account's database.
    var catalogPath: String {
        folder.appendingPathComponent("catalog.sqlite").path
    }

    /// Every folder kept for the account: its database and its caches.
    var folders: [URL] {
        [folder, CoverCache.folder(accountID: accountID), EbookCache.folder(accountID: accountID)]
    }

    // The account's settings on this device, by key.

    /// The Library picked.
    var libraryIDKey: String { "libraryID.\(accountID)" }
    /// The order picked for a Library, or 0 for every Library.
    func sortKey(libraryID: Int) -> String { sortKeyPrefix + String(libraryID) }
    private var sortKeyPrefix: String { "sort.\(accountID)." }
    /// Downloading every cover on Wi-Fi.
    var prefetchCoversKey: String { "prefetchCovers.\(accountID)" }
    /// The audiobook File the player had open.
    var lastFileKey: String { "player.lastFile.\(accountID)" }
    /// The speed picked for an audiobook File.
    func speedKey(fileID: Int) -> String { speedKeyPrefix + String(fileID) }
    private var speedKeyPrefix: String { "player.speed.\(accountID).file." }

    /// Deletes the account's folders, its credential, and its settings on
    /// this device.
    func remove(defaults: UserDefaults = .standard) {
        removeFolders()
        removeCredentialAndSettings(defaults: defaults)
    }

    /// Deletes the account's folders, which can hold thousands of covers.
    func removeFolders() {
        for folder in folders {
            try? FileManager.default.removeItem(at: folder)
        }
    }

    func removeCredentialAndSettings(defaults: UserDefaults = .standard) {
        Keychain.removeCredential(for: accountID)
        let exact: Set = [libraryIDKey, prefetchCoversKey, lastFileKey]
        let prefixes = [sortKeyPrefix, speedKeyPrefix]
        for key in defaults.dictionaryRepresentation().keys
        where exact.contains(key) || prefixes.contains(where: { key.hasPrefix($0) }) {
            defaults.removeObject(forKey: key)
        }
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
        /// Accounts removed whose folders may not all be deleted yet.
        var removing: [String]?
    }

    private var index: Index

    init() {
        if let data = try? Data(contentsOf: AccountStore.indexURL),
           let decoded = try? JSONDecoder().decode(Index.self, from: data) {
            index = decoded
        } else {
            index = Index()
        }
        // Deleting a removed account's folders goes on in the background, so
        // quitting can cut it short. What's left is deleted now, and an
        // account stays listed until a launch finds its folders gone.
        if let removing = index.removing {
            let left = AccountStore.withFoldersLeft(removing)
            if left != removing {
                index.removing = left.isEmpty ? nil : left
                write()
            }
            if !left.isEmpty {
                Task.detached(priority: .utility) {
                    for id in left {
                        AccountData(accountID: id).removeFolders()
                    }
                }
            }
        }
    }

    /// The accounts among ids with any of their folders still on disk.
    static func withFoldersLeft(_ ids: [String]) -> [String] {
        ids.filter { id in
            AccountData(accountID: id).folders.contains { FileManager.default.fileExists(atPath: $0.path) }
        }
    }

    var accounts: [Account] { index.accounts }

    var current: Account? {
        index.accounts.first { $0.id == index.currentID }
    }

    /// The account a sign-in to a user on a server is, so signing in again
    /// reuses its database.
    func match(serverURL: URL, userID: Int, instanceID: String) -> AccountMatch {
        AccountStore.match(index.accounts, serverURL: serverURL, userID: userID, instanceID: instanceID)
    }

    /// The account for a user on a server: the one with the same address
    /// and user id, unless both it and the server have an instance id and
    /// they differ. An account stored without one adopts the server's, and a
    /// server too old to send one is never a change.
    static func match(_ accounts: [Account], serverURL: URL, userID: Int, instanceID: String) -> AccountMatch {
        let candidates = accounts.filter { $0.serverURL == serverURL && $0.userID == userID }
        // The account for this very instance wins over one it replaced.
        guard let known = candidates.first(where: { !instanceID.isEmpty && $0.instanceID == instanceID }) ?? candidates.first else {
            return .new
        }
        guard !instanceID.isEmpty, let stored = known.instanceID, !stored.isEmpty, stored != instanceID else {
            return .same(known)
        }
        return .replaced(known)
    }

    /// Saves account, and whether that reached the disk.
    @discardableResult
    func save(_ account: Account, makeCurrent: Bool = true) -> Bool {
        if let position = index.accounts.firstIndex(where: { $0.id == account.id }) {
            index.accounts[position] = account
        } else {
            index.accounts.append(account)
        }
        if makeCurrent {
            index.currentID = account.id
        }
        return write()
    }

    /// Saves account as the current one in place of replaced, in one write,
    /// then deletes everything kept for replaced, unsent changes included.
    /// If the write fails, nothing changes, in memory or on disk, and it
    /// returns false.
    func save(_ account: Account, replacing replaced: Account) -> Bool {
        let before = index
        index.accounts.removeAll { $0.id == replaced.id }
        index.removing = (index.removing ?? []) + [replaced.id]
        guard save(account) else {
            index = before
            return false
        }
        // The index no longer lists it, so nothing opens its folders again,
        // and deleting what may be thousands of covers needn't hold up the
        // account opening in its place. It's listed as removing, so a quit
        // before this finishes is caught on the next launch.
        let removed = AccountData(accountID: replaced.id)
        removed.removeCredentialAndSettings()
        Task.detached(priority: .utility) { removed.removeFolders() }
        return true
    }

    /// Stops using the current account, keeping its data for signing in
    /// again.
    func leaveCurrent() {
        index.currentID = nil
        write()
    }

    @discardableResult
    private func write() -> Bool {
        do {
            try FileManager.default.createDirectory(at: AccountStore.accountsFolder, withIntermediateDirectories: true)
            let data = try JSONEncoder().encode(index)
            try data.write(to: AccountStore.indexURL, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
            return true
        } catch {
            Task { await Diagnostics.shared.log(.app, "couldn't save the accounts: \(Diagnostics.describe(error))") }
            return false
        }
    }
}
