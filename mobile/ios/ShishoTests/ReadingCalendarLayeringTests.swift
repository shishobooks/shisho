import GRDB
import ShishoAPI
import XCTest
@testable import Shisho

/// Reading Stats' Calendar counts reading done on this device that the
/// server hasn't counted yet, as the Overview does: on the days of the
/// server's time zone it fell on, once, and alone before a part was ever
/// fetched (CalendarLayering).
final class ReadingCalendarLayeringTests: XCTestCase {
    private let utc = TimeZone(identifier: "UTC")!
    private let chicago = TimeZone(identifier: "America/Chicago")!

    private func instant(_ value: String) -> Date {
        Date(timeIntervalSince1970: CatalogStore.timestamp(value)!)
    }

    private func session(_ book: Int, _ start: String, _ end: String, seconds: Double? = nil, counted: Double = 0,
                         listening: Bool = false) -> UnsyncedCalendarSession {
        let started = instant(start), ended = instant(end)
        return UnsyncedCalendarSession(book: Shishoapp.StatsBookRef(key: "book-\(book)", title: "Book \(book)", bookId: book),
                                       listening: listening, startedAt: started, endedAt: ended,
                                       seconds: seconds ?? ended.timeIntervalSince(started), counted: counted,
                                       startPercentage: 0.25, endPercentage: 0.5)
    }

    private func finish(_ book: Int, _ day: String) -> UnsyncedFinish {
        UnsyncedFinish(book: Shishoapp.StatsBookRef(key: "book-\(book)", title: "Book \(book)", bookId: book), day: day)
    }

    private func heatmap(days: [Shishoapp.StatsDay], today: String = "2026-11-20") -> Shishoapp.StatsHeatmapResponse {
        Shishoapp.StatsHeatmapResponse(today: today, from: CalendarLayering.window(year: nil, today: today).from, to: today,
                                       firstDay: "2026-01-02", days: days)
    }

    // MARK: - The heatmap

    func testASessionAddsItsTimeToItsDay() throws {
        let copy = heatmap(days: [Shishoapp.StatsDay(day: "2026-11-20", readingSeconds: 600, listeningSeconds: 0)])
        let shown = try XCTUnwrap(CalendarLayering.heatmap(
            copy, year: nil, today: "2026-11-20",
            sessions: [session(1, "2026-11-20T10:00:00Z", "2026-11-20T10:30:00Z"),
                       session(2, "2026-11-19T08:00:00Z", "2026-11-19T08:10:00Z", seconds: 500, listening: true)],
            finishes: [], zone: utc))
        XCTAssertEqual(shown.days, [Shishoapp.StatsDay(day: "2026-11-19", readingSeconds: 0, listeningSeconds: 500),
                                    Shishoapp.StatsDay(day: "2026-11-20", readingSeconds: 2400, listeningSeconds: 0)])
        XCTAssertEqual(shown.firstDay, "2026-01-02")
    }

    func testASessionPastMidnightSplitsBetweenBothDaysInTheServersZone() throws {
        // 11:40 PM to 12:20 AM in Chicago, 1,200 seconds listened: split in
        // proportion to the time on each side of local midnight.
        let late = session(1, "2026-11-11T05:40:00Z", "2026-11-11T06:20:00Z", seconds: 1200, listening: true)
        let parts = CalendarLayering.days(late, zone: chicago)
        XCTAssertEqual(parts.map(\.day), ["2026-11-10", "2026-11-11"])
        XCTAssertEqual(parts[0].seconds, 600, accuracy: 0.001)
        XCTAssertEqual(parts[1].seconds, 600, accuracy: 0.001)

        let shown = try XCTUnwrap(CalendarLayering.heatmap(heatmap(days: []), year: nil, today: "2026-11-20", sessions: [late],
                                                           finishes: [], zone: chicago))
        XCTAssertEqual(shown.days.map(\.day), ["2026-11-10", "2026-11-11"])
        XCTAssertEqual(shown.days.map(\.listeningSeconds), [600, 600])

        // In UTC the same Session is all on one day.
        XCTAssertEqual(CalendarLayering.days(late, zone: utc).map(\.day), ["2026-11-11"])
    }

    func testAStaleCopyMovesOnToTodaySoTodaysReadingShows() throws {
        let copy = heatmap(days: [Shishoapp.StatsDay(day: "2025-11-20", readingSeconds: 60, listeningSeconds: 0),
                                  Shishoapp.StatsDay(day: "2026-11-01", readingSeconds: 60, listeningSeconds: 0)],
                           today: "2026-11-19")
        let shown = try XCTUnwrap(CalendarLayering.heatmap(
            copy, year: nil, today: "2026-11-21", sessions: [session(1, "2026-11-21T10:00:00Z", "2026-11-21T10:10:00Z")],
            finishes: [], zone: utc))
        XCTAssertEqual(shown.today, "2026-11-21")
        XCTAssertEqual(shown.from, "2025-11-22")
        XCTAssertEqual(shown.to, "2026-11-21")
        XCTAssertEqual(shown.days.map(\.day), ["2026-11-01", "2026-11-21"], "a day now outside the 12 months drops off")
    }

    func testTheLast12MonthsBeginAsTheServersDo() {
        XCTAssertEqual(CalendarLayering.window(year: nil, today: "2026-11-13").from, "2025-11-14")
        // A year back from February 29 is March 1, as Go's AddDate gives.
        XCTAssertEqual(CalendarLayering.window(year: nil, today: "2028-02-28").from, "2027-03-01")
        XCTAssertEqual(CalendarLayering.window(year: 2026, today: "2026-11-13").from, "2026-01-01")
        XCTAssertEqual(CalendarLayering.window(year: 2026, today: "2026-11-13").to, "2026-12-31")
    }

    // MARK: - What the copy counts

    private func snapshot(_ id: String, sitting: String, end: String, accepted: Double?) -> SessionSnapshot {
        SessionSnapshot(eventID: id, payload: Shishoapp.JournalSessionPayload(
            sessionId: sitting, fileId: 10, startedAt: "2026-11-20T10:00:00Z", endedAt: end, startPercentage: 0, endPercentage: 0.5
        ), acceptedAt: accepted)
    }

    func testASessionTheCopyCountsIsDropped() {
        let fetchedAt = 1000.0
        let uncounted = UnsyncedReading.uncounted([
            // Taken in before the copy was fetched: the copy counts it.
            snapshot("e1", sitting: "counted", end: "2026-11-20T10:30:00Z", accepted: 900),
            // Taken in after, and still to send: not counted.
            snapshot("e2", sitting: "after", end: "2026-11-20T10:30:00Z", accepted: 1100),
            snapshot("e3", sitting: "unsent", end: "2026-11-20T10:30:00Z", accepted: nil),
        ], counted: [:], fetchedAt: fetchedAt)
        XCTAssertEqual(uncounted.map(\.key), ["after", "unsent"])
        XCTAssertEqual(uncounted.map(\.counted), [0, 0])
    }

    func testALaterSnapshotOfACountedSittingAddsOnlyWhatsNewAtItsEnd() throws {
        let uncounted = UnsyncedReading.uncounted([
            snapshot("e1", sitting: "sit", end: "2026-11-20T10:30:00Z", accepted: 900),
            snapshot("e2", sitting: "sit", end: "2026-11-20T11:00:00Z", accepted: nil),
        ], counted: [:], fetchedAt: 1000)
        let sitting = try XCTUnwrap(uncounted.first)
        XCTAssertEqual(sitting.counted, 1800)
        XCTAssertEqual(sitting.seconds, 3600)

        // The half the copy counts stays out; the rest goes where it was
        // read, from 10:30 on.
        let late = session(1, "2026-11-20T10:00:00Z", "2026-11-20T11:00:00Z", counted: 1800)
        XCTAssertEqual(late.uncountedStart, instant("2026-11-20T10:30:00Z"))
        let shown = try XCTUnwrap(CalendarLayering.heatmap(
            heatmap(days: [Shishoapp.StatsDay(day: "2026-11-20", readingSeconds: 1800, listeningSeconds: 0)]),
            year: nil, today: "2026-11-20", sessions: [late], finishes: [], zone: utc))
        XCTAssertEqual(shown.days.first?.readingSeconds, 3600)

        let day = try XCTUnwrap(CalendarLayering.day(nil, day: "2026-11-20", sessions: [late], finishes: [], zone: utc,
                                                     deviceID: "me", deviceName: "iPhone"))
        XCTAssertEqual(day.response.sessions.first?.startedAt, "2026-11-20T10:30:00Z")
        XCTAssertEqual(day.response.sessions.first?.seconds, 1800)
        XCTAssertEqual(day.response.sessions.first?.startPercentage, 0.375,
                       "from where the counted half ended, halfway from 25% to 50%, so the rows don't overlap")
    }

    func testAFullyCountedSittingAddsNothing() {
        let counted = session(1, "2026-11-20T10:00:00Z", "2026-11-20T10:30:00Z", counted: 1800)
        XCTAssertTrue(CalendarLayering.days(counted, zone: utc).isEmpty)
        XCTAssertNil(CalendarLayering.month(nil, year: 2026, month: 11, sessions: [counted], finishes: [], zone: utc))
    }

    // MARK: - The month and the day

    func testASessionAddsItsBookToItsDaysOfTheMonth() throws {
        let copy = Shishoapp.StatsCalendarResponse(
            year: 2026, month: 11, books: [Shishoapp.StatsBookRef(key: "book-2", title: "Another", bookId: 2)],
            days: [Shishoapp.StatsCalendarDay(day: "2026-11-10", books: [Shishoapp.StatsCalendarDayBook(key: "book-2", seconds: 300)])]
        )
        let shown = try XCTUnwrap(CalendarLayering.month(
            copy, year: 2026, month: 11,
            sessions: [session(1, "2026-11-11T05:40:00Z", "2026-11-11T06:20:00Z"),
                       // October's reading stays out of November.
                       session(3, "2026-10-01T10:00:00Z", "2026-10-01T10:10:00Z")],
            finishes: [], zone: chicago))
        XCTAssertEqual(shown.books.map(\.key), ["book-2", "book-1"], "sorted by title as the server sorts them")
        XCTAssertEqual(shown.days.map(\.day), ["2026-11-10", "2026-11-11"])
        XCTAssertEqual(shown.days[0].books, [Shishoapp.StatsCalendarDayBook(key: "book-2", seconds: 300),
                                             Shishoapp.StatsCalendarDayBook(key: "book-1", seconds: 1200)])
        XCTAssertEqual(shown.days[1].books, [Shishoapp.StatsCalendarDayBook(key: "book-1", seconds: 1200)])
    }

    func testAFinishShowsOnItsDayInTheMonthAndTheDay() throws {
        let month = try XCTUnwrap(CalendarLayering.month(
            Shishoapp.StatsCalendarResponse(year: 2026, month: 11), year: 2026, month: 11,
            sessions: [session(1, "2026-11-20T10:00:00Z", "2026-11-20T10:30:00Z")],
            finishes: [finish(1, "2026-11-20"), finish(4, "2026-11-21"), finish(5, "2026-12-01")], zone: utc))
        XCTAssertEqual(month.days, [
            Shishoapp.StatsCalendarDay(day: "2026-11-20", books: [Shishoapp.StatsCalendarDayBook(key: "book-1", seconds: 1800, finished: true)]),
            Shishoapp.StatsCalendarDay(day: "2026-11-21", books: [Shishoapp.StatsCalendarDayBook(key: "book-4", seconds: 0, finished: true)]),
        ])
        XCTAssertEqual(Set(month.books.map(\.key)), ["book-1", "book-4"])
        // A finish with no reading that day is a marker on its own.
        let week = CalendarLayout.monthWeeks(year: 2026, month: 11)[2]
        let layout = CalendarLayout.layoutWeek(week, days: Dictionary(uniqueKeysWithValues: month.days.map { ($0.day, $0) }))
        XCTAssertEqual(layout.items.first { $0.key == "book-4" }?.markerOnly, true)
        XCTAssertEqual(layout.items.first { $0.key == "book-1" }?.finishes, [5])

        let copy = Shishoapp.StatsDayResponse(day: "2026-11-21", timeZone: "UTC",
                                              finished: [Shishoapp.StatsBookRef(key: "book-9", title: "Synced", bookId: 9)])
        let day = try XCTUnwrap(CalendarLayering.day(copy, day: "2026-11-21", sessions: [],
                                                     finishes: [finish(4, "2026-11-21"), finish(9, "2026-11-21")],
                                                     zone: utc, deviceID: "me", deviceName: nil))
        XCTAssertEqual(day.response.finished.map(\.key), ["book-9", "book-4"], "a finish the copy lists isn't listed twice")
        XCTAssertEqual(day.pendingFinished, ["book-4"])
    }

    func testTheDayListsASessionAsThisDevicesNotSyncedYet() throws {
        let copy = Shishoapp.StatsDayResponse(day: "2026-11-20", timeZone: "America/Chicago", sessions: [
            Shishoapp.StatsDaySession(id: 7, book: Shishoapp.StatsBookRef(key: "book-2", title: "Another", bookId: 2),
                                      startedAt: "2026-11-20T20:00:00Z", endedAt: "2026-11-20T20:30:00Z", seconds: 1800,
                                      kind: .reading, source: .webReader, measured: true),
        ])
        let detail = try XCTUnwrap(CalendarLayering.day(
            copy, day: "2026-11-20",
            sessions: [session(1, "2026-11-20T14:00:00Z", "2026-11-20T14:30:00Z", seconds: 1500, listening: true),
                       // The next day in Chicago.
                       session(1, "2026-11-21T07:00:00Z", "2026-11-21T07:30:00Z")],
            finishes: [], zone: utc, deviceID: "me", deviceName: "Robin's iPhone"))
        XCTAssertEqual(detail.response.sessions.map(\.id), [-1, 7], "in the order they started")
        XCTAssertEqual(detail.pendingSessions, [-1])
        let added = detail.response.sessions[0]
        XCTAssertEqual(added.book.key, "book-1")
        XCTAssertEqual(added.startedAt, "2026-11-20T14:00:00Z")
        XCTAssertEqual(added.endedAt, "2026-11-20T14:30:00Z")
        XCTAssertEqual(added.seconds, 1500)
        XCTAssertEqual(added.startPercentage, 0.25)
        XCTAssertEqual(added.endPercentage, 0.5)
        XCTAssertEqual(added.kind, .listening)
        XCTAssertEqual(added.source, .shishoApp)
        XCTAssertEqual(added.deviceId, "me")
        XCTAssertEqual(added.deviceName, "Robin's iPhone")
        XCTAssertEqual(CalendarText.source(added), "Robin's iPhone")
    }

    func testABooksColorStaysAsItWillBeOnceTheServerCountsIt() throws {
        // book-0 and book-11 share a color; book-1 has its own.
        let copy = Shishoapp.StatsCalendarResponse(
            year: 2026, month: 11,
            books: [Shishoapp.StatsBookRef(key: "book-0", title: "A", bookId: 0), Shishoapp.StatsBookRef(key: "book-1", title: "B", bookId: 1)],
            days: [Shishoapp.StatsCalendarDay(day: "2026-11-02", books: [Shishoapp.StatsCalendarDayBook(key: "book-0", seconds: 60),
                                                                          Shishoapp.StatsCalendarDayBook(key: "book-1", seconds: 60)])]
        )
        let before = CalendarLayout.assignBookColors(copy.books.map(\.key))
        let shown = try XCTUnwrap(CalendarLayering.month(copy, year: 2026, month: 11,
                                                         sessions: [session(11, "2026-11-20T10:00:00Z", "2026-11-20T10:30:00Z")],
                                                         finishes: [], zone: utc))
        let colors = CalendarLayout.assignBookColors(shown.books.map(\.key))
        XCTAssertEqual(colors, ["book-0": "#c2570c", "book-1": "#23822b", "book-11": "#c22b2b"])
        XCTAssertEqual(colors["book-0"], before["book-0"])
        XCTAssertEqual(colors["book-1"], before["book-1"])
        // Once synced, the server names the Book by the same key, so its
        // color doesn't change.
        XCTAssertEqual(CalendarLayout.assignBookColors(["book-11", "book-1", "book-0"]), colors)
    }

    // MARK: - Never fetched

    func testAPartNeverFetchedShowsThisDevicesReadingAlone() throws {
        let reading = [session(1, "2026-11-20T10:00:00Z", "2026-11-20T10:30:00Z")]
        let shown = try XCTUnwrap(CalendarLayering.heatmap(nil, year: nil, today: "2026-11-20", sessions: reading,
                                                           finishes: [], zone: utc))
        XCTAssertEqual(shown.from, "2025-11-21")
        XCTAssertEqual(shown.to, "2026-11-20")
        XCTAssertEqual(shown.firstDay, "2026-11-20")
        XCTAssertEqual(shown.days, [Shishoapp.StatsDay(day: "2026-11-20", readingSeconds: 1800, listeningSeconds: 0)])
        let year = try XCTUnwrap(CalendarLayering.heatmap(nil, year: 2025, today: "2026-11-20", sessions: reading,
                                                          finishes: [], zone: utc))
        XCTAssertEqual(year.from, "2025-01-01")
        XCTAssertTrue(year.days.isEmpty, "the year shows, empty, so the calendar can reach the reading")

        let month = try XCTUnwrap(CalendarLayering.month(nil, year: 2026, month: 11, sessions: reading, finishes: [], zone: utc))
        XCTAssertEqual(month.books.map(\.title), ["Book 1"])
        let day = try XCTUnwrap(CalendarLayering.day(nil, day: "2026-11-20", sessions: reading, finishes: [], zone: utc,
                                                     deviceID: "me", deviceName: "iPhone"))
        // The zone's own name for itself, which iOS gives UTC as "GMT".
        XCTAssertEqual(day.response.timeZone, utc.identifier)
        XCTAssertEqual(day.pendingSessions, [-1])

        // With nothing unsynced, a part never fetched has nothing to show.
        XCTAssertNil(CalendarLayering.heatmap(nil, year: nil, today: "2026-11-20", sessions: [], finishes: [], zone: utc))
        XCTAssertNil(CalendarLayering.month(nil, year: 2026, month: 10, sessions: reading, finishes: [], zone: utc))
        XCTAssertNil(CalendarLayering.day(nil, day: "2026-11-19", sessions: reading, finishes: [], zone: utc, deviceID: "me",
                                          deviceName: nil))
    }
}

/// The calendar read from the account's database: each part's copy with the
/// journal's sessions and finishes laid over it, by the copy's fetched_at.
final class ReadingCalendarStoreTests: XCTestCase {
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
        let stats = try makeShishoDecoder().decode(Shishoapp.StatsResponse.self, from: Data("""
        {"year":null,"years":[],"time_zone":"UTC","totals":{"finished":0,"rereads":0,"did_not_finish":0,"reading_seconds":0,
          "listening_seconds":0,"words_read":0,"average_rating":null},"pace":{},"periods":[],"genres":[],"tags":[],"authors":[],
          "kinds":[],"ratings":[],"places":[{"source":"shisho_app","device_id":"me","device_name":"Kitchen iPad",
          "reading_seconds":0,"listening_seconds":0}]}
        """.utf8))
        try store.saveStats(stats, year: nil, fetchedAt: start)
    }

    private func answerAccepted() throws {
        let rows = try store.pendingJournal(capabilities: "", limit: 500)
        try store.applyJournalResults(rows.map { Shishoapp.JournalResult(id: $0.eventID, result: .accepted, reason: "") },
                                      sent: rows, capabilities: "")
    }

    private func listen(_ sitting: String, seconds: Int, from: Date) throws {
        try store.saveSitting(nil, recording: FinishedSession(sittingID: sitting, fileID: 10, startedAt: from,
                                                              endedAt: from.addingTimeInterval(Double(seconds)), startPercentage: 0,
                                                              endPercentage: 0.5, duration: seconds))
    }

    private func heatmap(now: Date) throws -> CalendarShown<Shishoapp.StatsHeatmapResponse>? {
        try store.pool.read { try CatalogStore.calendarHeatmap($0, year: nil, now: now) }
    }

    func testTheCalendarCountsASessionOnceTheCopyHasIt() throws {
        let copy = Shishoapp.StatsHeatmapResponse(today: "2027-01-15", from: "2026-01-16", to: "2027-01-15",
                                                  days: [Shishoapp.StatsDay(day: "2027-01-15", readingSeconds: 0, listeningSeconds: 600)])
        try store.saveCalendar(copy, key: .heatmap(year: nil), fetchedAt: start)
        try listen("sit-1", seconds: 300, from: start)
        var shown = try XCTUnwrap(heatmap(now: start))
        XCTAssertFalse(shown.deviceOnly)
        XCTAssertEqual(shown.value.days.first?.listeningSeconds, 900)

        // Taken in, then fetched again: the new copy counts it.
        try answerAccepted()
        let refetched = Date()
        var counted = copy
        counted.days[0].listeningSeconds = 900
        try store.saveCalendar(counted, key: .heatmap(year: nil), fetchedAt: refetched)
        shown = try XCTUnwrap(heatmap(now: start))
        XCTAssertEqual(shown.value.days.first?.listeningSeconds, 900, "not counted twice once the copy has it")
    }

    func testAnOldCalendarCopyIsFetchedAgainBeforeReadingStatsStopLayingASessionOverIt() throws {
        // The month was fetched before the session synced.
        let month = Shishoapp.StatsCalendarResponse(year: 2027, month: 1)
        try store.saveCalendar(month, key: .month(year: 2027, month: 1), fetchedAt: Date().addingTimeInterval(-60))
        try listen("sit-2", seconds: 600, from: start)
        // The sync notes the session's days as it sends it.
        try store.markCalendarStale(store.pendingJournal(capabilities: "", limit: 500))
        try answerAccepted()
        func shownSeconds() throws -> Int? {
            try XCTUnwrap(store.pool.read { try CatalogStore.calendarMonth($0, year: 2027, month: 1) }).value.days.first?.books.first?.seconds
        }
        XCTAssertEqual(try shownSeconds(), 600, "laid over the old copy until it's fetched again")

        let stats = try store.pool.read { try XCTUnwrap(CatalogStore.stats($0, year: nil, ownDeviceID: "me")) }.stats
        try store.saveStats(stats, year: nil, fetchedAt: Date())
        XCTAssertEqual(try store.calendarCopiesToRefresh(), [.month(year: 2027, month: 1)])
        // Fetched again, the month counts the session itself.
        var fetched = month
        fetched.books = [Shishoapp.StatsBookRef(key: "book-1", title: "Book 1", bookId: 1)]
        fetched.days = [Shishoapp.StatsCalendarDay(day: "2027-01-15", books: [Shishoapp.StatsCalendarDayBook(key: "book-1", seconds: 600)])]
        try store.saveCalendar(fetched, key: .month(year: 2027, month: 1), fetchedAt: Date())
        try store.calendarRefreshed(before: Date().addingTimeInterval(1))
        try store.statsCaughtUp()

        let unstated = try store.pool.read { try Int.fetchOne($0, sql: "SELECT COUNT(*) FROM unstated_sessions") }
        XCTAssertEqual(unstated, 0, "the calendar holds nothing back")
        XCTAssertEqual(try shownSeconds(), 600, "counted once")
    }

    func testADayNeverFetchedShowsThisDevicesReadingAndFinishes() throws {
        try listen("sit-3", seconds: 600, from: start)
        try store.record(.finish(book: 2), at: start)
        let endDay = try XCTUnwrap(store.pool.read { try String.fetchOne($0, sql: "SELECT end_day FROM read_throughs WHERE book_id = 2") })

        let shown = try XCTUnwrap(heatmap(now: start))
        XCTAssertTrue(shown.deviceOnly)
        XCTAssertEqual(shown.value.days, [Shishoapp.StatsDay(day: "2027-01-15", readingSeconds: 0, listeningSeconds: 600)])

        let day = try XCTUnwrap(store.pool.read {
            try CatalogStore.calendarDay($0, day: "2027-01-15", ownDeviceID: "me", deviceName: "iPhone")
        })
        XCTAssertTrue(day.deviceOnly)
        XCTAssertEqual(day.value.response.sessions.map(\.book.title), ["Book 1"])
        XCTAssertEqual(day.value.response.sessions.first?.deviceName, "Kitchen iPad", "the server's name for this Device")
        XCTAssertEqual(day.value.response.sessions.first?.kind, .listening)

        let finishDay = try XCTUnwrap(store.pool.read {
            try CatalogStore.calendarDay($0, day: endDay, ownDeviceID: "me", deviceName: "iPhone")
        })
        XCTAssertEqual(finishDay.value.pendingFinished, ["book-2"])
    }
}
