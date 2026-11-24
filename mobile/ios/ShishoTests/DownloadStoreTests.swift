import ShishoAPI
import XCTest
@testable import Shisho

/// Downloads' rows in the account's database (ADR 0022): what can be
/// queued, what a finished transfer records, and which go when their File
/// leaves the catalog.
final class DownloadStoreTests: XCTestCase {
    private var store: CatalogStore!

    override func setUpWithError() throws {
        store = try CatalogStore(path: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path)
        try store.startOverIfNeeded()
        try store.apply(page(#"{"entries":[\#(library),\#(book(1, files: [(10, "m4b", "main"), (11, "epub", "main"), (12, "pdf", "supplement")])),\#(book(2, files: [(20, "epub", "main"), (21, "cbr", "main")])),\#(book(3, files: [(30, "epub", "main")], series: false))],"cursor":3}"#))
    }

    private func page(_ json: String) throws -> Shishoapp.FeedResponse {
        try makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data(json.utf8))
    }

    private let library = #"{"kind":"library","id":1,"library_id":1,"library":{"id":1,"name":"Fiction","cover_aspect_ratio":"book"}}"#

    private func book(_ id: Int, files: [(Int, String, String)], series: Bool = true, sizes: [Int: Int] = [:]) -> String {
        let fileJSON = files.map {
            #"{"id":\#($0.0),"file_type":"\#($0.1)","file_role":"\#($0.2)","size_bytes":\#(sizes[$0.0] ?? $0.0 * 100),"display_name":"File \#($0.0)"}"#
        }.joined(separator: ",")
        let seriesJSON = series ? #"[{"series_id":1,"name":"Saga","sort_name":"Saga","number":\#(id)}]"# : "[]"
        return """
        {"kind":"book","id":\(id),"library_id":1,"book":{"id":\(id),"library_id":1,"title":"Book \(id)","sort_title":"Book \(id)",
         "created_at":"2026-01-01T00:00:00Z","series":\(seriesJSON),"files":[\(fileJSON)],"cover_version":"\(id)-1"}}
        """
    }

    private func rows() throws -> [DownloadRow] {
        try store.pool.read(CatalogStore.downloads)
    }

    func testQueuesOnlyFilesItCanDownloadOnce() throws {
        // A File Type this version doesn't know, and one not in the catalog,
        // aren't queued.
        XCTAssertEqual(try store.queueDownloads([10, 21, 99]), [10])
        XCTAssertEqual(try store.queueDownloads([10, 11]), [11], "one already queued isn't queued again")
        XCTAssertEqual(try rows().map(\.fileID), [10, 11])
        XCTAssertEqual(try rows().first?.state, .queued)
        XCTAssertEqual(try rows().first?.sizeBytes, 1000, "the catalog's size, until it's downloaded")

        XCTAssertTrue(try store.markDownloaded(fileID: 10, sizeBytes: 987))
        XCTAssertEqual(try store.pool.read { try CatalogStore.download($0, fileID: 10) }?.state, .done)
        XCTAssertEqual(try store.pool.read { try CatalogStore.download($0, fileID: 10) }?.sizeBytes, 987)
        XCTAssertEqual(try store.queueDownloads([10]), [], "a finished Download isn't downloaded again")

        try store.markDownloadFailed(fileID: 11, message: "gone")
        XCTAssertEqual(try store.pool.read { try CatalogStore.download($0, fileID: 11) }?.error, "gone")
        XCTAssertEqual(try store.queueDownloads([11]), [11], "a failed one is tried again")
        let retried = try XCTUnwrap(store.pool.read { try CatalogStore.download($0, fileID: 11) })
        XCTAssertEqual(retried.state, .queued)
        XCTAssertNil(retried.error)
    }

    func testAFinishedTransferForARemovedDownloadIsntKept() throws {
        try store.queueDownloads([10])
        try store.deleteDownloads([10])
        XCTAssertFalse(try store.markDownloaded(fileID: 10, sizeBytes: 5))
        XCTAssertEqual(try rows(), [])
    }

    /// A Book's update deletes and inserts its Files' rows again, which
    /// leaves its Downloads alone; a File gone from it, or the Book gone,
    /// orphans them.
    func testDownloadsWhoseFileLeftTheCatalogAreOrphaned() throws {
        try store.queueDownloads([10, 11, 20, 30])
        try store.apply(page(#"{"entries":[\#(book(1, files: [(10, "m4b", "main"), (12, "pdf", "supplement")]))],"cursor":5}"#))
        XCTAssertEqual(try store.pool.read(CatalogStore.orphanedDownloads), [11])

        try store.apply(page(#"{"entries":[{"kind":"book","id":2,"deleted":true,"library_id":1}],"cursor":6}"#))
        XCTAssertEqual(try store.pool.read(CatalogStore.orphanedDownloads).sorted(), [11, 20])

        // Losing the Library drops its Books.
        try store.reconcileAccess(Shishoapp.FeedAccess(libraryIds: [], listIds: []), force: true)
        XCTAssertEqual(try store.pool.read(CatalogStore.orphanedDownloads).sorted(), [10, 11, 20, 30])
    }

    /// A book read to its end goes once the reader lets go of it, so the
    /// mark is kept, and a finished Download whose file is gone downloads
    /// again.
    func testPendingRemovalsAndRequeues() throws {
        try store.queueDownloads([10, 11])
        XCTAssertTrue(try store.markDownloaded(fileID: 10, sizeBytes: 5))
        try store.markRemovePending(fileID: 11)
        XCTAssertEqual(try store.pool.read(CatalogStore.removePending), [], "only a finished Download")
        try store.markRemovePending(fileID: 10)
        XCTAssertEqual(try store.pool.read(CatalogStore.removePending), [10])

        try store.requeueDownloads([10])
        XCTAssertEqual(try store.pool.read { try CatalogStore.download($0, fileID: 10) }?.state, .queued)
        XCTAssertEqual(try store.queueDownloads([10]), [], "already queued")
    }

    /// A File replaced on the server, which the catalog sees as a new size,
    /// downloads again; one whose bytes on the device differ from the
    /// catalog's size, but whose catalog size hasn't changed, doesn't.
    func testAReplacedFileDownloadsAgain() throws {
        try store.queueDownloads([10, 11])
        XCTAssertTrue(try store.markDownloaded(fileID: 10, sizeBytes: 987))
        XCTAssertTrue(try store.markDownloaded(fileID: 11, sizeBytes: 1100))
        XCTAssertEqual(try store.pool.read(CatalogStore.replacedDownloads), [], "the catalog's size is the one it was queued at")

        try store.apply(page(#"{"entries":[\#(book(1, files: [(10, "m4b", "main"), (11, "epub", "main"), (12, "pdf", "supplement")], sizes: [10: 5000]))],"cursor":5}"#))
        XCTAssertEqual(try store.pool.read(CatalogStore.replacedDownloads), [10])

        XCTAssertEqual(try store.requeueReplaced(), [10])
        let row = try XCTUnwrap(store.pool.read { try CatalogStore.download($0, fileID: 10) })
        XCTAssertEqual(row.state, .queued)
        XCTAssertEqual(row.sizeBytes, 5000)
        XCTAssertEqual(try store.pool.read(CatalogStore.replacedDownloads), [], "queued at its new size")
        XCTAssertTrue(try store.markDownloaded(fileID: 10, sizeBytes: 4999))
        XCTAssertEqual(try store.pool.read(CatalogStore.replacedDownloads), [])
    }

    func testASeriesDownloadsItsBooksMainFilesItKnows() throws {
        let files = try store.pool.read { try CatalogStore.downloadableFiles($0, seriesID: 1) }
        XCTAssertEqual(files, [10, 11, 20], "no supplement, and no File Type this version doesn't know")
        XCTAssertEqual(try store.pool.read { try CatalogStore.notDownloaded($0, fileIDs: files) }, 3)
        try store.queueDownloads(files)
        XCTAssertEqual(try store.pool.read { try CatalogStore.notDownloaded($0, fileIDs: files) }, 0)
        try store.markDownloadFailed(fileID: 20, message: "gone")
        XCTAssertEqual(try store.pool.read { try CatalogStore.notDownloaded($0, fileIDs: files) }, 1)
    }

    func testAListDownloadsItsBooks() throws {
        try store.apply(page("""
        {"entries":[{"kind":"list","id":7,"list":{"id":7,"name":"Trip","is_ordered":true,"default_sort":"manual","owner":{"id":3,"username":"ann"},
         "permission":"viewer","books":[{"book_id":3,"sort_order":1,"added_at":"2026-01-02T00:00:00Z"}]}}],"cursor":8}
        """))
        XCTAssertEqual(try store.pool.read { try CatalogStore.downloadableFiles($0, listID: 7) }, [30])
    }

    func testTheDownloadsTabListsEachWithItsBook() throws {
        try store.queueDownloads([10, 30])
        XCTAssertTrue(try store.markDownloaded(fileID: 10, sizeBytes: 900))
        let stored = try store.pool.read(CatalogStore.storedDownloads)
        XCTAssertEqual(stored.map(\.fileID).sorted(), [10, 30])
        let audiobook = try XCTUnwrap(stored.first { $0.fileID == 10 })
        XCTAssertEqual(audiobook.book.title, "Book 1")
        XCTAssertEqual(audiobook.label, "File 10")
        XCTAssertEqual(audiobook.state, .done)
        XCTAssertEqual(DownloadManager.totalSize(stored), 900, "only finished Downloads take space")
    }
}

final class DownloadRulesTests: XCTestCase {
    func testWiFiOnlyWaitsOffWiFi() {
        XCTAssertNil(DownloadRules.wait(wifiOnly: false, network: .cellular))
        XCTAssertNil(DownloadRules.wait(wifiOnly: true, network: .wifi))
        XCTAssertEqual(DownloadRules.wait(wifiOnly: true, network: .cellular), .wifi)
        XCTAssertEqual(DownloadRules.wait(wifiOnly: false, network: .offline), .connection)
        XCTAssertEqual(DownloadRules.wait(wifiOnly: true, network: .offline), .connection)
    }

    func testStatus() {
        let queued = DownloadRow(fileID: 1, fileType: "m4b", state: .queued, sizeBytes: 10, error: nil)
        XCTAssertEqual(DownloadRules.status(nil, running: false, progress: nil, wait: nil), .notDownloaded)
        XCTAssertEqual(DownloadRules.status(queued, running: true, progress: 0.5, wait: nil), .running(0.5))
        XCTAssertEqual(DownloadRules.status(queued, running: false, progress: nil, wait: .wifi), .waiting(.wifi))
        XCTAssertEqual(DownloadRules.status(queued, running: true, progress: 0.5, wait: .wifi), .waiting(.wifi),
                       "a transfer iOS holds off cellular waits for Wi-Fi")
        XCTAssertEqual(DownloadRules.status(queued, running: false, progress: nil, wait: nil), .waiting(.turn))
        let done = DownloadRow(fileID: 1, fileType: "m4b", state: .done, sizeBytes: 10, error: nil)
        XCTAssertEqual(DownloadRules.status(done, running: false, progress: nil, wait: .wifi), .downloaded)
        let failed = DownloadRow(fileID: 1, fileType: "m4b", state: .failed, sizeBytes: 10, error: "gone")
        XCTAssertEqual(DownloadRules.status(failed, running: false, progress: nil, wait: nil), .failed("gone"))
    }

    /// A finished transfer is the File only when it's a success that wasn't
    /// cut off; a resumed one's response is the 206 for the rest.
    func testChecksAFinishedTransfer() throws {
        let file = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try Data("12345".utf8).write(to: file)
        defer { try? FileManager.default.removeItem(at: file) }
        let url = URL(string: "https://shisho.example.com/mobile/files/1/download")!
        func response(_ status: Int, _ headers: [String: String]) -> HTTPURLResponse {
            HTTPURLResponse(url: url, statusCode: status, httpVersion: "HTTP/1.1", headerFields: headers)!
        }

        XCTAssertNoThrow(try APIClient.checkDownloaded(file, response: response(200, ["Content-Length": "5", "Content-Type": "audio/mp4"])))
        XCTAssertNoThrow(try APIClient.checkDownloaded(file, response: response(206, ["Content-Length": "2", "Content-Type": "audio/mp4"])))
        XCTAssertThrowsError(try APIClient.checkDownloaded(file, response: response(200, ["Content-Length": "9", "Content-Type": "audio/mp4"]))) {
            XCTAssertEqual($0 as? APIError, .unreachable("The download stopped partway"))
        }
        XCTAssertThrowsError(try APIClient.checkDownloaded(file, response: response(200, ["Content-Length": "5", "Content-Type": "text/html"]))) {
            XCTAssertEqual($0 as? APIError, .blockedByProxy)
        }
        try Data(#"{"error":{"code":"not_found","message":"File not found"}}"#.utf8).write(to: file)
        XCTAssertThrowsError(try APIClient.checkDownloaded(file, response: response(404, ["Content-Type": "application/json"]))) {
            XCTAssertEqual($0 as? APIError, .server(status: 404, message: "File not found"))
        }
    }
}
