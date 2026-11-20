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

    func testMessagesNameBothVersions() {
        let tooOld = Messages.signIn(.serverTooOld(serverVersion: "v0.10.0"))
        XCTAssertTrue(tooOld.contains("v0.10.0"), tooOld)
        XCTAssertTrue(tooOld.contains(Installation.appVersion), tooOld)
        let predates = Messages.signIn(.serverPredatesApp)
        XCTAssertTrue(predates.contains(Installation.minimumServerVersion), predates)
        XCTAssertTrue(Messages.isAppTooOld(.appTooOld(serverVersion: nil)))
    }
}
