import XCTest
@testable import Shisho

final class APIErrorTests: XCTestCase {
    private func error(_ status: Int, _ code: String, headers: [String: String] = [:]) -> APIError {
        let response = HTTPURLResponse(url: URL(string: "https://example.com")!, statusCode: status, httpVersion: nil, headerFields: headers)!
        let body = Data(#"{"error":{"code":"\#(code)","message":"m","status_code":\#(status)}}"#.utf8)
        return APIClient.error(status: status, response: response, body: body)
    }

    func testRecognizesTheServersCodes() {
        XCTAssertEqual(error(403, "demo_mode"), .demoMode)
        XCTAssertEqual(error(426, "app_update_required", headers: ["X-Shisho-Server-Version": "v0.12.0"]), .appTooOld(serverVersion: "v0.12.0"))
        XCTAssertEqual(error(426, "server_update_required"), .serverTooOld(serverVersion: nil))
        XCTAssertEqual(error(401, "unauthorized"), .signedOut)
        XCTAssertEqual(error(403, "password_reset_required"), .passwordResetRequired)
        XCTAssertEqual(error(500, "internal_server_error"), .server(status: 500, message: "m"))
    }

    /// Only Shisho's own refusal signs the device out. A 401 or 403 from
    /// something in front of the server, such as an authentication proxy,
    /// keeps the credential and says what answered.
    func testRefusalsWithoutShishosBodyCameFromAProxy() {
        let response = { (status: Int) in HTTPURLResponse(url: URL(string: "https://example.com")!, statusCode: status, httpVersion: nil, headerFields: nil)! }
        let page = Data("<html><body>Sign in to Authelia</body></html>".utf8)
        XCTAssertEqual(APIClient.error(status: 401, response: response(401), body: page), .blockedByProxy)
        XCTAssertEqual(APIClient.error(status: 403, response: response(403), body: page), .blockedByProxy)
        XCTAssertEqual(APIClient.error(status: 401, response: response(401), body: Data(#"{"message":"Unauthorized"}"#.utf8)), .blockedByProxy)
        XCTAssertEqual(error(403, "forbidden"), .noAccess)
    }

    /// The API types decode leniently, so any JSON decodes. A feed page or a
    /// sign-in without the fields only Shisho sends came from something
    /// else, and must not change what's on the device.
    func testResponsesNeedShishosFields() throws {
        let unrelated = Data(#"{"status":"ok"}"#.utf8)
        XCTAssertThrowsError(try APIClient.feedPage(unrelated)) { XCTAssertEqual($0 as? APIError, .serverPredatesApp) }
        XCTAssertThrowsError(try APIClient.signInResponse(unrelated)) { XCTAssertEqual($0 as? APIError, .serverPredatesApp) }

        let page = try APIClient.feedPage(Data(#"{"entries":[],"cursor":0,"has_more":false}"#.utf8))
        XCTAssertEqual(page.cursor, 0)
        let signIn = try APIClient.signInResponse(Data(#"{"credential":"abc","device_id":"dev-3","user":{"id":1,"username":"u"},"server":{}}"#.utf8))
        XCTAssertEqual(signIn.credential, "abc")
    }

    func testMessagesNameBothVersions() {
        let tooOld = Messages.signIn(.serverTooOld(serverVersion: "v0.10.0"))
        XCTAssertTrue(tooOld.contains("v0.10.0"), tooOld)
        XCTAssertTrue(tooOld.contains(Installation.appVersion), tooOld)
        let predates = Messages.signIn(.serverPredatesApp)
        XCTAssertTrue(predates.contains(Installation.minimumServerVersion), predates)
        XCTAssertTrue(Messages.isAppTooOld(.appTooOld(serverVersion: nil)))
    }
}
