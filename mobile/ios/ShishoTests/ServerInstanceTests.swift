import Foundation
import os
import XCTest
@testable import Shisho

/// Answers every request from a server whose instance is
/// StubInstanceServer.served, with an empty feed page or, when
/// refusing, Shisho's refusal of the credential, and keeps the instance each
/// request named.
private final class StubInstanceServer: URLProtocol {
    private struct State {
        var served = ""
        var refusing = false
        var named: [String?] = []
    }

    private static let state = OSAllocatedUnfairLock(initialState: State())

    static func reset(serving instance: String, refusing: Bool = false) {
        state.withLock { $0 = State(served: instance, refusing: refusing) }
    }

    static var named: [String?] { state.withLock { $0.named } }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let named = request.value(forHTTPHeaderField: "X-Shisho-Instance")
        let (served, refusing) = Self.state.withLock { state in
            state.named.append(named)
            return (state.served, state.refusing)
        }
        let response = HTTPURLResponse(url: request.url!, statusCode: refusing ? 401 : 200, httpVersion: "HTTP/1.1", headerFields: [
            "Content-Type": "application/json",
            "X-Shisho-Instance": served,
        ])!
        let body = refusing
            ? #"{"error":{"code":"unauthorized","message":"m","status_code":401}}"#
            : #"{"entries":[],"cursor":0,"has_more":false,"start_over":false,"expired_through":0}"#
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(body.utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}

/// Each sync names the server instance its account was made on, so a
/// server set up again refuses it (#967).
final class ServerInstanceTests: XCTestCase {
    private let server = URL(string: "http://shisho.test")!

    func testNamesTheAccountsInstance() async throws {
        StubInstanceServer.reset(serving: "one")
        let adopted = OSAllocatedUnfairLock<[String]>(initialState: [])
        let client = APIClient(baseURL: server, credential: "c", instanceID: "one",
                               onInstance: { id in adopted.withLock { $0.append(id) } },
                               protocolClasses: [StubInstanceServer.self])
        _ = try await client.changes(cursor: 0, expiredThrough: 0)
        XCTAssertEqual(StubInstanceServer.named, ["one"])
        XCTAssertEqual(adopted.withLock { $0 }, [])
    }

    /// An account from before servers had an instance id names none until
    /// a response says which it is, then keeps that one.
    func testAnAccountWithoutAnInstanceAdoptsTheFirstItSees() async throws {
        StubInstanceServer.reset(serving: "one")
        let adopted = OSAllocatedUnfairLock<[String]>(initialState: [])
        let client = APIClient(baseURL: server, credential: "c",
                               onInstance: { id in adopted.withLock { $0.append(id) } },
                               protocolClasses: [StubInstanceServer.self])
        _ = try await client.changes(cursor: 0, expiredThrough: 0)
        _ = try await client.changes(cursor: 0, expiredThrough: 0)
        XCTAssertEqual(StubInstanceServer.named, [nil, "one"])
        XCTAssertEqual(adopted.withLock { $0 }, ["one"])
    }

    /// A server that refuses the credential may be another server, set up
    /// again, so its instance isn't the account's.
    func testARefusedCredentialTeachesNoInstance() async {
        StubInstanceServer.reset(serving: "fresh", refusing: true)
        let adopted = OSAllocatedUnfairLock<[String]>(initialState: [])
        let client = APIClient(baseURL: server, credential: "c",
                               onInstance: { id in adopted.withLock { $0.append(id) } },
                               protocolClasses: [StubInstanceServer.self])
        await XCTAssertThrowsErrorAsync(try await client.changes(cursor: 0, expiredThrough: 0)) {
            XCTAssertEqual($0 as? APIError, .signedOut)
        }
        _ = try? await client.changes(cursor: 0, expiredThrough: 0)
        XCTAssertEqual(StubInstanceServer.named, [nil, nil])
        XCTAssertEqual(adopted.withLock { $0 }, [])
    }

    /// Something in front of the server may drop the instance the app
    /// names, so an answer from another instance is refused here too,
    /// before the feed is applied or the journal sent.
    func testAnAnswerFromAnotherInstanceIsAReplacedServer() async {
        StubInstanceServer.reset(serving: "two")
        let client = APIClient(baseURL: server, credential: "c", instanceID: "one",
                               protocolClasses: [StubInstanceServer.self])
        await XCTAssertThrowsErrorAsync(try await client.changes(cursor: 0, expiredThrough: 0)) {
            XCTAssertEqual($0 as? APIError, .serverReplaced)
        }
    }
}

private func XCTAssertThrowsErrorAsync<T>(_ expression: @autoclosure () async throws -> T,
                                          file: StaticString = #filePath, line: UInt = #line,
                                          _ handler: (Error) -> Void) async {
    do {
        _ = try await expression()
        XCTFail("no error was thrown", file: file, line: line)
    } catch {
        handler(error)
    }
}
