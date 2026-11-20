import XCTest
@testable import Shisho

final class ServerAddressTests: XCTestCase {
    func testAddsHTTPSForAnInternetHost() throws {
        XCTAssertEqual(try ServerAddress("shisho.example.com").url.absoluteString, "https://shisho.example.com")
        XCTAssertEqual(try ServerAddress(" https://shisho.example.com/ ").url.absoluteString, "https://shisho.example.com")
    }

    func testAddsHTTPForALocalHost() throws {
        XCTAssertEqual(try ServerAddress("192.168.1.20:5173").url.absoluteString, "http://192.168.1.20:5173")
        XCTAssertEqual(try ServerAddress("nas.local:3689").url.absoluteString, "http://nas.local:3689")
        XCTAssertEqual(try ServerAddress("http://nas:3689").url.absoluteString, "http://nas:3689")
    }

    func testRefusesPlainHTTPToTheInternet() {
        XCTAssertThrowsError(try ServerAddress("http://shisho.example.com")) { error in
            XCTAssertEqual(error as? ServerAddress.Problem, .insecureHTTP)
        }
        XCTAssertThrowsError(try ServerAddress("http://8.8.8.8")) { error in
            XCTAssertEqual(error as? ServerAddress.Problem, .insecureHTTP)
        }
    }

    func testRefusesNothingAndNonsense() {
        XCTAssertThrowsError(try ServerAddress("  ")) { XCTAssertEqual($0 as? ServerAddress.Problem, .empty) }
        XCTAssertThrowsError(try ServerAddress("ftp://example.com")) { XCTAssertEqual($0 as? ServerAddress.Problem, .invalid) }
    }

    func testLocalHosts() {
        for host in ["localhost", "10.0.0.2", "172.20.1.1", "192.168.0.10", "169.254.3.4", "100.100.1.1", "nas", "box.local", "::1", "fe80::1", "fd12::1"] {
            XCTAssertTrue(ServerAddress.isLocal(host: host), host)
        }
        for host in ["8.8.8.8", "172.32.0.1", "example.com", "2001:db8::1"] {
            XCTAssertFalse(ServerAddress.isLocal(host: host), host)
        }
    }
}
