import XCTest
@testable import Shisho

/// The app's requests carry a password or a credential, so a redirect may
/// only keep them on the same server: the same host and port, or the same
/// host moving from HTTP to HTTPS.
final class RedirectPolicyTests: XCTestCase {
    private func allows(_ from: String, _ to: String) -> Bool {
        RedirectPolicy.allows(from: URL(string: from)!, to: URL(string: to)!)
    }

    func testAllowsTheSameServer() {
        XCTAssertTrue(allows("https://books.example.com/mobile/sign-in", "https://books.example.com/shisho/mobile/sign-in"))
        XCTAssertTrue(allows("http://nas.local:3689/mobile/sign-in", "http://nas.local:3689/mobile/sign-in/"))
        XCTAssertTrue(allows("https://Books.Example.com/a", "https://books.example.com:443/b"))
    }

    func testAllowsUpgradingToHTTPS() {
        XCTAssertTrue(allows("http://books.example.com/mobile/sign-in", "https://books.example.com/mobile/sign-in"))
    }

    func testRefusesAnotherServer() {
        XCTAssertFalse(allows("https://books.example.com/mobile/sign-in", "https://evil.example.net/mobile/sign-in"))
        XCTAssertFalse(allows("https://books.example.com/a", "https://books.example.com:8443/a"))
        XCTAssertFalse(allows("http://nas.local:3689/a", "http://nas.local:3690/a"))
    }

    func testRefusesDowngradingToHTTP() {
        XCTAssertFalse(allows("https://books.example.com/a", "http://books.example.com/a"))
    }
}
