import os
import XCTest
@testable import Shisho

/// Whether a server can be reached, as every request and screen reads it.
final class ConnectionTests: XCTestCase {
    func testTheNetworkAndTheServerTogetherSayTheConnection() {
        let reachability = ServerReachability()
        let seen = OSAllocatedUnfairLock<[Connection]>(initialState: [])
        reachability.observe { connection in seen.withLock { $0.append(connection) } }
        XCTAssertEqual(reachability.connection, .online)

        reachability.failed(URLError(.timedOut))
        XCTAssertEqual(reachability.connection, .unreachable)
        XCTAssertFalse(reachability.allowsRequests)

        reachability.networkChanged(available: false)
        XCTAssertEqual(reachability.connection, .offline, "no network at all says more than an unanswered server")
        reachability.networkChanged(available: true)
        XCTAssertEqual(reachability.connection, .unreachable, "a network back doesn't say the server answers")

        reachability.answered()
        XCTAssertEqual(reachability.connection, .online)
        XCTAssertEqual(seen.withLock { $0 }, [.unreachable, .offline, .unreachable, .online], "only changes are told")
    }

    /// Only a failure that says the server can't be reached counts; one the
    /// app caused, such as a cancelled request, doesn't.
    func testWhichFailuresMeanUnreachable() {
        let reachability = ServerReachability()
        reachability.failed(URLError(.cancelled))
        reachability.failed(APIError.server(status: 500, message: "boom"))
        XCTAssertEqual(reachability.connection, .online)
        for code in [URLError.Code.notConnectedToInternet, .networkConnectionLost, .cannotConnectToHost, .cannotFindHost] {
            XCTAssertTrue(ConnectionRules.meansUnreachable(URLError(code)), "\(code)")
        }
    }

    /// A client known not to reach its server fails at once rather than
    /// waiting out a timeout.
    func testRequestsFailAtOnceWhileTheServerCantBeReached() async {
        let client = APIClient(baseURL: URL(string: "https://shisho.invalid")!, credential: "c")
        client.reachability.networkChanged(available: false)
        let started = Date()
        do {
            _ = try await client.fileCover(fileID: 1, version: "1", size: 128, renderKey: "")
            XCTFail("expected a failure")
        } catch {
            XCTAssertEqual(error as? APIError, .unreachable("offline"))
        }
        XCTAssertLessThan(Date().timeIntervalSince(started), 1)
    }
}
