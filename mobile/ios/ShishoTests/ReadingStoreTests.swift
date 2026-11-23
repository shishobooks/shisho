import GRDB
import ShishoAPI
import XCTest
@testable import Shisho

/// Reading an ebook in one account's database (#919): CFI Reading
/// Positions and Reading Sessions go into the journal, the table of
/// contents comes from Shisho's chapters, and a File opens at the newest
/// place across devices.
final class ReadingStoreTests: XCTestCase {
    private var store: CatalogStore!
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    override func setUpWithError() throws {
        store = try CatalogStore(path: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path)
        try store.startOverIfNeeded()
        try store.apply(page("""
        {"entries":[{"kind":"library","id":1,"library_id":1,"library":{"id":1,"name":"Books","cover_aspect_ratio":"book"}},
         {"kind":"book","id":1,"library_id":1,"book":{"id":1,"library_id":1,"title":"The Harness Novel","sort_title":"Harness Novel",
          "created_at":"2026-01-01T00:00:00Z","cover_version":"1-1","files":[
           {"id":8,"file_type":"epub","file_role":"main","size_bytes":4096,"display_name":"Novel","chapters":[
            {"id":20,"title":"Part One","href":"part1.xhtml"},
            {"id":21,"parent_id":20,"title":"Chapter One","href":"chapter1.xhtml"},
            {"id":22,"parent_id":20,"title":"Chapter Two","href":"chapter2.xhtml#start"},
            {"id":23,"title":"Notes"}]},
           {"id":9,"file_type":"m4b","file_role":"main","size_bytes":10,"audiobook_duration_seconds":600,"chapters":[]}]}}],
         "cursor":2}
        """))
    }

    private func page(_ json: String) throws -> Shishoapp.FeedResponse {
        try makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data(json.utf8))
    }

    private func journal() throws -> [Row] {
        try store.pool.read { try Row.fetchAll($0, sql: "SELECT type, payload, held_for FROM journal ORDER BY seq") }
    }

    private func payload(_ row: Row) throws -> [String: Any] {
        try XCTUnwrap(JSONSerialization.jsonObject(with: row["payload"] as Data) as? [String: Any])
    }

    private func session(_ id: String, duration: Int, end: TimeInterval) -> FinishedSession {
        FinishedSession(sittingID: id, fileID: 8, startedAt: now, endedAt: now.addingTimeInterval(end),
                        startPercentage: 0.1, endPercentage: 0.2, duration: duration)
    }

    // MARK: - Recording

    /// A turn saves the CFI and the session so far together; later turns of
    /// the same session replace both until they're sent.
    func testTurnsSaveTheCFIAndTheSessionSoFar() throws {
        try store.recordReading(fileID: 8, cfi: "epubcfi(/6/4!/4/2)", percentage: 0.1, atEnd: false, at: now, sessionID: "s1",
                                sessionStartedAt: now, session: session("s1", duration: 6, end: 6))
        try store.recordReading(fileID: 8, cfi: "epubcfi(/6/6!/4/2)", percentage: 0.2, atEnd: false, at: now.addingTimeInterval(20),
                                sessionID: "s1", sessionStartedAt: now, session: session("s1", duration: 20, end: 20))

        let rows = try journal()
        XCTAssertEqual(rows.map { $0["type"] as String }, ["reading_position", "reading_session"])
        let position = try payload(rows[0])
        XCTAssertEqual(position["file_id"] as? Int, 8)
        XCTAssertEqual(position["cfi"] as? String, "epubcfi(/6/6!/4/2)")
        XCTAssertEqual(position["percentage"] as? Double, 0.2)
        XCTAssertNil(position["seconds"])
        XCTAssertNil(position["listened_seconds"])
        // When the session began, so a Read-through it starts begins then.
        XCTAssertEqual(position["listening_since"] as? String, JournalTime.string(now))
        let sent = try payload(rows[1])
        XCTAssertEqual(sent["session_id"] as? String, "s1")
        XCTAssertEqual(sent["duration"] as? Int, 20)

        let local = try XCTUnwrap(store.pool.read { try CatalogStore.localPlace($0, fileID: 8) })
        XCTAssertEqual(local.cfi, "epubcfi(/6/6!/4/2)")
        XCTAssertEqual(local.percentage, 0.2)
        XCTAssertTrue(local.pending)
    }

    /// Reaching the end is always sent, never replaced by a later move.
    func testReachingTheEndIsKept() throws {
        try store.recordReading(fileID: 8, cfi: "epubcfi(/6/20!/4/2)", percentage: 1, atEnd: true, at: now,
                                sessionID: "s", sessionStartedAt: now, session: nil)
        try store.recordReading(fileID: 8, cfi: "epubcfi(/6/18!/4/2)", percentage: 0.95, atEnd: false, at: now.addingTimeInterval(5),
                                sessionID: "s", sessionStartedAt: now, session: nil)
        let positions = try journal().filter { $0["type"] == "reading_position" }
        XCTAssertEqual(try positions.map { try payload($0)["at_end"] as? Bool ?? false }, [true, false])
    }

    /// Every end is sent, in order: the server finishes at the first and
    /// undoes it only for a move back soon after, so a later end can't
    /// stand in for an earlier one.
    func testEveryEndIsSent() throws {
        for (offset, cfi) in ["epubcfi(/6/20!/4/2)", "epubcfi(/6/18!/4/2)", "epubcfi(/6/20!/4/4)"].enumerated() {
            try store.recordReading(fileID: 8, cfi: cfi, percentage: offset == 1 ? 0.95 : 1, atEnd: offset != 1,
                                    at: now.addingTimeInterval(Double(offset) * 1200), sessionID: "s", sessionStartedAt: now, session: nil)
        }
        let positions = try journal().filter { $0["type"] == "reading_position" }
        XCTAssertEqual(try positions.map { try payload($0)["cfi"] as? String },
                       ["epubcfi(/6/20!/4/2)", "epubcfi(/6/18!/4/2)", "epubcfi(/6/20!/4/4)"])
    }

    /// Turns of two sessions are sent apart, even before either counted
    /// time, so the first turn that starts a Read-through isn't replaced by
    /// a later day's.
    func testTurnsOfAnotherSessionAreKept() throws {
        try store.recordReading(fileID: 8, cfi: "epubcfi(/6/4!/4/2)", percentage: 0.1, atEnd: false, at: now, sessionID: "a",
                                sessionStartedAt: now, session: nil)
        try store.recordReading(fileID: 8, cfi: "epubcfi(/6/6!/4/2)", percentage: 0.2, atEnd: false, at: now.addingTimeInterval(86400),
                                sessionID: "b", sessionStartedAt: now.addingTimeInterval(86400), session: nil)
        XCTAssertEqual(try journal().count, 2)
    }

    /// Staying where the reader is, over another device's place, keeps it
    /// for the next open even with no turn of this device's to send.
    func testStayingKeepsThePlaceWithoutSendingIt() throws {
        try serverPosition(cfi: "epubcfi(/6/14!/4/2)", percentage: 0.6, at: "2026-05-02T10:00:00Z", device: "tablet")
        try store.stayHere(fileID: 8, cfi: "epubcfi(/6/4!/4/2)", percentage: 0.1, at: now)
        let choice = try store.pool.read { try CatalogStore.ebookOpening($0, fileID: 8, ownDeviceID: "me") }
        XCTAssertEqual(choice.start, .cfi("epubcfi(/6/4!/4/2)"))
        XCTAssertNil(choice.offer)
        XCTAssertEqual(try journal().count, 0)
    }

    /// Staying replaces an older place of this device's, so the next open
    /// isn't sent back to it.
    func testStayingReplacesAnOlderPlaceHere() throws {
        try store.recordReading(fileID: 8, cfi: "epubcfi(/6/4!/4/2)", percentage: 0.1, atEnd: false, at: now,
                                sessionID: "s", sessionStartedAt: now, session: nil)
        try store.pool.write { try $0.execute(sql: "DELETE FROM journal") }
        try serverPosition(cfi: "epubcfi(/6/10!/4/2)", percentage: 0.4, at: "2030-01-01T10:00:00Z", device: "tablet")
        try serverPosition(cfi: "epubcfi(/6/14!/4/2)", percentage: 0.6, at: "2030-01-02T10:00:00Z", device: "laptop")
        try store.stayHere(fileID: 8, cfi: "epubcfi(/6/10!/4/2)", percentage: 0.4, at: now)
        let choice = try store.pool.read { try CatalogStore.ebookOpening($0, fileID: 8, ownDeviceID: "me") }
        XCTAssertEqual(choice.start, .cfi("epubcfi(/6/10!/4/2)"))
        XCTAssertNil(choice.offer)
    }

    /// A session ending on its own, as when the reader closes, is queued
    /// with no move.
    func testASessionCanBeSavedAlone() throws {
        try store.recordReading(session: session("s2", duration: 30, end: 40))
        let rows = try journal()
        XCTAssertEqual(rows.map { $0["type"] as String }, ["reading_session"])
    }

    // MARK: - Where it opens

    private func serverPosition(cfi: String?, percentage: Double, at: String, device: String?) throws {
        let cfiJSON = cfi.map { "\"\($0)\"" } ?? "null"
        let deviceJSON = device.map { "\"\($0)\"" } ?? "null"
        try store.apply(page("""
        {"entries":[{"kind":"reading_position","id":8,"reading_position":{"file_id":8,"cfi":\(cfiJSON),"percentage":\(percentage),
          "occurred_at":"\(at)","source":"web_reader","device_id":\(deviceJSON)}}],"cursor":\(Int.random(in: 3...1000))}
        """), ownDeviceID: "me")
    }

    /// With nothing moved here, a File opens at the server's place.
    func testItOpensAtTheServersPlace() throws {
        try serverPosition(cfi: "epubcfi(/6/8!/4/2)", percentage: 0.4, at: "2026-05-01T10:00:00Z", device: nil)
        let choice = try store.pool.read { try CatalogStore.ebookOpening($0, fileID: 8, ownDeviceID: "me") }
        XCTAssertEqual(choice.start, .cfi("epubcfi(/6/8!/4/2)"))
        XCTAssertNil(choice.offer)
    }

    /// A place another reader left only as a percentage opens there.
    func testAPercentageOnlyPlaceOpensAtItsPercentage() throws {
        try serverPosition(cfi: nil, percentage: 0.25, at: "2026-05-01T10:00:00Z", device: "kobo")
        let choice = try store.pool.read { try CatalogStore.ebookOpening($0, fileID: 8, ownDeviceID: "me") }
        XCTAssertEqual(choice.start, .fraction(0.25))
        XCTAssertEqual(choice.percentage, 0.25)
    }

    func testANewFileOpensAtTheStart() throws {
        let choice = try store.pool.read { try CatalogStore.ebookOpening($0, fileID: 8, ownDeviceID: "me") }
        XCTAssertNil(choice.start)
    }

    /// Moves here not yet sent win, and another device's newer place is
    /// offered instead of jumped to.
    func testBothMovedOffersTheOtherPlace() throws {
        try serverPosition(cfi: "epubcfi(/6/4!/4/2)", percentage: 0.1, at: "2026-05-01T10:00:00Z", device: nil)
        try store.recordReading(fileID: 8, cfi: "epubcfi(/6/6!/4/2)", percentage: 0.2, atEnd: false, at: now,
                                sessionID: "s", sessionStartedAt: now, session: nil)
        try serverPosition(cfi: "epubcfi(/6/14!/4/2)", percentage: 0.6, at: "2026-05-02T10:00:00Z", device: "tablet")

        let choice = try store.pool.read { try CatalogStore.ebookOpening($0, fileID: 8, ownDeviceID: "me") }
        XCTAssertEqual(choice.start, .cfi("epubcfi(/6/6!/4/2)"))
        XCTAssertEqual(choice.offer?.cfi, "epubcfi(/6/14!/4/2)")
        XCTAssertEqual(choice.offer?.percentage, 0.6)
    }

    /// Two places on the same page, laid out on different screens, aren't
    /// offered as different.
    func testNearbyPlacesAreTheSame() {
        let place = ServerPlace(seconds: 0, occurredAt: now, deviceID: "tablet", source: "shisho_app", cfi: "epubcfi(/6/6!/4/8)",
                                percentage: 0.2015)
        XCTAssertNil(PlaceRules.offerWhileReading(server: place, at: (cfi: "epubcfi(/6/6!/4/2)", percentage: 0.2), known: nil,
                                                  ownDeviceID: "me"))
        let far = ServerPlace(seconds: 0, occurredAt: now, deviceID: "tablet", source: "shisho_app", cfi: "epubcfi(/6/14!/4/2)",
                              percentage: 0.6)
        XCTAssertEqual(PlaceRules.offerWhileReading(server: far, at: (cfi: "epubcfi(/6/6!/4/2)", percentage: 0.2), known: nil,
                                                    ownDeviceID: "me"), far)
    }

    // MARK: - Contents

    /// Shisho's chapters, nested under their parents, are the table of
    /// contents for an EPUB, so chapter edits show up in the reader.
    func testTheContentsAreShishosChapters() throws {
        let file = try XCTUnwrap(store.pool.read { try CatalogStore.readingFile($0, fileID: 8) })
        XCTAssertEqual(file.fileType, "epub")
        XCTAssertEqual(file.book.title, "The Harness Novel")
        XCTAssertEqual(file.chapters, [
            ReaderChapter(title: "Part One", href: "part1.xhtml", depth: 0),
            ReaderChapter(title: "Chapter One", href: "chapter1.xhtml", depth: 1),
            ReaderChapter(title: "Chapter Two", href: "chapter2.xhtml#start", depth: 1),
            ReaderChapter(title: "Notes", href: nil, depth: 0),
        ])
        XCTAssertTrue(file.usesShishoChapters)
    }

    /// An audiobook isn't read.
    func testOnlyReflowableFilesAreRead() throws {
        XCTAssertNil(try store.pool.read { try CatalogStore.readingFile($0, fileID: 9) })
    }

    /// The Book page's Read card: the ebook read most recently, and how far
    /// through it the reader is.
    func testTheBookPageShowsHowFarThroughTheEbook() throws {
        var progress = try XCTUnwrap(store.pool.read { try CatalogStore.readProgress($0, bookID: 1, ownDeviceID: "me") })
        XCTAssertEqual(progress.fileID, 8)
        XCTAssertEqual(progress.percentage, 0)
        try store.recordReading(fileID: 8, cfi: "epubcfi(/6/6!/4/2)", percentage: 0.35, atEnd: false, at: now,
                                sessionID: "s", sessionStartedAt: now, session: nil)
        progress = try XCTUnwrap(store.pool.read { try CatalogStore.readProgress($0, bookID: 1, ownDeviceID: "me") })
        XCTAssertEqual(progress.percentage, 0.35)
    }
}
