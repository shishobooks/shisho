#if DEBUG
import XCTest
@testable import Shisho

final class HarnessSignInTests: XCTestCase {
    func testReadsTheAccountFromLaunchArguments() {
        let signIn = HarnessSignIn(arguments: [
            "harnessServer": "http://127.0.0.1:4000",
            "harnessUsername": "harness",
            "harnessPassword": "secret",
        ])
        XCTAssertEqual(signIn, HarnessSignIn(server: "http://127.0.0.1:4000", username: "harness", password: "secret"))
    }

    func testNeedsAllThree() {
        XCTAssertNil(HarnessSignIn(arguments: [:]))
        XCTAssertNil(HarnessSignIn(arguments: ["harnessServer": "http://127.0.0.1:4000", "harnessUsername": "harness"]))
        XCTAssertNil(HarnessSignIn(arguments: ["harnessServer": "", "harnessUsername": "harness", "harnessPassword": "secret"]))
    }
}
#endif
