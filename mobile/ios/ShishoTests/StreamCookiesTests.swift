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

    /// The cookies go only with requests for Files, so a redirect elsewhere
    /// on the same host doesn't carry the credential.
    func testGoOnlyToTheServersFileRoutes() throws {
        XCTAssertEqual(cookies("http://192.168.1.20:3789")["shisho_app_credential"]?.path, "/mobile/files/")
        XCTAssertEqual(cookies("https://example.com/shisho")["shisho_app_credential"]?.path, "/shisho/mobile/files/")
        XCTAssertEqual(cookies("https://example.com/shisho/")["shisho_app_credential"]?.path, "/shisho/mobile/files/")

        // A server under a path with a space: the cookie goes with its
        // streams as the system sends them.
        let client = APIClient(baseURL: URL(string: "https://example.com/my%20books")!, credential: "secret-credential")
        let storage = HTTPCookieStorage.sharedCookieStorage(forGroupContainerIdentifier: UUID().uuidString)
        storage.setCookies(client.streamCookies, for: client.streamURL(fileID: 3), mainDocumentURL: nil)
        XCTAssertEqual(storage.cookies(for: client.streamURL(fileID: 3))?.map(\.name).contains("shisho_app_credential"), true)
    }

    /// The stream's cookies and every request's headers come from one list
    /// (`APIClient.identity`), so a field added to one reaches the other.
    func testEveryHeaderARequestCarriesHasACookieForTheStream() throws {
        let client = APIClient(baseURL: URL(string: "https://books.example.com")!, credential: "secret-credential", instanceID: "instance-1")
        let request = client.downloadRequest(fileID: 3)
        let cookies = Dictionary(uniqueKeysWithValues: client.streamCookies.map { ($0.name, $0.value) })
        XCTAssertFalse(client.identity.isEmpty)
        for field in client.identity {
            XCTAssertEqual(request.value(forHTTPHeaderField: field.header), field.headerValue, field.header)
            XCTAssertEqual(cookies[field.cookie], field.cookieValue, field.cookie)
        }
        XCTAssertEqual(cookies["shisho_app_instance"], "instance-1", "the instance reaches the stream too")
        XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer secret-credential")
        XCTAssertEqual(cookies["shisho_app_credential"], "secret-credential")
    }

    func testAnEmptyInstanceIsntSent() throws {
        let client = APIClient(baseURL: URL(string: "https://books.example.com")!, credential: "secret-credential")
        XCTAssertNil(client.downloadRequest(fileID: 3).value(forHTTPHeaderField: "X-Shisho-Instance"))
        XCTAssertFalse(client.streamCookies.contains { $0.name == "shisho_app_instance" })
    }
}
