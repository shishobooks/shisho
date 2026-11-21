import XCTest
@testable import Shisho

final class KeychainTests: XCTestCase {
    /// The credential has to survive the app closing, so a build that can't
    /// write the Keychain fails here rather than signing everyone out.
    func testKeepsACredential() throws {
        let account = "test-\(UUID().uuidString)"
        defer { Keychain.removeCredential(for: account) }
        try Keychain.setCredential("secret", for: account)
        XCTAssertEqual(Keychain.credential(for: account), "secret")
        Keychain.removeCredential(for: account)
        XCTAssertNil(Keychain.credential(for: account))
    }
}
