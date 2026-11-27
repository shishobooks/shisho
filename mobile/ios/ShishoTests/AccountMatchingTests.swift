import XCTest
@testable import Shisho

/// A sign-in is the same account only on the same server instance: a server
/// set up again from scratch at the same address hands out the same user
/// ids, and its journal must not reach the new one.
final class AccountMatchingTests: XCTestCase {
    private let server = URL(string: "https://shisho.example.com")!

    private func account(_ id: String, server: URL? = nil, userID: Int = 1, instanceID: String) -> Account {
        Account(id: id, serverURL: server ?? self.server, username: "robin", userID: userID, deviceID: "device",
                serverVersion: "v1.0.0", thumbnailRenderKey: "r", capabilities: [], signedOut: false,
                instanceID: instanceID)
    }

    func testNoStoredAccountIsNew() {
        let stored = [account("a", userID: 2, instanceID: "one"),
                      account("b", server: URL(string: "https://other.example.com")!, instanceID: "one")]
        XCTAssertEqual(AccountStore.match(stored, serverURL: server, userID: 1, instanceID: "one"), .new)
    }

    func testTheSameInstanceIsTheSameAccount() {
        let stored = account("a", instanceID: "one")
        XCTAssertEqual(AccountStore.match([stored], serverURL: server, userID: 1, instanceID: "one"), .same(stored))
    }

    func testAnotherInstanceReplacesTheAccount() {
        let stored = account("a", instanceID: "one")
        XCTAssertEqual(AccountStore.match([stored], serverURL: server, userID: 1, instanceID: "two"), .replaced(stored))
    }

    /// An account left behind beside the one that replaced it, as by the
    /// app being killed midway, never replaces it again.
    func testTheAccountForThisInstanceWinsOverAnOlderOne() {
        let older = account("a", instanceID: "one")
        let current = account("b", instanceID: "two")
        XCTAssertEqual(AccountStore.match([older, current], serverURL: server, userID: 1, instanceID: "two"), .same(current))
    }

    /// Removing an account removes everything kept for it, and nothing of
    /// another account's.
    func testRemovingAnAccountRemovesEverythingKeptForIt() throws {
        let removed = "test-\(UUID().uuidString)"
        let kept = "test-\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: "AccountMatchingTests-\(UUID().uuidString)"))
        let fileManager = FileManager.default
        defer {
            for id in [removed, kept] {
                AccountData(accountID: id).remove(defaults: defaults)
            }
        }
        for id in [removed, kept] {
            let data = AccountData(accountID: id)
            for folder in data.folders {
                try fileManager.createDirectory(at: folder, withIntermediateDirectories: true)
                try Data("x".utf8).write(to: folder.appendingPathComponent("file"))
            }
            try Keychain.setCredential("secret", for: id)
            defaults.set(3, forKey: data.libraryIDKey)
            defaults.set("title", forKey: data.sortKey(libraryID: 3))
            defaults.set(7, forKey: data.lastFileKey)
            defaults.set(1.5, forKey: data.speedKey(fileID: 7))
        }

        AccountData(accountID: removed).remove(defaults: defaults)

        for folder in AccountData(accountID: removed).folders {
            XCTAssertFalse(fileManager.fileExists(atPath: folder.path), folder.path)
        }
        XCTAssertNil(Keychain.credential(for: removed))
        let gone = AccountData(accountID: removed)
        for key in [gone.libraryIDKey, gone.sortKey(libraryID: 3), gone.lastFileKey, gone.speedKey(fileID: 7)] {
            XCTAssertNil(defaults.object(forKey: key), key)
        }

        for folder in AccountData(accountID: kept).folders {
            XCTAssertTrue(fileManager.fileExists(atPath: folder.appendingPathComponent("file").path), folder.path)
        }
        XCTAssertEqual(Keychain.credential(for: kept), "secret")
        let stays = AccountData(accountID: kept)
        XCTAssertEqual(defaults.integer(forKey: stays.libraryIDKey), 3)
        XCTAssertEqual(defaults.string(forKey: stays.sortKey(libraryID: 3)), "title")
        XCTAssertEqual(defaults.double(forKey: stays.speedKey(fileID: 7)), 1.5)
    }

    /// A removed account stays listed as removing until a launch finds its
    /// folders gone, so a quit while they're deleted doesn't leave them.
    func testRemovingGoesOnUntilTheFoldersAreGone() throws {
        let left = UUID().uuidString.lowercased()
        let gone = UUID().uuidString.lowercased()
        let cache = CoverCache.folder(accountID: left)
        try FileManager.default.createDirectory(at: cache, withIntermediateDirectories: true)
        defer { AccountData(accountID: left).removeFolders() }

        XCTAssertEqual(AccountStore.withFoldersLeft([left, gone]), [left])
        AccountData(accountID: left).removeFolders()
        XCTAssertEqual(AccountStore.withFoldersLeft([left, gone]), [])
    }
}
