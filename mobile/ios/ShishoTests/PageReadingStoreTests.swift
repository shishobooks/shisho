import GRDB
import ShishoAPI
import XCTest
@testable import Shisho

/// Reading a comic or a PDF in one account's database (#921): Reading
/// Positions are 0-based pages, the pages come in the server's order, and a
/// File opens at the newest page across devices.
final class PageReadingStoreTests: XCTestCase {
    private var store: CatalogStore!
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    override func setUpWithError() throws {
        store = try CatalogStore(path: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path)
        try store.startOverIfNeeded()
        // The pages aren't in name order: the server's order is the one kept.
        try store.apply(page("""
        {"entries":[{"kind":"library","id":1,"library_id":1,"library":{"id":1,"name":"Comics","cover_aspect_ratio":"book"}},
         {"kind":"book","id":1,"library_id":1,"book":{"id":1,"library_id":1,"title":"Harness Manga","sort_title":"Harness Manga",
          "created_at":"2026-01-01T00:00:00Z","cover_version":"1-1","files":[
           {"id":8,"file_type":"cbz","file_role":"main","size_bytes":4096,"display_name":"Manga","reading_direction":"rtl",
            "pages":["cover.jpg","page10.jpg","page2.jpg"],"chapters":[
            {"id":20,"title":"Chapter One","start_page":1},
            {"id":21,"title":"Chapter Two","start_page":2}]},
           {"id":9,"file_type":"pdf","file_role":"main","size_bytes":10,"display_name":"Manual","pages":[],"chapters":[]}]}}],
         "cursor":2}
        """))
    }

    private func page(_ json: String) throws -> Shishoapp.FeedResponse {
        try makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data(json.utf8))
    }

    private func serverPosition(page: Int?, percentage: Double, at: String, device: String?) throws {
        let pageJSON = page.map(String.init) ?? "null"
        let deviceJSON = device.map { "\"\($0)\"" } ?? "null"
        try store.apply(self.page("""
        {"entries":[{"kind":"reading_position","id":8,"reading_position":{"file_id":8,"page":\(pageJSON),"percentage":\(percentage),
          "occurred_at":"\(at)","source":"web_reader","device_id":\(deviceJSON)}}],"cursor":\(Int.random(in: 3...1000))}
        """), ownDeviceID: "me")
    }

    /// A CBZ is read with its pages as the server listed them, its Reading
    /// Direction, and chapters that start on pages.
    func testAComicIsReadInTheServersPageOrder() throws {
        let file = try XCTUnwrap(store.pool.read { try CatalogStore.readingFile($0, fileID: 8) })
        XCTAssertTrue(file.isPageBased)
        XCTAssertEqual(file.pages, ["cover.jpg", "page10.jpg", "page2.jpg"])
        XCTAssertTrue(file.rightToLeft)
        XCTAssertEqual(file.chapters.map(\.startPage), [1, 2])
        let pdf = try XCTUnwrap(store.pool.read { try CatalogStore.readingFile($0, fileID: 9) })
        XCTAssertTrue(pdf.isPageBased)
        XCTAssertFalse(pdf.rightToLeft)
    }

    /// A turn saves the page, sent as a page, and kept as this device's.
    func testATurnSavesThePage() throws {
        try store.recordReading(fileID: 8, cfi: nil, page: 1, percentage: 0.5, atEnd: false, at: now,
                                sessionID: "s", sessionStartedAt: now, session: nil)
        let payload = try store.pool.read { try Data.fetchOne($0, sql: "SELECT payload FROM journal") }
        let sent = try XCTUnwrap(JSONSerialization.jsonObject(with: XCTUnwrap(payload)) as? [String: Any])
        XCTAssertEqual(sent["page"] as? Int, 1)
        XCTAssertNil(sent["cfi"])
        let choice = try store.pool.read { try CatalogStore.ebookOpening($0, fileID: 8, ownDeviceID: "me") }
        XCTAssertEqual(choice.start, .page(1))
    }

    func testItOpensAtTheServersPage() throws {
        try serverPosition(page: 2, percentage: 1, at: "2026-05-01T10:00:00Z", device: nil)
        let choice = try store.pool.read { try CatalogStore.ebookOpening($0, fileID: 8, ownDeviceID: "me") }
        XCTAssertEqual(choice.start, .page(2))
    }

    /// Another device's page, newer than this one's unsent turn, is offered
    /// rather than gone to; the same page isn't.
    func testAnotherDevicesPageIsOffered() throws {
        try serverPosition(page: 0, percentage: 0, at: "2026-05-01T10:00:00Z", device: nil)
        try store.recordReading(fileID: 8, cfi: nil, page: 1, percentage: 0.5, atEnd: false, at: now,
                                sessionID: "s", sessionStartedAt: now, session: nil)
        try serverPosition(page: 2, percentage: 1, at: "2026-05-02T10:00:00Z", device: "tablet")
        let choice = try store.pool.read { try CatalogStore.ebookOpening($0, fileID: 8, ownDeviceID: "me") }
        XCTAssertEqual(choice.start, .page(1))
        XCTAssertEqual(choice.offer?.page, 2)
    }

    /// Pages are compared as pages: one page away is another place, however
    /// close its percentage in a long book.
    func testPagesAreTheSamePlaceOnlyOnTheSamePage() {
        let place = ServerPlace(seconds: 0, occurredAt: now, deviceID: "tablet", source: "shisho_app", percentage: 0.501, page: 501)
        XCTAssertEqual(PlaceRules.offerWhileReading(server: place, at: (cfi: nil, percentage: 0.5), page: 500, known: nil,
                                                    ownDeviceID: "me"), place)
        XCTAssertNil(PlaceRules.offerWhileReading(server: place, at: (cfi: nil, percentage: 0.6), page: 501, known: nil,
                                                  ownDeviceID: "me"))
    }

    /// Staying on this page, rather than going to another device's, keeps
    /// it for the next open.
    func testStayingKeepsThePage() throws {
        try serverPosition(page: 2, percentage: 1, at: "2026-05-02T10:00:00Z", device: "tablet")
        try store.stayHere(fileID: 8, cfi: nil, page: 1, percentage: 0.5, at: now)
        let choice = try store.pool.read { try CatalogStore.ebookOpening($0, fileID: 8, ownDeviceID: "me") }
        XCTAssertEqual(choice.start, .page(1))
    }

    /// The Read card covers a comic: its pages before it's read, then the
    /// pages left and the chapter, exactly, from wherever it was read.
    func testTheBookPageCountsAComicsPages() throws {
        var progress = try XCTUnwrap(store.pool.read { try CatalogStore.readProgress($0, bookID: 1, ownDeviceID: "me") })
        XCTAssertEqual(progress.ebookCount, 2)
        XCTAssertEqual(progress.pageCount, 3)
        XCTAssertEqual(progress.pagesLeft, 2)
        try serverPosition(page: 1, percentage: 0.5, at: "2026-05-01T10:00:00Z", device: nil)
        progress = try XCTUnwrap(store.pool.read { try CatalogStore.readProgress($0, bookID: 1, ownDeviceID: "me") })
        XCTAssertEqual(progress.pagesLeft, 1)
        XCTAssertEqual(progress.chapter, "Chapter One")
    }

    // MARK: - Page flow

    private func journalTypes() throws -> [String] {
        try store.pool.read { try String.fetchAll($0, sql: "SELECT type FROM journal ORDER BY seq") }
    }

    /// The flow the user chose for a Book comes with their record of it,
    /// from the web or another device.
    func testTheBooksFlowComesFromTheFeed() throws {
        XCTAssertNil(try store.pool.read { try CatalogStore.pageFlow($0, bookID: 1) })
        try store.apply(page("""
        {"entries":[{"kind":"user_book","id":1,"user_book":{"book_id":1,"page_flow":"scrolled"}}],"cursor":5}
        """))
        XCTAssertEqual(try store.pool.read { try CatalogStore.pageFlow($0, bookID: 1) }, .scrolled)
    }

    /// A flow chosen here shows at once and is queued for the server, one
    /// per Book; the feed doesn't undo it while it waits, and once the
    /// server answers, the feed's is the Book's.
    func testAFlowChosenHereWaitsForTheServer() throws {
        try store.choosePageFlow(.paginated, bookID: 1, at: now)
        try store.choosePageFlow(.scrolled, bookID: 1, at: now)
        XCTAssertEqual(try journalTypes(), ["page_flow"])
        let payload = try store.pool.read { try Data.fetchOne($0, sql: "SELECT payload FROM journal") }
        let sent = try XCTUnwrap(JSONSerialization.jsonObject(with: XCTUnwrap(payload)) as? [String: Any])
        XCTAssertEqual(sent["book_id"] as? Int, 1)
        XCTAssertEqual(sent["page_flow"] as? String, "scrolled")

        try store.apply(page("""
        {"entries":[{"kind":"user_book","id":1,"user_book":{"book_id":1,"page_flow":"paginated"}}],"cursor":6}
        """))
        XCTAssertEqual(try store.pool.read { try CatalogStore.pageFlow($0, bookID: 1) }, .scrolled)

        let rows = try store.pendingJournal(capabilities: "", limit: 10)
        try store.applyJournalResults([Shishoapp.JournalResult(id: rows[0].eventID, result: .accepted)], sent: rows, capabilities: "")
        XCTAssertEqual(try store.pool.read { try CatalogStore.pageFlow($0, bookID: 1) }, .paginated)
    }
}
