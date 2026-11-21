import XCTest
@testable import Shisho

/// The system player takes cookies, not headers, so the stream's credential
/// and protocol go as cookies for the server it streams from.
final class StreamCookiesTests: XCTestCase {
    private func cookies(_ server: String) -> [String: HTTPCookie] {
        let client = APIClient(baseURL: URL(string: server)!, credential: "secret-credential")
        return Dictionary(uniqueKeysWithValues: client.streamCookies.map { ($0.name, $0) })
    }

    func testCarriesTheCredentialAndProtocolForTheServer() throws {
        let local = cookies("http://192.168.1.20:3789")
        XCTAssertEqual(local["shisho_app_credential"]?.value, "secret-credential")
        XCTAssertEqual(local["shisho_app_protocol"]?.value, String(Installation.appProtocol))
        XCTAssertEqual(local["shisho_app_version"]?.value, Installation.appVersion)
        XCTAssertEqual(local["shisho_app_credential"]?.domain, "192.168.1.20")
        XCTAssertEqual(local["shisho_app_credential"]?.isSecure, false)

        let remote = cookies("https://books.example.com")
        XCTAssertEqual(remote["shisho_app_credential"]?.domain, "books.example.com")
        XCTAssertEqual(remote["shisho_app_credential"]?.isSecure, true)
        XCTAssertNotNil(cookies("http://localhost:3789")["shisho_app_credential"])
    }
}
