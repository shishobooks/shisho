import XCTest
@testable import Shisho

/// Which account takes over when one's sign-in ends, by the user signing
/// out or by the server ending it, so the user's other libraries stay in
/// reach.
final class AccountSwitchingTests: XCTestCase {
    private func account(_ id: String, signedOut: Bool = false) -> Account {
        Account(id: id, serverURL: URL(string: "https://\(id).example.com")!, username: "robin", userID: 1, deviceID: "d",
                serverVersion: "0.10.0", thumbnailRenderKey: "", capabilities: [], signedOut: signedOut, instanceID: "i")
    }

    func testTheFirstOtherAccountStillSignedInTakesOver() {
        let accounts = [account("a"), account("b", signedOut: true), account("c"), account("d")]
        let next = AccountSwitching.next(after: "a", in: accounts) { $0 != "c" }
        XCTAssertEqual(next?.id, "d", "b is signed out, and c has no credential on this device")
    }

    func testNoneWhenNoOtherAccountIsSignedIn() {
        let accounts = [account("a"), account("b", signedOut: true)]
        XCTAssertNil(AccountSwitching.next(after: "a", in: accounts) { _ in true })
    }

    func testTheOtherAccountsTheSignInScreenOffers() {
        let accounts = [account("a", signedOut: true), account("b"), account("c", signedOut: true), account("d")]
        let offered = AccountSwitching.signedIn(in: accounts, except: "a") { $0 != "d" }
        XCTAssertEqual(offered.map(\.id), ["b"])
    }
}
