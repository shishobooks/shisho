import os
import XCTest
@testable import Shisho

/// Answers every request with an empty 204, and keeps what each asked.
private final class StubRecordingServer: URLProtocol {
    private static let asked = OSAllocatedUnfairLock<[String]>(initialState: [])

    static func reset() { asked.withLock { $0 = [] } }
    static var requests: [String] { asked.withLock { $0 } }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let asked = "\(request.httpMethod ?? "GET") \(request.url?.path ?? "")"
        Self.asked.withLock { $0.append(asked) }
        let response = HTTPURLResponse(url: request.url!, statusCode: 204, httpVersion: "HTTP/1.1", headerFields: [:])!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}

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

    /// Signing out tells the server even while the last word was that it
    /// can't be reached, which may be out of date: otherwise the Device's
    /// credential would stay valid on a server that's back.
    func testSigningOutTriesTheServerWhateverWasLastHeard() async {
        StubRecordingServer.reset()
        let client = APIClient(baseURL: URL(string: "https://shisho.test")!, credential: "c",
                               protocolClasses: [StubRecordingServer.self])
        client.reachability.failed(URLError(.timedOut))
        await client.signOut()
        XCTAssertEqual(StubRecordingServer.requests, ["DELETE /mobile/device"])
    }

    /// Signing out sends unsent changes first. The server may be back with
    /// nothing having noticed yet, as when the connection returned moments
    /// before, so it's asked first, as a sync asks, rather than failing at
    /// once and keeping the changes on the device.
    func testSendingBeforeSigningOutAsksTheServerFirst() async throws {
        StubRecordingServer.reset()
        let client = APIClient(baseURL: URL(string: "https://shisho.test")!, credential: "c",
                               protocolClasses: [StubRecordingServer.self])
        let store = try CatalogStore(path: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path)
        try store.startOverIfNeeded()
        _ = try await store.pool.write { db in
            try CatalogStore.append(db, type: .readingPosition, at: Date(), payload: Data(#"{"file_id":1,"seconds":1,"percentage":0.1}"#.utf8),
                                    fileID: nil, coalesceKey: nil)
        }
        client.reachability.failed(URLError(.timedOut))

        // The stub's empty answer isn't a journal response; only what was
        // asked matters here.
        try? await SyncEngine(client: client, store: store).sendJournalCheckingFirst()
        XCTAssertEqual(StubRecordingServer.requests, ["GET /mobile/server", "POST /mobile/journal"])
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

    /// Without the server, only what's on the device opens, or what's
    /// already playing; a Book with nothing downloaded looks dimmed.
    func testWhatOpensWithoutTheServer() {
        XCTAssertTrue(ConnectionRules.canOpen(connection: .online, downloaded: false, playing: false))
        XCTAssertFalse(ConnectionRules.canOpen(connection: .unreachable, downloaded: false, playing: false))
        XCTAssertTrue(ConnectionRules.canOpen(connection: .offline, downloaded: true, playing: false))
        XCTAssertTrue(ConnectionRules.canOpen(connection: .offline, downloaded: false, playing: true))

        XCTAssertFalse(ConnectionRules.dimmed(bookID: 1, connection: .online, downloaded: nil))
        XCTAssertTrue(ConnectionRules.dimmed(bookID: 1, connection: .offline, downloaded: nil), "not known yet")
        XCTAssertTrue(ConnectionRules.dimmed(bookID: 1, connection: .unreachable, downloaded: [2]))
        XCTAssertFalse(ConnectionRules.dimmed(bookID: 2, connection: .unreachable, downloaded: [2]))
    }
}
