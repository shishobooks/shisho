import GRDB
import ShishoAPI
import XCTest
@testable import Shisho

/// The reading record offline (#918): changes made on the device show at
/// once over the feed's copy, until the server confirms or refuses them,
/// and Reading Stats count reading not yet synced exactly once.
final class ReadingRecordTests: XCTestCase {
    private var store: CatalogStore!
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    override func setUpWithError() throws {
        store = try CatalogStore(path: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path)
        try store.startOverIfNeeded()
        let books = (1...3).map { id in
            """
            {"kind":"book","id":\(id),"library_id":1,"book":{"id":\(id),"library_id":1,"title":"Book \(id)","sort_title":"Book \(id)",
             "created_at":"2026-01-01T00:00:00Z","cover_version":"","files":[{"id":\(id * 10),"file_type":"m4b","file_role":"main",
             "size_bytes":10,"audiobook_duration_seconds":600}]}}
            """
        }.joined(separator: ",")
        try store.apply(page("""
        {"entries":[{"kind":"library","id":1,"library_id":1,"library":{"id":1,"name":"Audio","cover_aspect_ratio":"audiobook"}},
         \(books)],"cursor":2}
        """))
    }

    private func page(_ json: String) throws -> Shishoapp.FeedResponse {
        try makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data(json.utf8))
    }

    private func status(_ bookID: Int) throws -> String? {
        try store.pool.read { try CatalogStore.standing($0, bookID: bookID) }.status
    }

    private func journalTypes() throws -> [String] {
        try store.pool.read { try String.fetchAll($0, sql: "SELECT type FROM journal ORDER BY seq") }
    }

    /// The server's answer to everything in the journal.
    private func answer(_ result: Shishoapp.JournalResultKind, reason: String = "") throws {
        let rows = try store.pendingJournal(capabilities: "", limit: 500)
        try store.applyJournalResults(rows.map { Shishoapp.JournalResult(id: $0.eventID, result: result, reason: reason) },
                                      sent: rows, capabilities: "")
    }

    func testAStartShowsAtOnceAndGivesWayToTheServersReadThrough() throws {
        try store.record(.start(book: 1), at: now)
        XCTAssertEqual(try status(1), "currently_reading")
        XCTAssertEqual(try journalTypes(), ["read_through_start"])
        let entry = try XCTUnwrap(store.pool.read { try CatalogStore.history($0) }.first)
        XCTAssertTrue(entry.pending)
        XCTAssertLessThan(entry.id, 0)

        // Taken in: it shows until the feed brings the server's Read-through,
        // which it doesn't start a second one beside.
        let began = Date()
        try answer(.accepted)
        XCTAssertEqual(try status(1), "currently_reading")
        try store.apply(page("""
        {"entries":[{"kind":"read_through","id":7,"read_through":{"id":7,"book_id":1,"begin_day":"2027-01-15","outcome":"underway","paused":false}}],
         "cursor":3}
        """))
        XCTAssertEqual(try store.pool.read { try CatalogStore.history($0) }.map(\.id).count, 1)
        try store.feedCaughtUp(since: began.addingTimeInterval(1))
        let confirmed = try store.pool.read { try CatalogStore.history($0) }
        XCTAssertEqual(confirmed.map(\.id), [7])
        XCTAssertFalse(confirmed[0].pending)
    }

    func testDeletingAReadThroughNotYetSentDropsItsChoice() throws {
        try store.record(.finish(book: 1), at: now)
        let id = try XCTUnwrap(store.pool.read { try CatalogStore.history($0) }.first?.id)
        try store.record(.deleteReadThrough(id: id), at: now)
        XCTAssertTrue(try store.pool.read { try CatalogStore.history($0) }.isEmpty)
        XCTAssertEqual(try journalTypes(), [])
    }

    func testARefusedChangeFallsAway() throws {
        try store.record(.rating(book: 2, units: 18), at: now)
        XCTAssertEqual(try store.pool.read { try CatalogStore.standing($0, bookID: 2) }.rating, 18)
        try answer(.refused, reason: "The Book doesn't exist, or you can no longer reach it.")
        XCTAssertNil(try store.pool.read { try CatalogStore.standing($0, bookID: 2) }.rating)
        XCTAssertEqual(try store.pool.read { try CatalogStore.journalNotes($0) }.count, 1)
    }

    func testToReadOrderFollowsMovesMadeOffline() throws {
        for id in [1, 2, 3] { try store.record(.addToRead(book: id), at: now) }
        try store.record(.moveToRead(book: 3, after: nil), at: now)
        try store.record(.moveToRead(book: 1, after: 2), at: now)
        XCTAssertEqual(try store.pool.read { try CatalogStore.toReadBooks($0) }.map(\.id), [3, 2, 1])
        XCTAssertEqual(try status(1), "to_read")

        // Starting a Book takes it off To Read.
        try store.record(.start(book: 2), at: now)
        XCTAssertEqual(try store.pool.read { try CatalogStore.toReadBooks($0) }.map(\.id), [3, 1])
    }

    func testAListMadeOfflineIsNamedByItsLocalIDUntilTheFeedBringsIt() throws {
        let id = try XCTUnwrap(try store.record(.createList(name: "Trip", description: nil, ordered: true), at: now))
        XCTAssertLessThan(id, 0)
        try store.record(.addToList(id: id, book: 2), at: now)
        try store.record(.addToList(id: id, book: 1), at: now)
        try store.record(.moveInList(id: id, book: 1, after: nil), at: now)
        XCTAssertEqual(try store.pool.read { try CatalogStore.listOrder($0, listID: id) }, [1, 2])
        let payloads = try store.pool.read { try Data.fetchAll($0, sql: "SELECT payload FROM journal WHERE type = 'list_add'") }
        let ref = try makeShishoDecoder().decode(Shishoapp.JournalListBookPayload.self, from: payloads[0])
        XCTAssertNil(ref.listId)
        let localID = try XCTUnwrap(ref.localId)

        let began = Date()
        try answer(.accepted)
        try store.apply(page("""
        {"entries":[{"kind":"list","id":40,"list":{"id":40,"name":"Trip","is_ordered":true,"default_sort":"manual","owner":{"id":1,"username":"me"},
          "permission":"owner","books":[{"book_id":1,"sort_order":1,"added_at":"2027-01-01T00:00:00Z"},
          {"book_id":2,"sort_order":2,"added_at":"2027-01-01T00:00:00Z"}],"shares":[],"local_id":"\(localID)"}}],"cursor":3}
        """))
        try store.feedCaughtUp(since: began.addingTimeInterval(1))
        XCTAssertEqual(try store.pool.read { try CatalogStore.lists($0) }.map(\.id), [40])
    }

    func testListeningOfflineStartsAndFinishesTheBook() throws {
        let (sitting, _) = ListeningRules.play(nil, fileID: 10, at: now, percentage: 0)
        try store.recordPosition(fileID: 10, seconds: 3, percentage: 0.005, at: now.addingTimeInterval(3), atEnd: false, sitting: sitting)
        XCTAssertNil(try status(1), "a few seconds don't start a Book")
        try store.recordPosition(fileID: 10, seconds: 12, percentage: 0.02, at: now.addingTimeInterval(12), atEnd: false, sitting: sitting)
        XCTAssertEqual(try status(1), "currently_reading")
        try store.recordPosition(fileID: 10, seconds: 560, percentage: 0.93, at: now.addingTimeInterval(20), atEnd: false, sitting: sitting)
        XCTAssertEqual(try status(1), "read", "the last minute is the end")

        // A Reference Book never starts.
        try store.record(.markReference(book: 2), at: now)
        let (other, _) = ListeningRules.play(nil, fileID: 20, at: now, percentage: 0)
        try store.recordPosition(fileID: 20, seconds: 30, percentage: 0.05, at: now.addingTimeInterval(30), atEnd: false, sitting: other)
        XCTAssertEqual(try status(2), "reference")
    }

    func testAStatusChangeThatDeletesTheReadThroughStillShowsItsNotSynced() throws {
        func pending(_ book: Int) throws -> Bool {
            try store.pool.read { try ReadingControlsState.pendingStatus($0, bookID: book) }
        }
        XCTAssertFalse(try pending(2))
        try store.record(.markReference(book: 2), at: now)
        XCTAssertTrue(try pending(2), "Mark as Reference, with no Read-through left to mark")
        try answer(.accepted)
        XCTAssertTrue(try pending(2), "until the feed brings it back")
    }

    func testAReadThroughStartedByReadingKeepsItsIDSaveToSave() throws {
        let (sitting, _) = ListeningRules.play(nil, fileID: 10, at: now, percentage: 0)
        try store.recordPosition(fileID: 10, seconds: 12, percentage: 0.02, at: now.addingTimeInterval(12), atEnd: false, sitting: sitting)
        let first = try XCTUnwrap(store.pool.read { try CatalogStore.history($0) }.first?.id)
        try store.recordPosition(fileID: 10, seconds: 20, percentage: 0.03, at: now.addingTimeInterval(20), atEnd: false, sitting: sitting)
        XCTAssertEqual(try store.pool.read { try CatalogStore.history($0) }.map(\.id), [first])
    }

    func testAnOfflineRereadIsAPassOfItsOwn() throws {
        let (sitting, _) = ListeningRules.play(nil, fileID: 10, at: now, percentage: 0)
        try store.recordPosition(fileID: 10, seconds: 12, percentage: 0.02, at: now.addingTimeInterval(12), atEnd: false, sitting: sitting)
        try store.recordPosition(fileID: 10, seconds: 590, percentage: 0.98, at: now.addingTimeInterval(20), atEnd: true, sitting: sitting)
        try store.record(.addToRead(book: 1), at: now.addingTimeInterval(30))
        let (again, _) = ListeningRules.play(nil, fileID: 10, at: now.addingTimeInterval(40), percentage: 0)
        try store.recordPosition(fileID: 10, seconds: 12, percentage: 0.02, at: now.addingTimeInterval(52), atEnd: false, sitting: again)
        let outcomes = try store.pool.read { try CatalogStore.history($0) }.map(\.outcome)
        XCTAssertEqual(outcomes, ["underway", "finished"])
    }

    func testDeletingAnOfflinePassLeavesTheNextOne() throws {
        try store.record(.start(book: 1), at: now)
        try store.record(.finish(book: 1), at: now)
        try store.record(.start(book: 1), at: now)
        let first = try XCTUnwrap(store.pool.read { try CatalogStore.history($0) }.first { $0.outcome == "finished" }?.id)
        try store.record(.deleteReadThrough(id: first), at: now)
        XCTAssertEqual(try store.pool.read { try CatalogStore.history($0) }.map(\.outcome), ["underway"])
        XCTAssertEqual(try journalTypes(), ["read_through_start"])
    }

    func testClearingAPlaceKeepsTheMovesNotSent() throws {
        let (sitting, _) = ListeningRules.play(nil, fileID: 10, at: now, percentage: 0)
        try store.recordPosition(fileID: 10, seconds: 12, percentage: 0.02, at: now.addingTimeInterval(12), atEnd: false, sitting: sitting)
        try store.record(.clearPosition(file: 10), at: now.addingTimeInterval(30))
        XCTAssertEqual(try journalTypes(), ["reading_position", "position_clear"])
        XCTAssertEqual(try status(1), "currently_reading")
        XCTAssertNil(try store.pool.read { try CatalogStore.localPlace($0, fileID: 10) })
    }

    func testDeletingAReadThroughMadeOfflineDropsEveryChoiceAboutIt() throws {
        try store.record(.start(book: 1), at: now)
        try store.record(.finish(book: 1), at: now)
        let id = try XCTUnwrap(store.pool.read { try CatalogStore.history($0) }.first?.id)
        try store.record(.deleteReadThrough(id: id), at: now)
        XCTAssertTrue(try store.pool.read { try CatalogStore.history($0) }.isEmpty)
        XCTAssertEqual(try journalTypes(), [])
    }

    func testAChangeIsConfirmedWhenTheFeedBringsItsBook() throws {
        try store.record(.finish(book: 1), at: now)
        try answer(.accepted)
        // The server's day differs from the phone's, as with a time zone set.
        try store.apply(page("""
        {"entries":[{"kind":"read_through","id":9,"read_through":{"id":9,"book_id":1,"end_day":"2000-01-02","outcome":"finished","paused":false}}],
         "cursor":3}
        """))
        XCTAssertEqual(try store.pool.read { try CatalogStore.history($0) }.map(\.id), [9])
        XCTAssertEqual(try store.pool.read { try Int.fetchOne($0, sql: "SELECT COUNT(*) FROM journal_applied") }, 0)
    }

    func testAFinishTheFeedBroughtBackIsntShownTwice() throws {
        try store.record(.finish(book: 1), at: now)
        let day = CatalogStore.day(now)
        try answer(.accepted)
        try store.apply(page("""
        {"entries":[{"kind":"read_through","id":9,"read_through":{"id":9,"book_id":1,"end_day":"\(day)","outcome":"finished","paused":false}}],
         "cursor":3}
        """))
        XCTAssertEqual(try store.pool.read { try CatalogStore.history($0) }.map(\.id), [9])
    }

    func testMarkingReferenceDissolvesTheReadThroughUnderway() throws {
        try store.record(.start(book: 1), at: now)
        try store.record(.markReference(book: 1), at: now)
        XCTAssertEqual(try status(1), "reference")
        XCTAssertTrue(try store.pool.read { try CatalogStore.history($0) }.isEmpty)
        try store.record(.finish(book: 1), at: now)
        XCTAssertEqual(try status(1), "read")
    }

    // MARK: - Home

    func testRecentReferenceListsReferenceBooksOpenedHereLatestFirst() throws {
        try store.record(.markReference(book: 1), at: now)
        try store.record(.markReference(book: 2), at: now)
        try store.noteOpened(fileID: 10, at: now)
        try store.noteOpened(fileID: 20, at: now.addingTimeInterval(60))
        try store.noteOpened(fileID: 30, at: now.addingTimeInterval(120))
        XCTAssertEqual(try store.pool.read { try CatalogStore.recentReference($0) }.map(\.id), [2, 1], "only Reference Books")

        try store.noteOpened(fileID: 10, at: now.addingTimeInterval(180))
        XCTAssertEqual(try store.pool.read { try CatalogStore.recentReference($0) }.map(\.id), [1, 2])
        try store.clearRecent(bookIDs: [1])
        XCTAssertEqual(try store.pool.read { try CatalogStore.recentReference($0) }.map(\.id), [2])
        try store.record(.removeReference(book: 2), at: now)
        XCTAssertTrue(try store.pool.read { try CatalogStore.recentReference($0) }.isEmpty, "no longer Reference")
        try store.record(.markReference(book: 2), at: now)
        try store.clearRecent(bookIDs: nil)
        XCTAssertTrue(try store.pool.read { try CatalogStore.recentReference($0) }.isEmpty)
    }

    func testContinueCardsGoBackToTheFileUsedLast() throws {
        try store.record(.start(book: 1), at: now)
        let items = try store.pool.read { try CatalogStore.continueItems($0) }
        XCTAssertEqual(items.map(\.book.id), [1])
        XCTAssertEqual(items.first?.fileID, 10)
        XCTAssertEqual(items.first?.listens, true)

        try store.record(.markReference(book: 2), at: now)
        try store.noteOpened(fileID: 20, at: now)
        XCTAssertEqual(try store.pool.read { try CatalogStore.recentReferenceItems($0) }.map(\.fileID), [20])
    }

    // MARK: - Reading Stats

    private func cache(_ json: String, fetchedAt: Date) throws {
        let stats = try makeShishoDecoder().decode(Shishoapp.StatsResponse.self, from: Data(json.utf8))
        try store.saveStats(stats, year: nil, fetchedAt: fetchedAt)
    }

    private func allTime() throws -> ReadingStats {
        try XCTUnwrap(store.pool.read { try CatalogStore.stats($0, year: nil, ownDeviceID: "me") })
    }

    func testReadingStatsCountUnsyncedReadingOnce() throws {
        try cache("""
        {"year":null,"years":[2027],"time_zone":"UTC","totals":{"finished":1,"rereads":0,"did_not_finish":0,"reading_seconds":0,
          "listening_seconds":600,"words_read":0,"average_rating":null},"pace":{},"periods":[{"year":2027,"month":null,"finished":1,
          "reading_seconds":0,"listening_seconds":600}],"genres":[],"tags":[],"authors":[],"kinds":[],"ratings":[],
          "places":[{"source":"shisho_app","device_id":"me","device_name":"iPhone","reading_seconds":0,"listening_seconds":600}]}
        """, fetchedAt: now)

        // Ten minutes listened offline, sent in two snapshots of one sitting.
        let start = Date(timeIntervalSince1970: 1_800_000_000)
        try store.saveSitting(nil, recording: FinishedSession(sittingID: "sit-1", fileID: 10, startedAt: start,
                                                              endedAt: start.addingTimeInterval(300), startPercentage: 0, endPercentage: 0.5,
                                                              duration: 300))
        XCTAssertEqual(try allTime().stats.totals.listeningSeconds, 900)
        XCTAssertEqual(try allTime().stats.places.first?.listeningSeconds, 900)

        // The first snapshot is taken in and Reading Stats fetched again,
        // counting it; the second adds only what's new.
        try answer(.accepted)
        let refetched = Date()
        try cache("""
        {"year":null,"years":[2027],"time_zone":"UTC","totals":{"finished":1,"rereads":0,"did_not_finish":0,"reading_seconds":0,
          "listening_seconds":900,"words_read":0,"average_rating":null},"pace":{},"periods":[],"genres":[],"tags":[],"authors":[],
          "kinds":[],"ratings":[],"places":[]}
        """, fetchedAt: refetched)
        XCTAssertEqual(try allTime().stats.totals.listeningSeconds, 900, "not counted twice once the copy has it")
        try store.statsCaughtUp()
        try store.saveSitting(nil, recording: FinishedSession(sittingID: "sit-1", fileID: 10, startedAt: start,
                                                              endedAt: start.addingTimeInterval(600), startPercentage: 0, endPercentage: 1,
                                                              duration: 600))
        XCTAssertEqual(try allTime().stats.totals.listeningSeconds, 1200)
        XCTAssertEqual(try allTime().unsyncedSeconds, 300)
    }

    func testReadingStatsSplitUnsyncedReadingAtMidnightAsTheServerDoes() throws {
        try cache("""
        {"year":null,"years":[],"time_zone":"UTC","totals":{"finished":0,"rereads":0,"did_not_finish":0,"reading_seconds":0,
          "listening_seconds":0,"words_read":0,"average_rating":null},"pace":{},"periods":[],"genres":[],"tags":[],"authors":[],
          "kinds":[],"ratings":[],"places":[]}
        """, fetchedAt: now)
        // An hour listened from 11:30pm on New Year's Eve, half in each year.
        let start = ISO8601DateFormatter().date(from: "2026-12-31T23:30:00Z")!
        try store.saveSitting(nil, recording: FinishedSession(sittingID: "nye", fileID: 10, startedAt: start,
                                                              endedAt: start.addingTimeInterval(3600), startPercentage: 0, endPercentage: 0.5,
                                                              duration: 3600))
        let stats = try allTime().stats
        XCTAssertEqual(stats.totals.listeningSeconds, 3600)
        XCTAssertEqual(stats.periods.first { $0.year == 2026 }?.listeningSeconds, 1800)
        XCTAssertEqual(stats.periods.first { $0.year == 2027 }?.listeningSeconds, 1800)
        XCTAssertEqual(stats.years, [2027, 2026])
        XCTAssertEqual(try allTime().unsyncedSeconds, 3600)
    }

    func testReadingStatsCountFinishesMadeOffline() throws {
        try cache("""
        {"year":null,"years":[],"time_zone":"UTC","totals":{"finished":0,"rereads":0,"did_not_finish":0,"reading_seconds":0,
          "listening_seconds":0,"words_read":0,"average_rating":null},"pace":{},"periods":[],"genres":[],"tags":[],"authors":[],
          "kinds":[],"ratings":[],"places":[]}
        """, fetchedAt: now)
        try store.record(.finish(book: 1), at: now)
        try store.record(.start(book: 2), at: now)
        try store.record(.abandon(book: 2), at: now)
        let totals = try allTime().stats.totals
        XCTAssertEqual(totals.finished, 1)
        XCTAssertEqual(totals.didNotFinish, 1)
    }
}

/// The status menu and Reading History word things as the web does.
final class ReadingWordingTests: XCTestCase {
    func testTheStatusMenuOffersTheWebsTransitions() {
        XCTAssertEqual(ReadingTransitions.available("currently_reading"), [.pause, .finish, .abandon])
        XCTAssertEqual(ReadingTransitions.available("paused"), [.resume, .finish, .abandon])
        XCTAssertEqual(ReadingTransitions.available(nil), [.start, .finish])
        XCTAssertEqual(ReadingTransitions.label(.start, status: "read", readBefore: true), "Read again")
        XCTAssertEqual(ReadingTransitions.label(.start, status: "reference", readBefore: false), "Start reading")
        XCTAssertEqual(ReadingTransitions.label(.start, status: "reference", readBefore: true), "Read again")
        XCTAssertFalse(ReadingTransitions.canQueue("paused"))
        XCTAssertTrue(ReadingTransitions.canQueue("read"))
    }

    func testHistoryDatesReadAsTheWebsDo() {
        func entry(_ outcome: String, _ begin: String?, _ end: String?) -> HistoryEntry {
            HistoryEntry(id: 1, book: nil, title: "", authors: "", outcome: outcome, paused: false, beginDay: begin, endDay: end, pending: false)
        }
        XCTAssertEqual(HistoryText.days(entry("underway", "2026-03-01", nil)), "Started Mar 1, 2026")
        XCTAssertEqual(HistoryText.days(entry("finished", "2026-03-01", "2026-03-09")), "Mar 1, 2026 to Mar 9, 2026")
        XCTAssertEqual(HistoryText.days(entry("finished", nil, "2026-03-09")), "Ended Mar 9, 2026, start date unknown")
        XCTAssertEqual(HistoryText.days(entry("abandoned", nil, nil)), "Dates unknown")
    }

    func testADragIsAMoveAfterItsNewNeighbor() {
        XCTAssertNil(ReadingLayer.move(in: [1, 2, 3], from: 1, to: 1))
        XCTAssertEqual(ReadingLayer.move(in: [1, 2, 3], from: 2, to: 0).map { [$0.book, $0.after ?? 0] }, [3, 0])
        XCTAssertEqual(ReadingLayer.move(in: [1, 2, 3], from: 0, to: 2).map { [$0.book, $0.after ?? 0] }, [1, 3])
    }

    func testPlacesAreNamedAsTheWebNamesThem() {
        let places = StatsText.places([
            Shishoapp.StatsPlace(source: .webReader, readingSeconds: 600, listeningSeconds: 120),
            Shishoapp.StatsPlace(source: .shishoApp, deviceId: "me", deviceName: "iPhone", listeningSeconds: 900),
            Shishoapp.StatsPlace(source: .kobo, deviceId: "k", deviceName: "Kobo Libra", readingSeconds: 60),
        ], ownDeviceID: "me")
        XCTAssertEqual(places.map(\.label), ["iPhone (this device)", "Shisho web reader", "Shisho web player", "Kobo Libra"])
    }

    func testChartTicksMatchTheWebs() {
        XCTAssertEqual(ChartTicks.nice(18319), [0, 5000, 10000, 15000, 20000])
        XCTAssertEqual(ChartTicks.nice(0), [0, 1])
        let hours = ChartTicks.duration(7.2 * 3600)
        XCTAssertEqual(hours.ticks.map { $0 * hours.unit }, [0, 7200, 14400, 21600, 28800])
        XCTAssertEqual(hours.label(2), "2h")
        let minutes = ChartTicks.duration(25 * 60)
        XCTAssertEqual(minutes.unit, 60)
        XCTAssertEqual(minutes.label(10), "10m")
        XCTAssertEqual(minutes.label(0), "0")
        XCTAssertEqual(ChartTicks.roomy(1), [0, 1, 2], "a step more when the most is exactly the top tick")
        XCTAssertEqual(ChartTicks.roomy(5), [0, 2, 4, 6])
        XCTAssertEqual(ChartTicks.duration(4 * 3600, roomy: true).ticks, [0, 1, 2, 3, 4, 5])
    }

    func testRereadsAreCountedInWords() {
        XCTAssertEqual(StatsText.rereads(1), "1 reread")
        XCTAssertEqual(StatsText.rereads(2), "2 rereads")
    }

    func testTotalsGoToDaysAsTheWebs() {
        XCTAssertEqual(StatsText.totalDuration(526_800), "6d 2h 20m")
        XCTAssertEqual(StatsText.totalHours(526_800), "146h 20m")
        XCTAssertEqual(StatsText.totalDuration(3661), "1h 1m")
        XCTAssertNil(StatsText.totalHours(3661))
    }

    func testMovesFollowTheWebsRule() {
        var order = [1, 2, 3, 4]
        ReadingLayer.move(4, after: nil, in: &order)
        XCTAssertEqual(order, [4, 1, 2, 3])
        ReadingLayer.move(4, after: 2, in: &order)
        XCTAssertEqual(order, [1, 2, 4, 3])
        ReadingLayer.move(1, after: 9, in: &order)
        XCTAssertEqual(order, [1, 2, 4, 3], "a Book whose neighbor is gone stays where it is")
    }
}
