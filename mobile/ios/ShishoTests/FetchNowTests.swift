import GRDB
import ShishoAPI
import XCTest
@testable import Shisho

/// A server that starts sending a File and never finishes, as a large
/// book does on a slow connection.
private final class StallingServer: URLProtocol {
    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let response = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: "HTTP/1.1",
                                       headerFields: ["Content-Length": "1000000"])!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(count: 1000))
    }

    override func stopLoading() {}
}

/// Opening a book that isn't downloaded downloads it in front of the
/// reader (`DownloadManager.fetchNow`).
@MainActor
final class FetchNowTests: XCTestCase {
    func testClosingTheReaderBeforeTheFileArrivesLeavesItDownloading() async throws {
        let accountID = UUID().uuidString
        let account = Account(id: accountID, serverURL: URL(string: "https://shisho.test")!, username: "u", userID: 1, deviceID: "d",
                              serverVersion: "0.10.0", thumbnailRenderKey: "", capabilities: [], signedOut: false, instanceID: "i")
        defer { AccountData(accountID: accountID).remove() }
        let store = try CatalogStore(path: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path)
        try store.startOverIfNeeded()
        try store.apply(try makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data("""
            {"entries":[{"kind":"library","id":1,"library_id":1,"library":{"id":1,"name":"Comics","cover_aspect_ratio":"book"}},
             {"kind":"book","id":1,"library_id":1,"book":{"id":1,"library_id":1,"title":"Big","sort_title":"Big",
              "created_at":"2026-01-01T00:00:00Z","files":[{"id":10,"file_type":"cbz","file_role":"main","size_bytes":1000000,
              "display_name":"Big"}],"cover_version":"1"}}],"cursor":2}
            """.utf8)))
        let client = APIClient(baseURL: account.serverURL, credential: "c", instanceID: "i", onCapabilities: { _ in },
                               protocolClasses: [StallingServer.self])
        let downloads = DownloadManager(account: account, client: client, store: store)
        defer { downloads.stop() }

        let open = Task { try await downloads.fetchNow(fileID: 10) }
        // The transfer is under way when the reader closes.
        try await Task.sleep(for: .milliseconds(300))
        open.cancel()
        _ = try? await open.value

        let row = try await store.pool.read { try CatalogStore.download($0, fileID: 10) }
        XCTAssertEqual(row?.state, .queued, "it carries on as a Download, so the wait isn't lost")
    }
}
