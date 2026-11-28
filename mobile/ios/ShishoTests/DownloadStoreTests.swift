import GRDB
import ShishoAPI
import XCTest
@testable import Shisho

extension CatalogStore {
    /// Records a finished transfer's row, without its file, as
    /// `DownloadCopies.publish` does.
    @discardableResult
    func markDownloaded(fileID: Int, sizeBytes: Int) throws -> UUID? {
        try pool.write { try CatalogStore.markDownloaded($0, fileID: fileID, sizeBytes: sizeBytes) }
    }
}

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

        XCTAssertNotNil(try store.markDownloaded(fileID: 10, sizeBytes: 987))
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
        XCTAssertNil(try store.markDownloaded(fileID: 10, sizeBytes: 5))
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
        XCTAssertNotNil(try store.markDownloaded(fileID: 10, sizeBytes: 5))
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
        XCTAssertNotNil(try store.markDownloaded(fileID: 10, sizeBytes: 987))
        XCTAssertNotNil(try store.markDownloaded(fileID: 11, sizeBytes: 1100))
        XCTAssertEqual(try store.pool.read(CatalogStore.replacedDownloads), [], "the catalog's size is the one it was queued at")

        try store.apply(page(#"{"entries":[\#(book(1, files: [(10, "m4b", "main"), (11, "epub", "main"), (12, "pdf", "supplement")], sizes: [10: 5000]))],"cursor":5}"#))
        XCTAssertEqual(try store.pool.read(CatalogStore.replacedDownloads), [10])

        XCTAssertEqual(try store.requeueReplaced(), [10])
        let row = try XCTUnwrap(store.pool.read { try CatalogStore.download($0, fileID: 10) })
        XCTAssertEqual(row.state, .queued)
        XCTAssertEqual(row.sizeBytes, 5000)
        XCTAssertEqual(try store.pool.read(CatalogStore.replacedDownloads), [], "queued at its new size")
        XCTAssertNotNil(try store.markDownloaded(fileID: 10, sizeBytes: 4999))
        XCTAssertEqual(try store.pool.read(CatalogStore.replacedDownloads), [])
    }

    func testASeriesDownloadsItsBooksFilesItKnows() throws {
        let files = try store.pool.read { try CatalogStore.downloadableFiles($0, seriesID: 1) }
        XCTAssertEqual(files, [10, 11, 12, 20], "supplements too, after their Book's main Files, and no File Type this version doesn't know")
        XCTAssertEqual(try store.pool.read { try CatalogStore.notDownloaded($0, fileIDs: files) }, 4)
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
        XCTAssertNotNil(try store.markDownloaded(fileID: 10, sizeBytes: 900))
        let stored = try store.pool.read(CatalogStore.storedDownloads)
        XCTAssertEqual(stored.map(\.fileID).sorted(), [10, 30])
        let audiobook = try XCTUnwrap(stored.first { $0.fileID == 10 })
        XCTAssertEqual(audiobook.book.title, "Book 1")
        XCTAssertEqual(audiobook.label, "File 10")
        XCTAssertEqual(audiobook.state, .done)
        XCTAssertEqual(DownloadManager.totalSize(stored), 900, "only finished Downloads take space")
    }
}

extension DownloadStoreTests {
    /// The Library's Downloaded filter and dimming, and the tab's badge,
    /// count only finished Downloads, and the badge only those on their way.
    func testWhatsDownloadedAndWhatsOnItsWay() throws {
        try store.queueDownloads([10, 20])
        XCTAssertNotNil(try store.markDownloaded(fileID: 20, sizeBytes: 5))
        XCTAssertEqual(try store.pool.read(CatalogStore.downloadedBookIDs), [2])
        XCTAssertEqual(try store.pool.read(CatalogStore.downloadingCount), 1)
        let filtered = try store.pool.read { try CatalogStore.books($0, libraryID: nil, filter: LibraryFilter(downloaded: true)) }
        XCTAssertEqual(filtered.map(\.id), [2])
    }

    /// Each Download's row says how far the user is: this device's place
    /// before the server's.
    func testTheDownloadsTabShowsProgress() throws {
        try store.queueDownloads([10, 11, 30])
        try store.apply(page(#"{"entries":[{"kind":"reading_position","id":11,"reading_position":{"file_id":11,"percentage":0.4,"occurred_at":"2026-01-01T00:00:00Z","source":"web_reader"}}],"cursor":4}"#))
        try store.recordPosition(fileID: 10, seconds: 0, percentage: 0.25, at: Date(), atEnd: false, sitting: nil)
        let stored = try store.pool.read(CatalogStore.storedDownloads)
        XCTAssertEqual(stored.first { $0.fileID == 10 }?.progress, 0.25)
        XCTAssertEqual(stored.first { $0.fileID == 11 }?.progress, 0.4)
        XCTAssertNil(stored.first { $0.fileID == 30 }?.progress)
    }
}

final class DownloadRulesTests: XCTestCase {
    /// The reader waits for a Download already transferring rather than
    /// start a second, but not one that can't move now, as one waiting for
    /// Wi-Fi: the reader downloads on any network.
    func testTheReaderJoinsOnlyATransferThatCanMove() {
        XCTAssertTrue(DownloadRules.joinsTransfer(running: true, wait: nil))
        XCTAssertFalse(DownloadRules.joinsTransfer(running: true, wait: .wifi))
        XCTAssertFalse(DownloadRules.joinsTransfer(running: true, wait: .connection))
        XCTAssertFalse(DownloadRules.joinsTransfer(running: false, wait: nil))
    }

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

final class DownloadWordsTests: XCTestCase {
    private func item(label: String = "Book", fileType: String = "m4b", status: String? = nil, progress: Double?) -> StoredDownload {
        StoredDownload(fileID: 1, book: BookSummary(id: 1, libraryID: 1, title: "Book", authors: "", coverVersion: "", coverAspectRatio: "book",
                                                    fileTypes: [fileType], status: status),
                       fileType: fileType, label: label, state: .done, sizeBytes: 1, error: nil, progress: progress)
    }

    /// A row names the File only when it isn't the Book's title, then its
    /// type and how far the user is, to help choose what to remove.
    func testARowSaysHowFarTheUserIs() {
        XCTAssertEqual(DownloadWords.facts(item(progress: nil)), "M4B · Not started")
        XCTAssertEqual(DownloadWords.facts(item(progress: 0.42)), "M4B · Listening \(0.42.formatted(.percent.precision(.fractionLength(0))))")
        XCTAssertEqual(DownloadWords.facts(item(label: "Unabridged", fileType: "epub", progress: 0.1)),
                       "Unabridged · EPUB · Reading \(0.1.formatted(.percent.precision(.fractionLength(0))))")
        XCTAssertEqual(DownloadWords.facts(item(progress: 0.999)), "M4B · Finished")
        XCTAssertEqual(DownloadWords.facts(item(status: "read", progress: nil)), "M4B · Finished", "marked read")
    }

    /// The stop button says how far a Download has got, or why it waits.
    func testTheStopButtonsValue() {
        XCTAssertEqual(DownloadWords.progress(.running(0.42)), 0.42.formatted(.percent.precision(.fractionLength(0))))
        XCTAssertEqual(DownloadWords.progress(.waiting(.wifi)), "Waiting for Wi-Fi")
        XCTAssertEqual(DownloadWords.progress(.running(nil)), "Downloading")
    }
}

/// The manager's rules for Downloads already on the device: Remove when
/// finished, which the Maestro flow reaches only through the player, and a
/// File the server replaced.
@MainActor
final class DownloadManagerTests: XCTestCase {
    private var session: AccountSession!
    private var removeWhenFinished = false

    override func setUp() async throws {
        removeWhenFinished = DownloadSettings.removeWhenFinished
        let missing = FileManager.default.temporaryDirectory.appendingPathComponent("no-server-\(UUID().uuidString)", isDirectory: true)
        let account = Account(id: "test-\(UUID().uuidString)", serverURL: missing, username: "u", userID: 1,
                              deviceID: "me", serverVersion: "dev", thumbnailRenderKey: "", capabilities: [], signedOut: false,
                              instanceID: "")
        session = try AccountSession(account: account, credential: "credential") { _ in }
        try session.store.startOverIfNeeded()
        try session.store.apply(makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data("""
        {"entries":[{"kind":"library","id":1,"library_id":1,"library":{"id":1,"name":"Fiction","cover_aspect_ratio":"book"}},
         {"kind":"book","id":1,"library_id":1,"book":{"id":1,"library_id":1,"title":"Book","sort_title":"Book","created_at":"2026-01-01T00:00:00Z",
          "cover_version":"1-1","files":[{"id":10,"file_type":"m4b","file_role":"main","size_bytes":3},{"id":11,"file_type":"epub","file_role":"main","size_bytes":3},
           {"id":12,"file_type":"epub","file_role":"supplement","size_bytes":3}]}}],
         "cursor":2}
        """.utf8)))
    }

    override func tearDown() async throws {
        DownloadSettings.removeWhenFinished = removeWhenFinished
        session.downloads.stop()
        AccountData(accountID: session.account.id).remove()
    }

    /// A finished Download on the device, as a transfer leaves it.
    private func downloaded(_ fileID: Int, _ fileType: String) throws -> URL {
        try session.store.queueDownloads([fileID])
        XCTAssertNotNil(try session.downloads.copies.publish(fileID: fileID, from: transferred("abc")))
        return session.downloads.location(fileID: fileID, fileType: fileType)
    }

    func testAFinishedFileIsRemovedOnlyWithTheSettingOn() throws {
        let audiobook = try downloaded(10, "m4b")
        session.downloads.removeWhenFinished = false
        session.downloads.finished(fileID: 10)
        XCTAssertNotNil(session.downloads.localFile(fileID: 10), "kept with the setting off")

        session.downloads.removeWhenFinished = true
        session.downloads.finished(fileID: 10)
        XCTAssertNil(session.downloads.localFile(fileID: 10))
        XCTAssertFalse(FileManager.default.fileExists(atPath: audiobook.path))
        XCTAssertNil(try session.store.pool.read { try CatalogStore.download($0, fileID: 10) })
    }

    func testABookFinishedInTheReaderGoesWhenTheReaderLetsGo() throws {
        let ebook = try downloaded(11, "epub")
        session.downloads.removeWhenFinished = true
        session.downloads.finished(fileID: 11, holding: true)
        XCTAssertTrue(FileManager.default.fileExists(atPath: ebook.path), "the reader's pages still read it")

        session.downloads.released(fileID: 11)
        XCTAssertFalse(FileManager.default.fileExists(atPath: ebook.path))
        XCTAssertNil(try session.store.pool.read { try CatalogStore.download($0, fileID: 11) })
    }

    /// Peeking at the end and going back, which undoes an automatic finish,
    /// keeps the Download.
    func testGoingBackFromTheEndKeepsTheDownload() throws {
        let ebook = try downloaded(11, "epub")
        session.downloads.removeWhenFinished = true
        session.downloads.finished(fileID: 11, holding: true)
        session.downloads.unfinished(fileID: 11)
        session.downloads.released(fileID: 11)
        XCTAssertTrue(FileManager.default.fileExists(atPath: ebook.path))
        XCTAssertNotNil(session.downloads.localFile(fileID: 11))
    }

    /// A supplement never finishes a Read-through, so reaching its end
    /// doesn't remove it.
    func testASupplementIsntRemovedAtItsEnd() throws {
        _ = try downloaded(12, "epub")
        session.downloads.removeWhenFinished = true
        session.downloads.finished(fileID: 12)
        XCTAssertNotNil(session.downloads.localFile(fileID: 12))
    }

    /// A File the server replaced downloads again, and the copy on the
    /// device still plays and opens until the new one arrives, offline too.
    func testAReplacedFilesCopyStaysUsableUntilTheNewOneArrives() throws {
        _ = try downloaded(10, "m4b")
        try session.store.apply(makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data("""
        {"entries":[{"kind":"book","id":1,"library_id":1,"book":{"id":1,"library_id":1,"title":"Book","sort_title":"Book","created_at":"2026-01-01T00:00:00Z",
          "cover_version":"1-1","files":[{"id":10,"file_type":"m4b","file_role":"main","size_bytes":5000},{"id":11,"file_type":"epub","file_role":"main","size_bytes":3},
           {"id":12,"file_type":"epub","file_role":"supplement","size_bytes":3}]}}],"cursor":3}
        """.utf8)))
        XCTAssertEqual(try session.store.requeueReplaced(), [10])
        XCTAssertNotNil(session.downloads.localFile(fileID: 10))
        // Still on the device for the Library's filter and dimming.
        XCTAssertEqual(try session.store.pool.read(CatalogStore.downloadedBookIDs), [1])

        // Finished in the reader meanwhile, with Remove when finished on, it
        // goes when the reader lets go, replacement and all.
        session.downloads.removeWhenFinished = true
        session.downloads.finished(fileID: 10, holding: true)
        session.downloads.released(fileID: 10)
        XCTAssertNil(session.downloads.localFile(fileID: 10))
        XCTAssertNil(try session.store.pool.read { try CatalogStore.download($0, fileID: 10) })
    }

    /// A file a transfer, or an open's download, leaves, to put in a File's
    /// place.
    private func transferred(_ text: String) throws -> URL {
        let url = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try Data(text.utf8).write(to: url)
        return url
    }

    /// The copy an open downloads with `fetchNow`.
    private func fetched(_ fileID: Int, _ text: String = "old") throws -> UUID {
        let fetched = try session.downloads.copies.publishFetched(fileID: fileID, from: transferred(text))
        guard case .published(_, let copy) = fetched else { return try XCTUnwrap(nil, "not put in place: \(fetched)") }
        return copy
    }

    /// A copy a background transfer put in place while an open downloaded
    /// the same File is the one it opens, and it isn't written over.
    func testAnOpenUsesACopyATransferPutThereFirst() throws {
        try session.store.queueDownloads([11])
        XCTAssertNotNil(try session.downloads.copies.publish(fileID: 11, from: transferred("transfer")))
        let url = session.downloads.location(fileID: 11, fileType: "epub")
        XCTAssertEqual(try session.downloads.copies.publishFetched(fileID: 11, from: transferred("fetched")), .found(url))
        XCTAssertEqual(try contents(11), "transfer")
    }

    /// An open's download is put in place over a background transfer the
    /// server refused meanwhile, and over a finished one whose file is gone.
    func testAnOpenPublishesOverAFailedOrMissingCopy() throws {
        try session.store.queueDownloads([10, 11])
        try session.store.markDownloadFailed(fileID: 11, message: "refused")
        guard case .published = try session.downloads.copies.publishFetched(fileID: 11, from: transferred("fetched")) else {
            return XCTFail("a failed Download takes the open's copy")
        }
        XCTAssertEqual(try contents(11), "fetched")

        XCTAssertNotNil(try session.store.markDownloaded(fileID: 10, sizeBytes: 3))
        guard case .published = try session.downloads.copies.publishFetched(fileID: 10, from: transferred("fetched")) else {
            return XCTFail("a finished Download whose file is gone takes the open's copy")
        }
        XCTAssertEqual(try contents(10), "fetched")
    }

    func testAnOpenOfAFileNotInTheCatalogKeepsNothing() throws {
        XCTAssertEqual(try session.downloads.copies.publishFetched(fileID: 99, from: transferred("fetched")), .unknown)
        XCTAssertNil(try session.store.pool.read { try CatalogStore.download($0, fileID: 99) })
    }

    /// Rolls back the next transaction as it commits, as a full disk would.
    private final class CommitVeto: TransactionObserver, Sendable {
        struct Vetoed: Error {}
        func observes(eventsOfKind eventKind: DatabaseEventKind) -> Bool { true }
        func databaseDidChange(with event: DatabaseEvent) {}
        func databaseWillCommit() throws { throw Vetoed() }
        func databaseDidCommit(_ db: Database) {}
        func databaseDidRollback(_ db: Database) {}
    }

    /// A copy put in place whose row then fails to save gives its place back
    /// to the copy before, which the row still names.
    func testACopyWhoseRowDoesntSaveGivesItsPlaceBack() throws {
        let copy = try fetched(11)
        try replaceTheEbook()
        let veto = CommitVeto()
        try session.store.pool.writeWithoutTransaction { $0.add(transactionObserver: veto, extent: .nextTransaction) }
        XCTAssertThrowsError(try session.downloads.copies.publish(fileID: 11, from: transferred("new")))
        XCTAssertEqual(try contents(11), "old")
        XCTAssertEqual(try session.store.pool.read { try CatalogStore.download($0, fileID: 11) }?.copy, copy)
    }

    /// An open finding a transfer's copy in place leaves its Download as it
    /// is, a replacement that failed included.
    func testAnOpenFindingACopyLeavesItsDownloadAsItIs() throws {
        _ = try fetched(11)
        try replaceTheEbook()
        try session.store.markDownloadFailed(fileID: 11, message: "refused")
        let url = session.downloads.location(fileID: 11, fileType: "epub")
        XCTAssertEqual(try session.downloads.copies.publishFetched(fileID: 11, from: transferred("fetched")), .found(url))
        let row = try XCTUnwrap(session.store.pool.read { try CatalogStore.download($0, fileID: 11) })
        XCTAssertEqual(row.state, .failed)
        XCTAssertEqual(row.error, "refused")
    }

    /// A File that left the catalog while an open downloaded it gets no
    /// copy, even with its Download's row not removed yet.
    func testAnOpenOfAFileThatLeftTheCatalogKeepsNothing() throws {
        try session.store.queueDownloads([11])
        try session.store.apply(makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data("""
        {"entries":[{"kind":"book","id":1,"library_id":1,"book":{"id":1,"library_id":1,"title":"Book","sort_title":"Book","created_at":"2026-01-01T00:00:00Z",
          "cover_version":"1-1","files":[{"id":10,"file_type":"m4b","file_role":"main","size_bytes":3},
           {"id":12,"file_type":"epub","file_role":"supplement","size_bytes":3}]}}],"cursor":3}
        """.utf8)))
        XCTAssertEqual(try session.downloads.copies.publishFetched(fileID: 11, from: transferred("fetched")), .unknown)
        XCTAssertFalse(FileManager.default.fileExists(atPath: session.downloads.location(fileID: 11, fileType: "epub").path))
    }

    /// A copy that can't be put in place leaves the one there, and its row,
    /// as they were.
    func testACopyThatCantBePutInPlaceLeavesTheOldOne() throws {
        _ = try fetched(11)
        try replaceTheEbook()
        let missing = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        XCTAssertThrowsError(try session.downloads.copies.publish(fileID: 11, from: missing))
        XCTAssertEqual(try contents(11), "old")
        XCTAssertEqual(try session.store.pool.read { try CatalogStore.download($0, fileID: 11) }?.replacing, true)
    }

    /// The server replaces the ebook, File 11, which queues its Download
    /// again with its copy still in place.
    private func replaceTheEbook() throws {
        try session.store.apply(makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data("""
        {"entries":[{"kind":"book","id":1,"library_id":1,"book":{"id":1,"library_id":1,"title":"Book","sort_title":"Book","created_at":"2026-01-01T00:00:00Z",
          "cover_version":"1-1","files":[{"id":10,"file_type":"m4b","file_role":"main","size_bytes":3},{"id":11,"file_type":"epub","file_role":"main","size_bytes":5000},
           {"id":12,"file_type":"epub","file_role":"supplement","size_bytes":3}]}}],"cursor":3}
        """.utf8)))
        XCTAssertEqual(try session.store.requeueReplaced(), [11])
    }

    private func contents(_ fileID: Int) throws -> String? {
        try session.downloads.localFile(fileID: fileID).map { try String(contentsOf: $0, encoding: .utf8) }
    }

    /// A copy an open downloaded that won't open goes with its Download, so
    /// Try Again downloads it again.
    func testACopyAnOpenFetchedThatWontOpenGoes() throws {
        let copy = try fetched(11)
        XCTAssertEqual(try contents(11), "old")
        session.downloads.discard(fileID: 11, copy: copy)
        XCTAssertNil(session.downloads.localFile(fileID: 11))
        XCTAssertFalse(FileManager.default.fileExists(atPath: session.downloads.location(fileID: 11, fileType: "epub").path))
        XCTAssertNil(try session.store.pool.read { try CatalogStore.download($0, fileID: 11) })
    }

    /// A replacement a transfer put in the File's place after the open
    /// downloaded its copy, and before that copy failed to open, stays.
    func testANewerCopyPutThereMeanwhileStays() throws {
        let copy = try fetched(11)
        try replaceTheEbook()
        XCTAssertNotNil(try session.downloads.copies.publish(fileID: 11, from: transferred("new")))
        session.downloads.discard(fileID: 11, copy: copy)
        XCTAssertEqual(try contents(11), "new")
        XCTAssertEqual(try session.store.pool.read { try CatalogStore.download($0, fileID: 11) }?.state, .done)
        XCTAssertEqual(try FileManager.default.contentsOfDirectory(atPath: session.downloads.folder.path).sorted(), ["11.epub"],
                       "the copy it replaced isn't kept once its row is saved")
    }

    /// A Download removed and made again since has a copy of its own, which
    /// an older open's failure leaves.
    func testACopyMadeAgainAfterARemovalStays() throws {
        let copy = try fetched(11)
        session.downloads.remove([11])
        _ = try fetched(11, "again")
        session.downloads.discard(fileID: 11, copy: copy)
        XCTAssertEqual(try contents(11), "again")
    }

    /// A replacement still on its way when the copy fails to open keeps
    /// going: only the copy that won't open goes, and the new one is kept
    /// when it arrives.
    func testAReplacementOnItsWayKeepsGoing() throws {
        let copy = try fetched(11)
        try replaceTheEbook()
        // What the replacement's transfer carries on from, which stopping
        // the transfer drops.
        DownloadResume.save(Data("resume".utf8), fileID: 11, in: session.downloads.folder)
        session.downloads.discard(fileID: 11, copy: copy)
        XCTAssertNotNil(DownloadResume.take(fileID: 11, in: session.downloads.folder), "its transfer isn't stopped")
        XCTAssertNil(session.downloads.localFile(fileID: 11))
        XCTAssertFalse(FileManager.default.fileExists(atPath: session.downloads.location(fileID: 11, fileType: "epub").path))
        let row = try XCTUnwrap(session.store.pool.read { try CatalogStore.download($0, fileID: 11) }, "still wanted")
        XCTAssertEqual(row.state, .queued)
        XCTAssertFalse(row.replacing, "with no copy on the device meanwhile")
        XCTAssertEqual(try session.store.pool.read(CatalogStore.downloadedBookIDs), [])

        XCTAssertNotNil(try session.downloads.copies.publish(fileID: 11, from: transferred("new")))
        XCTAssertEqual(try contents(11), "new")
    }

    /// A Download that was never finished has no copy to use, whatever is in
    /// its place on disk.
    func testAQueuedDownloadHasNoCopyYet() throws {
        try session.store.queueDownloads([11])
        let url = session.downloads.location(fileID: 11, fileType: "epub")
        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data("abc".utf8).write(to: url)
        XCTAssertNil(session.downloads.localFile(fileID: 11))
    }
}
