import GRDB
import ShishoAPI
import XCTest
@testable import Shisho

/// What makes a sync fetch the calendar's copies again, and how a session
/// splits across days.
final class ReadingCalendarStaleTests: XCTestCase {
    private func row(_ type: Shishoapp.JournalEventType, _ payload: some Encodable, at: Date = Date()) throws -> JournalRow {
        JournalRow(seq: 1, eventID: UUID().uuidString, type: type.rawValue, at: at, payload: try JSONEncoder().encode(payload))
    }

    func testReadingChangesAndPositionsNearTheEndCanChangeTheCalendar() throws {
        let session = try row(.readingSession, Shishoapp.JournalSessionPayload(sessionId: "sit", fileId: 10, startedAt: "2026-11-11T05:40:00Z",
                                                                               endedAt: "2026-11-11T06:20:00Z", startPercentage: 0, endPercentage: 0.5))
        XCTAssertTrue(CatalogStore.changesCalendar(session))
        XCTAssertTrue(CatalogStore.changesCalendar(try row(.readThroughFinish, Shishoapp.JournalBookPayload(bookId: 2, day: "2026-11-12"))))
        XCTAssertTrue(CatalogStore.changesCalendar(try row(.readThroughDelete, Shishoapp.JournalReadThroughDeletePayload(readThroughId: 7))))
        XCTAssertTrue(CatalogStore.changesCalendar(try row(.referenceMark, Shishoapp.JournalBookPayload(bookId: 2))))
        // A position at the end can finish a Read-through on the server.
        XCTAssertTrue(CatalogStore.changesCalendar(try row(.readingPosition, Shishoapp.JournalPositionPayload(fileId: 10, percentage: 0.4, atEnd: true))))
        XCTAssertTrue(CatalogStore.changesCalendar(try row(.readingPosition, Shishoapp.JournalPositionPayload(fileId: 10, percentage: 0.95))))
        XCTAssertFalse(CatalogStore.changesCalendar(try row(.readingPosition, Shishoapp.JournalPositionPayload(fileId: 10, percentage: 0.4))))
        XCTAssertFalse(CatalogStore.changesCalendar(try row(.ratingSet, Shishoapp.JournalRatingPayload(bookId: 2, rating: 4))))
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

    func testEveryCopyIsFetchedAgainOnceAChangeThatCanAlterItWasSent() throws {
        try store.saveCalendar(Shishoapp.StatsHeatmapResponse(), key: .heatmap(year: nil), fetchedAt: start)
        try store.saveCalendar(Shishoapp.StatsCalendarResponse(year: 2026, month: 12), key: .month(year: 2026, month: 12), fetchedAt: start)
        try store.saveCalendar(Shishoapp.StatsDayResponse(day: "2020-01-01", timeZone: "UTC"), key: .day("2020-01-01"), fetchedAt: start)
        XCTAssertEqual(try store.calendarCopiesToRefresh(), [], "nothing sent, nothing stale")

        // The server decides which days a change lands on, so every copy
        // goes stale, a day years before the session included.
        try listen("sit-1", seconds: 600, from: start)
        try store.markCalendarStale(pending())
        XCTAssertEqual(Set(try store.calendarCopiesToRefresh()), [.heatmap(year: nil), .month(year: 2026, month: 12), .day("2020-01-01")])

        try store.calendarRefreshed(before: Date().addingTimeInterval(1))
        XCTAssertEqual(try store.calendarCopiesToRefresh(), [])
        XCTAssertEqual(try count("calendar_cache"), 3, "fetching again doesn't drop copies")
    }

    func testTheDeviceKeepsTheCopiesTheCalendarShowedLast() throws {
        for day in 1...12 {
            let key = String(format: "2026-03-%02d", day)
            try store.saveCalendar(Shishoapp.StatsDayResponse(day: key, timeZone: "UTC"), key: .day(key), fetchedAt: start)
        }
        let kept = try store.pool.read { try String.fetchAll($0, sql: "SELECT cache_key FROM calendar_cache ORDER BY cache_key") }
        XCTAssertEqual(kept, (3...12).map { String(format: "day:2026-03-%02d", $0) })

        // A sync's copy doesn't count as shown, nor replace a newer one.
        try store.saveCalendar(Shishoapp.StatsDayResponse(day: "2026-03-03", timeZone: "UTC", sessions: []), key: .day("2026-03-03"),
                               fetchedAt: start.addingTimeInterval(-60), viewed: false)
        let fetched = try store.pool.read { try Double.fetchOne($0, sql: "SELECT fetched_at FROM calendar_cache WHERE cache_key = 'day:2026-03-03'") }
        XCTAssertEqual(fetched, start.timeIntervalSince1970)
    }

    func testADayCopyLaysThisDevicesReadingInItsOwnTimeZone() throws {
        // 11:40 PM in Chicago, the 14th, though 5:40 AM on the 15th in UTC,
        // which Reading Stats still say.
        try store.saveCalendar(Shishoapp.StatsDayResponse(day: "2027-01-14", timeZone: "America/Chicago"), key: .day("2027-01-14"),
                               fetchedAt: start, zone: "America/Chicago")
        try listen("sit-1", seconds: 600, from: Date(timeIntervalSince1970: try XCTUnwrap(CatalogStore.timestamp("2027-01-15T05:40:00Z"))))
        let shown = try XCTUnwrap(store.pool.read {
            try CatalogStore.calendarDay($0, day: "2027-01-14", ownDeviceID: "me", deviceName: "iPhone")
        })
        XCTAssertEqual(shown.value.response.sessions.count, 1)
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

    func testACalendarCopyHoldsBackWhatItDoesntCount() throws {
        try store.saveCalendar(Shishoapp.StatsDayResponse(day: "2026-01-01", timeZone: "UTC"), key: .day("2026-01-01"),
                               fetchedAt: Date().addingTimeInterval(-90 * 86400))
        try listen("sit-1", seconds: 600, from: start)
        let rows = try pending()
        try store.applyJournalResults(rows.map { Shishoapp.JournalResult(id: $0.eventID, result: .accepted, reason: "") },
                                      sent: rows, capabilities: "")
        XCTAssertEqual(try count("unstated_sessions"), 1)
        try store.saveStats(stats(zone: "UTC"), year: nil, fetchedAt: Date())
        try store.statsCaughtUp()
        XCTAssertEqual(try count("unstated_sessions"), 1, "the copy, not fetched again, doesn't count it")
    }
}
