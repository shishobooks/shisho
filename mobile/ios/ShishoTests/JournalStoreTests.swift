import GRDB
import ShishoAPI
import XCTest
@testable import Shisho

/// The journal of changes made on the device (ADR 0024) and the user's own
/// records from the feed, in one account's database.
final class JournalStoreTests: XCTestCase {
    private var store: CatalogStore!
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    override func setUpWithError() throws {
        store = try CatalogStore(path: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path)
        try store.startOverIfNeeded()
        try store.apply(page("""
        {"entries":[{"kind":"library","id":1,"library_id":1,"library":{"id":1,"name":"Audio","cover_aspect_ratio":"audiobook"}},
         {"kind":"book","id":1,"library_id":1,"book":{"id":1,"library_id":1,"title":"Harbor Lights","sort_title":"Harbor Lights",
          "created_at":"2026-01-01T00:00:00Z","cover_version":"1-1","files":[{"id":5,"file_type":"m4b","file_role":"main","size_bytes":10,
          "audiobook_duration_seconds":600,"chapters":[{"id":10,"title":"One","start_timestamp_ms":0},
          {"id":12,"parent_id":11,"title":"Two, part one","start_timestamp_ms":300000},{"id":11,"title":"Two","start_timestamp_ms":300000}]}]}}],
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

    func testTheFeedCarriesChaptersPositionsAndReadThroughs() throws {
        let file = try XCTUnwrap(store.pool.read { try CatalogStore.listeningFile($0, fileID: 5) })
        XCTAssertEqual(file.duration, 600)
        XCTAssertEqual(file.chapters, [AudioChapter(title: "One", start: 0), AudioChapter(title: "Two", start: 300)])

        try store.apply(page("""
        {"entries":[{"kind":"reading_position","id":5,"reading_position":{"file_id":5,"seconds":120,"percentage":0.2,
          "occurred_at":"2026-05-01T10:00:00Z","source":"web_reader","device_id":null}},
         {"kind":"read_through","id":3,"read_through":{"id":3,"book_id":1,"begin_day":"2026-05-01","outcome":"underway","paused":false}}],
         "cursor":4}
        """))
        let place = try XCTUnwrap(store.pool.read { try CatalogStore.serverPlace($0, fileID: 5) })
        XCTAssertEqual(place.seconds, 120)
        XCTAssertEqual(place.source, "web_reader")
        let progress = try XCTUnwrap(store.pool.read { try CatalogStore.listenProgress($0, bookID: 1, ownDeviceID: "me") })
        XCTAssertEqual(progress.seconds, 120)
        XCTAssertTrue(progress.underway)

        try store.apply(page(#"{"entries":[{"kind":"reading_position","id":5,"deleted":true},{"kind":"read_through","id":3,"deleted":true}],"cursor":6}"#))
        XCTAssertNil(try store.pool.read { try CatalogStore.serverPlace($0, fileID: 5) })
        XCTAssertFalse(try XCTUnwrap(store.pool.read { try CatalogStore.listenProgress($0, bookID: 1, ownDeviceID: "me") }).underway)
    }

    /// Moves of one sitting replace each other until sent; reaching the end
    /// is always sent, and so is every session.
    func testMovesOfOneSittingReplaceEachOther() throws {
        let (sitting, _) = ListeningRules.play(nil, fileID: 5, at: now, percentage: 0)
        try store.recordPosition(fileID: 5, seconds: 5, percentage: 0.01, at: now.addingTimeInterval(5), atEnd: false, sitting: sitting)
        try store.recordPosition(fileID: 5, seconds: 10, percentage: 0.02, at: now.addingTimeInterval(10), atEnd: false, sitting: sitting)
        try store.recordPosition(fileID: 5, seconds: 600, percentage: 1, at: now.addingTimeInterval(20), atEnd: true, sitting: sitting)
        try store.saveSitting(nil, recording: FinishedSession(fileID: 5, startedAt: now, endedAt: now.addingTimeInterval(20),
                                                              startPercentage: 0, endPercentage: 1, duration: 20))

        let rows = try journal()
        XCTAssertEqual(rows.map { $0["type"] as String }, ["reading_position", "reading_position", "reading_session"])
        let first = try payload(rows[0])
        XCTAssertEqual(first["seconds"] as? Double, 10)
        XCTAssertEqual(first["listened_seconds"] as? Double, 10)
        XCTAssertNotNil(first["listening_since"])
        XCTAssertEqual(try payload(rows[1])["at_end"] as? Bool, true)
        XCTAssertEqual(try payload(rows[2])["duration"] as? Int, 20)

        let local = try XCTUnwrap(store.pool.read { try CatalogStore.localPlace($0, fileID: 5) })
        XCTAssertEqual(local.seconds, 600)
        XCTAssertTrue(local.pending)
    }

    /// The server's answer: taken in and repeated events are done, refused
    /// ones are dropped with a note, and ones it didn't understand wait until
    /// its capabilities change.
    func testApplyingTheServersResults() throws {
        for seconds in [5.0, 10.0, 15.0, 20.0] {
            // No sitting, a different one each time, so none replace another.
            try store.recordPosition(fileID: 5, seconds: seconds, percentage: 0.1, at: now, atEnd: true, sitting: nil)
        }
        let sent = try store.pendingJournal(capabilities: "", limit: 10)
        XCTAssertEqual(sent.count, 4)
        let results = zip(sent, ["accepted", "duplicate", "refused", "not_understood"]).map { row, result in
            Shishoapp.JournalResult(id: row.eventID, result: Shishoapp.JournalResultKind(rawValue: result),
                                    reason: result == "refused" ? "The File was deleted." : "")
        }
        try store.applyJournalResults(results, sent: sent, capabilities: "")

        XCTAssertEqual(try journal().count, 1)
        XCTAssertTrue(try store.pendingJournal(capabilities: "", limit: 10).isEmpty)
        XCTAssertEqual(try store.pendingJournal(capabilities: "bookmarks", limit: 10).map(\.eventID), [sent[3].eventID])
        let notes = try store.pool.read(CatalogStore.journalNotes)
        XCTAssertEqual(notes.map(\.reason), ["The File was deleted."])
        try store.dismissJournalNotes()
        XCTAssertTrue(try store.pool.read(CatalogStore.journalNotes).isEmpty)
    }

    /// The server's place when the device first moved since its last sync is
    /// kept, so a later move elsewhere can be told from one before.
    func testTheBaseIsTheServersPlaceWhenTheDeviceFirstMoved() throws {
        try store.apply(page("""
        {"entries":[{"kind":"reading_position","id":5,"reading_position":{"file_id":5,"seconds":120,"percentage":0.2,
          "occurred_at":"2026-05-01T10:00:00Z","source":"web_reader"}}],"cursor":4}
        """))
        try store.recordPosition(fileID: 5, seconds: 30, percentage: 0.05, at: now, atEnd: false, sitting: nil)
        try store.apply(page("""
        {"entries":[{"kind":"reading_position","id":5,"reading_position":{"file_id":5,"seconds":400,"percentage":0.6,
          "occurred_at":"2026-05-02T10:00:00Z","source":"web_reader"}}],"cursor":6}
        """))
        try store.recordPosition(fileID: 5, seconds: 40, percentage: 0.06, at: now.addingTimeInterval(10), atEnd: false, sitting: nil)

        let choice = try store.pool.read { db in
            PlaceRules.open(local: try CatalogStore.localPlace(db, fileID: 5), server: try CatalogStore.serverPlace(db, fileID: 5), ownDeviceID: "me")
        }
        XCTAssertEqual(choice.start, 40)
        XCTAssertEqual(choice.offer?.seconds, 400)
    }

    /// Another device's place that arrives while this one has moves to send
    /// is kept as an offer, since sending them may replace it on the server,
    /// until the user picks.
    func testAnotherDevicesPlaceIsKeptAsAnOfferUntilResolved() throws {
        try store.recordPosition(fileID: 5, seconds: 30, percentage: 0.05, at: now, atEnd: false, sitting: nil)
        try store.apply(page("""
        {"entries":[{"kind":"reading_position","id":5,"reading_position":{"file_id":5,"seconds":400,"percentage":0.6,
          "occurred_at":"2026-05-02T10:00:00Z","source":"web_reader"}}],"cursor":4}
        """), ownDeviceID: "me")
        // This device's move reached the server, and replaced the web's.
        try store.applyJournalResults(try store.pendingJournal(capabilities: "", limit: 10).map {
            Shishoapp.JournalResult(id: $0.eventID, result: .accepted)
        }, sent: try store.pendingJournal(capabilities: "", limit: 10), capabilities: "")
        try store.apply(page("""
        {"entries":[{"kind":"reading_position","id":5,"reading_position":{"file_id":5,"seconds":30,"percentage":0.05,
          "occurred_at":"2027-01-15T08:00:00Z","source":"shisho_app","device_id":"me"}}],"cursor":6}
        """), ownDeviceID: "me")

        let choice = try store.pool.read { db in
            PlaceRules.open(local: try CatalogStore.localPlace(db, fileID: 5), server: try CatalogStore.serverPlace(db, fileID: 5), ownDeviceID: "me")
        }
        XCTAssertEqual(choice.start, 30)
        XCTAssertEqual(choice.offer?.seconds, 400)
        try store.resolveOffer(fileID: 5)
        XCTAssertNil(try store.pool.read { try CatalogStore.localPlace($0, fileID: 5) }?.offer)
    }

    /// The server resetting the place, as reading again on the web does,
    /// resets this device's synced place too, but not moves still to send.
    func testAResetPlaceIsForgottenUnlessMovesWait() throws {
        try store.recordPosition(fileID: 5, seconds: 30, percentage: 0.05, at: now, atEnd: false, sitting: nil)
        let reset = try page(#"{"entries":[{"kind":"reading_position","id":5,"deleted":true}],"cursor":4}"#)
        try store.apply(reset)
        XCTAssertNotNil(try store.pool.read { try CatalogStore.localPlace($0, fileID: 5) })

        let sent = try store.pendingJournal(capabilities: "", limit: 10)
        try store.applyJournalResults(sent.map { Shishoapp.JournalResult(id: $0.eventID, result: .accepted) }, sent: sent, capabilities: "")
        try store.apply(try page(#"{"entries":[{"kind":"reading_position","id":5,"deleted":true}],"cursor":6}"#))
        XCTAssertNil(try store.pool.read { try CatalogStore.localPlace($0, fileID: 5) })
    }

    func testTheOpenSittingIsKept() throws {
        let (sitting, _) = ListeningRules.play(nil, fileID: 5, at: now, percentage: 0)
        try store.saveSitting(sitting)
        XCTAssertEqual(try store.openSitting(), sitting)
        try store.saveSitting(nil)
        XCTAssertNil(try store.openSitting())
    }
}

final class DiagnosticsTests: XCTestCase {
    func testExportRemovesAddressesAndCredentials() {
        let text = """
        sync failed: unreachable("Could not connect to https://books.example.com/mobile/changes")
        GET http://192.168.1.20:3789/mobile/files/5/stream Authorization: Bearer abc.def-123
        server books.example.com said no; fe80::1:2:3:4 too, and fd12:3456::1 at 1:02:05
        file 12: couldn't play
        """
        let redacted = Diagnostics.redact(text, servers: [URL(string: "https://books.example.com")!])
        XCTAssertFalse(redacted.contains("books.example.com"))
        XCTAssertFalse(redacted.contains("192.168"))
        XCTAssertFalse(redacted.contains("abc.def"))
        XCTAssertFalse(redacted.contains("fe80"))
        XCTAssertFalse(redacted.contains("fd12"))
        XCTAssertTrue(redacted.contains("at 1:02:05"))
        XCTAssertTrue(redacted.contains("file 12: couldn't play"))
        XCTAssertTrue(redacted.contains("<server>"))
    }

    func testTheLogIsKeptAndNamesIDs() async throws {
        let folder = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        let diagnostics = Diagnostics(folder: folder)
        await diagnostics.log(.player, "file 12: couldn't play")
        await diagnostics.log(.sync, "book 3 moved")
        let ids = await diagnostics.mentionedIDs()
        XCTAssertEqual(ids.files, [12])
        XCTAssertEqual(ids.books, [3])
        let export = await diagnostics.export(accounts: [], servers: [], titles: ["file 12": "Harbor Lights"], device: "iPhone", system: "iOS 26")
        XCTAssertTrue(export.contains("[player] file 12: couldn't play"))
        XCTAssertTrue(export.contains("file 12: Harbor Lights"))
    }

    func testAJournalAnswerWithoutResultsKeepsTheBatch() {
        XCTAssertThrowsError(try APIClient.journalResponse(Data(#"{"items":[]}"#.utf8), sent: 2))
        XCTAssertThrowsError(try APIClient.journalResponse(Data(#"{"results":[{"id":"a","result":"accepted"}]}"#.utf8), sent: 2))
        XCTAssertNoThrow(try APIClient.journalResponse(Data(#"{"results":[{"id":"a","result":"accepted"}]}"#.utf8), sent: 1))
    }
}
