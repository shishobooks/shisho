import GRDB
import ShishoAPI
import XCTest
@testable import Shisho

/// A sync fetches again each calendar copy showing a day the changes it sent
/// fall on, so the copy counts them before Reading Stats stop laying them
/// over it, and drops every copy when Reading Stats move to another time
/// zone.
final class ReadingCalendarStaleDaysTests: XCTestCase {
    private let chicago = TimeZone(identifier: "America/Chicago")!

    private func row(_ type: Shishoapp.JournalEventType, _ payload: some Encodable, at: Date = Date()) throws -> JournalRow {
        JournalRow(seq: 1, eventID: UUID().uuidString, type: type.rawValue, at: at, payload: try JSONEncoder().encode(payload))
    }

    private func session(_ start: String, _ end: String) throws -> JournalRow {
        try row(.readingSession, Shishoapp.JournalSessionPayload(sessionId: "sit", fileId: 10, startedAt: start, endedAt: end,
                                                                startPercentage: 0, endPercentage: 0.5))
    }

    func testASessionPastMidnightTouchesBothDaysInTheServersZone() throws {
        // 11:40 PM to 12:20 AM in Chicago.
        let late = try session("2026-11-11T05:40:00Z", "2026-11-11T06:20:00Z")
        XCTAssertEqual(CalendarLayering.touchedDays([late], zone: chicago, endDays: [:]), ["2026-11-10", "2026-11-11"])
        XCTAssertEqual(CalendarLayering.touchedDays([late], zone: TimeZone(identifier: "UTC")!, endDays: [:]), ["2026-11-11"])
    }

    func testAFinishOrAbandonTouchesItsDayAndADeleteTheDayItEnded() throws {
        let finish = try row(.readThroughFinish, Shishoapp.JournalBookPayload(bookId: 2, day: "2026-11-12"))
        let abandon = try row(.readThroughAbandon, Shishoapp.JournalBookPayload(bookId: 3, day: "2026-11-13"))
        let deleted = try row(.readThroughDelete, Shishoapp.JournalReadThroughDeletePayload(readThroughId: 7, day: "2026-11-20"))
        // The one underway has no finish on the calendar.
        let underway = try row(.readThroughDelete, Shishoapp.JournalReadThroughDeletePayload(bookId: 4, day: "2026-11-20"))
        let position = try row(.readingPosition, Shishoapp.JournalPositionPayload())
        XCTAssertEqual(CalendarLayering.touchedDays([finish, abandon, deleted, underway, position], zone: chicago,
                                                    endDays: [7: "2026-10-01"]),
                       ["2026-11-12", "2026-11-13", "2026-10-01"])
    }

    func testACopyIsStaleWhenItShowsATouchedDay() {
        XCTAssertTrue(CalendarCacheKey.day("2026-11-12").covers("2026-11-12"))
        XCTAssertFalse(CalendarCacheKey.day("2026-11-12").covers("2026-11-13"))
        XCTAssertTrue(CalendarCacheKey.month(year: 2026, month: 11).covers("2026-11-30"))
        XCTAssertFalse(CalendarCacheKey.month(year: 2026, month: 11).covers("2026-12-01"))
        XCTAssertTrue(CalendarCacheKey.heatmap(year: 2026).covers("2026-01-01"))
        XCTAssertFalse(CalendarCacheKey.heatmap(year: 2026).covers("2027-01-01"))
        // The last 12 months move on to today, so they show every day from
        // the copy's first on, later ones included.
        XCTAssertTrue(CalendarCacheKey.heatmap(year: nil).covers("2026-12-05", heatmapFrom: "2025-11-21"))
        XCTAssertFalse(CalendarCacheKey.heatmap(year: nil).covers("2025-11-20", heatmapFrom: "2025-11-21"))
    }

    func testAKeyReadsBackFromItsRaw() {
        for key in [CalendarCacheKey.heatmap(year: nil), .heatmap(year: 2026), .month(year: 2026, month: 3), .day("2026-03-04")] {
            XCTAssertEqual(CalendarCacheKey(raw: key.raw), key)
        }
        XCTAssertNil(CalendarCacheKey(raw: "nonsense"))
    }

    func testASessionAcrossADayThatBeginsAtOneSplitsAtTheNextMidnight() throws {
        // In Santiago, September 6, 2026 begins at 01:00: clocks skip
        // midnight. 11:30 PM that day to 12:30 AM on the 7th is half on
        // each, as the server splits it.
        let santiago = try XCTUnwrap(TimeZone(identifier: "America/Santiago"))
        let start = Date(timeIntervalSince1970: try XCTUnwrap(CatalogStore.timestamp("2026-09-07T02:30:00Z")))
        let parts = UnsyncedReading.splitByDay(start: start, end: start.addingTimeInterval(3600), seconds: 3600, zone: santiago)
        XCTAssertEqual(parts.map(\.day), ["2026-09-06", "2026-09-07"])
        XCTAssertEqual(parts[0].seconds, 1800, accuracy: 0.001)
        XCTAssertEqual(parts[1].seconds, 1800, accuracy: 0.001)

        // And across the skipped midnight itself, 11:30 PM on the 5th to
        // 1:30 AM on the 6th, an hour long.
        let before = Date(timeIntervalSince1970: try XCTUnwrap(CatalogStore.timestamp("2026-09-06T03:30:00Z")))
        let across = UnsyncedReading.splitByDay(start: before, end: before.addingTimeInterval(3600), seconds: 3600, zone: santiago)
        XCTAssertEqual(across.map(\.day), ["2026-09-05", "2026-09-06"])
        XCTAssertEqual(across[0].seconds, 1800, accuracy: 0.001)
        XCTAssertEqual(across[1].seconds, 1800, accuracy: 0.001)
    }
}

/// The calendar's copies a sync fetches again, read from the account's
/// database.
final class ReadingCalendarRefreshStoreTests: XCTestCase {
    private var store: CatalogStore!
    private let start = Date(timeIntervalSince1970: 1_800_000_000) // 2027-01-15T08:00:00Z

    override func setUpWithError() throws {
        store = try CatalogStore(path: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path)
        try store.startOverIfNeeded()
        let books = (1...2).map { id in
            """
            {"kind":"book","id":\(id),"library_id":1,"book":{"id":\(id),"library_id":1,"title":"Book \(id)","sort_title":"Book \(id)",
             "created_at":"2026-01-01T00:00:00Z","cover_version":"","files":[{"id":\(id * 10),"file_type":"m4b","file_role":"main",
             "size_bytes":10,"audiobook_duration_seconds":600}]}}
            """
        }.joined(separator: ",")
        try store.apply(makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data("""
        {"entries":[{"kind":"library","id":1,"library_id":1,"library":{"id":1,"name":"Audio","cover_aspect_ratio":"audiobook"}},
         \(books)],"cursor":2}
        """.utf8)))
        try store.saveStats(stats(zone: "UTC"), year: nil, fetchedAt: start)
    }

    private func stats(zone: String) throws -> Shishoapp.StatsResponse {
        try makeShishoDecoder().decode(Shishoapp.StatsResponse.self, from: Data("""
        {"year":null,"years":[],"time_zone":"\(zone)","totals":{"finished":0,"rereads":0,"did_not_finish":0,"reading_seconds":0,
          "listening_seconds":0,"words_read":0,"average_rating":null},"pace":{},"periods":[],"genres":[],"tags":[],"authors":[],
          "kinds":[],"ratings":[],"places":[]}
        """.utf8))
    }

    private func listen(_ sitting: String, seconds: Int, from: Date) throws {
        try store.saveSitting(nil, recording: FinishedSession(sittingID: sitting, fileID: 10, startedAt: from,
                                                              endedAt: from.addingTimeInterval(Double(seconds)), startPercentage: 0,
                                                              endPercentage: 0.5, duration: seconds))
    }

    private func pending() throws -> [JournalRow] {
        try store.pendingJournal(capabilities: "", limit: 500)
    }

    private func count(_ table: String) throws -> Int {
        try store.pool.read { try Int.fetchOne($0, sql: "SELECT COUNT(*) FROM \(table)") ?? 0 }
    }

    func testTheCopiesShowingASentSessionsDayAreFetchedAgain() throws {
        let heatmap = Shishoapp.StatsHeatmapResponse(today: "2027-01-14", from: "2026-01-15", to: "2027-01-14")
        try store.saveCalendar(heatmap, key: .heatmap(year: nil), fetchedAt: start)
        try store.saveCalendar(Shishoapp.StatsHeatmapResponse(), key: .heatmap(year: 2027), fetchedAt: start)
        try store.saveCalendar(Shishoapp.StatsHeatmapResponse(), key: .heatmap(year: 2026), fetchedAt: start)
        try store.saveCalendar(Shishoapp.StatsCalendarResponse(year: 2027, month: 1), key: .month(year: 2027, month: 1), fetchedAt: start)
        try store.saveCalendar(Shishoapp.StatsCalendarResponse(year: 2026, month: 12), key: .month(year: 2026, month: 12), fetchedAt: start)
        try store.saveCalendar(Shishoapp.StatsDayResponse(day: "2027-01-15", timeZone: "UTC"), key: .day("2027-01-15"), fetchedAt: start)
        try store.saveCalendar(Shishoapp.StatsDayResponse(day: "2027-01-14", timeZone: "UTC"), key: .day("2027-01-14"), fetchedAt: start)
        XCTAssertEqual(try store.calendarCopiesToRefresh(), [], "nothing sent, nothing stale")

        try listen("sit-1", seconds: 600, from: start)
        try store.markCalendarStale(pending())
        XCTAssertEqual(Set(try store.calendarCopiesToRefresh()),
                       [.heatmap(year: nil), .heatmap(year: 2027), .month(year: 2027, month: 1), .day("2027-01-15")],
                       "the last 12 months too, which move on to the 15th though its copy ends on the 14th")

        try store.calendarRefreshed(before: Date().addingTimeInterval(1))
        XCTAssertEqual(try store.calendarCopiesToRefresh(), [])
        XCTAssertEqual(try count("calendar_cache"), 7, "fetching again doesn't drop copies")
    }

    func testAFinishMakesItsDaysCopyStale() throws {
        try store.record(.finish(book: 2), at: start)
        let endDay = try XCTUnwrap(store.pool.read { try String.fetchOne($0, sql: "SELECT end_day FROM read_throughs WHERE book_id = 2") })
        try store.saveCalendar(Shishoapp.StatsDayResponse(day: endDay, timeZone: "UTC"), key: .day(endDay), fetchedAt: start)
        try store.saveCalendar(Shishoapp.StatsDayResponse(day: "2020-01-01", timeZone: "UTC"), key: .day("2020-01-01"), fetchedAt: start)
        try store.markCalendarStale(pending())
        XCTAssertEqual(try store.calendarCopiesToRefresh(), [.day(endDay)])
    }

    func testReadingStatsInAnotherTimeZoneDropEveryCopy() throws {
        try store.saveCalendar(Shishoapp.StatsCalendarResponse(year: 2027, month: 1), key: .month(year: 2027, month: 1), fetchedAt: start)
        try store.saveCalendar(Shishoapp.StatsDayResponse(day: "2027-01-15", timeZone: "UTC"), key: .day("2027-01-15"), fetchedAt: start)
        _ = try store.calendarCopiesToRefresh()
        XCTAssertEqual(try count("calendar_cache"), 2, "the same zone keeps them")

        try store.saveStats(stats(zone: "America/Chicago"), year: nil, fetchedAt: Date())
        _ = try store.calendarCopiesToRefresh()
        XCTAssertEqual(try count("calendar_cache"), 0)

        // A copy fetched in the new zone stays.
        try store.saveCalendar(Shishoapp.StatsCalendarResponse(year: 2027, month: 1), key: .month(year: 2027, month: 1), fetchedAt: Date())
        _ = try store.calendarCopiesToRefresh()
        XCTAssertEqual(try count("calendar_cache"), 1)
    }

    func testReadingStatsCatchUpAtTheirOwnCopyWhateverTheCalendarHolds() throws {
        // A day seen long ago holds nothing back.
        try store.saveCalendar(Shishoapp.StatsDayResponse(day: "2026-01-01", timeZone: "UTC"), key: .day("2026-01-01"),
                               fetchedAt: Date().addingTimeInterval(-90 * 86400))
        try listen("sit-1", seconds: 600, from: start)
        let rows = try pending()
        try store.applyJournalResults(rows.map { Shishoapp.JournalResult(id: $0.eventID, result: .accepted, reason: "") },
                                      sent: rows, capabilities: "")
        XCTAssertEqual(try count("unstated_sessions"), 1)
        try store.saveStats(stats(zone: "UTC"), year: nil, fetchedAt: Date())
        try store.statsCaughtUp()
        XCTAssertEqual(try count("unstated_sessions"), 0)
        let counted = try store.pool.read { try Double.fetchOne($0, sql: "SELECT seconds FROM counted_sessions WHERE session_id = 'sit-1'") }
        XCTAssertEqual(counted, 600)
    }
}
