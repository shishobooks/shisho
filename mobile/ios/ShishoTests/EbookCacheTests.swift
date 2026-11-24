import Foundation
import os
import XCTest
@testable import Shisho

/// Answers a File's download from what the test set for its path.
private final class StubDownloads: URLProtocol {
    struct Reply: Sendable {
        var mimeType = "application/epub+zip"
        var body: Data
        /// The length the response claims, when it isn't the body's.
        var claimedLength: Int?
    }

    private struct State {
        var replies: [String: Reply] = [:]
        var counts: [String: Int] = [:]
    }

    private static let state = OSAllocatedUnfairLock(initialState: State())

    static func reply(_ path: String, _ reply: Reply) {
        state.withLock { $0.replies[path] = reply }
    }

    static func requests(_ path: String) -> Int {
        state.withLock { $0.counts[path] ?? 0 }
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let path = request.url?.path ?? ""
        let reply = Self.state.withLock { state in
            state.counts[path, default: 0] += 1
            return state.replies[path]
        }
        guard let reply, let url = request.url else {
            client?.urlProtocol(self, didFailWithError: URLError(.cannotConnectToHost))
            return
        }
        let response = HTTPURLResponse(url: url, statusCode: 200, httpVersion: "HTTP/1.1", headerFields: [
            "Content-Type": reply.mimeType,
            "Content-Length": String(reply.claimedLength ?? reply.body.count),
        ])!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: reply.body)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}

/// Fetching an ebook to read (ADR 0022): what isn't the File is refused,
/// and a kept copy is used until the File changes or won't open.
final class EbookCacheTests: XCTestCase {
    private let account = UUID().uuidString
    private var client: APIClient!

    override func setUp() {
        client = APIClient(baseURL: URL(string: "http://shisho.test")!, credential: nil, protocolClasses: [StubDownloads.self])
    }

    override func tearDown() {
        let folder = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("Ebooks/\(account)", isDirectory: true)
        try? FileManager.default.removeItem(at: folder)
    }

    private func file(_ id: Int, size: Int) -> ReadingFile {
        ReadingFile(fileID: id, book: BookSummary(id: 1, libraryID: 1, title: "Book", authors: "", coverVersion: "", coverAspectRatio: "book",
                                                  fileTypes: ["epub"]),
                    fileType: "epub", displayName: "", sizeBytes: size, chapters: [])
    }

    /// A login page from a proxy in front of the server isn't the book.
    func testALoginPageIsRefused() async {
        StubDownloads.reply("/mobile/files/101/download", .init(mimeType: "text/html", body: Data("<html>Sign in</html>".utf8)))
        do {
            _ = try await client.download(fileID: 101)
            XCTFail("a login page was taken for the book")
        } catch APIError.blockedByProxy {
        } catch {
            XCTFail("unexpected error: \(error)")
        }
    }

    /// A download cut off partway isn't the book either.
    func testADownloadCutShortIsRefused() async {
        StubDownloads.reply("/mobile/files/102/download", .init(body: Data(repeating: 1, count: 10), claimedLength: 20))
        do {
            _ = try await client.download(fileID: 102)
            XCTFail("a short download was taken for the book")
        } catch APIError.unreachable {
        } catch {
            XCTFail("unexpected error: \(error)")
        }
    }

    /// The kept copy is used again; a File whose size changed, or one
    /// discarded because it wouldn't open, is fetched again.
    func testTheKeptCopyIsUsedUntilTheFileChangesOrIsDiscarded() async throws {
        let path = "/mobile/files/103/download"
        let cache = EbookCache(client: client, accountID: account)
        StubDownloads.reply(path, .init(body: Data("first".utf8)))
        let first = try await cache.file(file(103, size: 5))
        _ = try await cache.file(file(103, size: 5))
        XCTAssertEqual(StubDownloads.requests(path), 1)
        XCTAssertEqual(try String(contentsOf: first, encoding: .utf8), "first")

        StubDownloads.reply(path, .init(body: Data("second!".utf8)))
        let changed = try await cache.file(file(103, size: 7))
        XCTAssertEqual(StubDownloads.requests(path), 2)
        XCTAssertEqual(try String(contentsOf: changed, encoding: .utf8), "second!")
        XCTAssertFalse(FileManager.default.fileExists(atPath: first.path), "the older copy is gone")

        cache.discard(file(103, size: 7))
        _ = try await cache.file(file(103, size: 7))
        XCTAssertEqual(StubDownloads.requests(path), 3)
    }
}
