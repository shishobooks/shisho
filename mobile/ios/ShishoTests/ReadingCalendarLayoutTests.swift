import ShishoAPI
import XCTest
@testable import Shisho

/// The calendar's layout helpers, ported from the web's
/// `app/utils/readingCalendar.ts`: the cases mirror its tests
/// (`readingCalendar.test.ts`), so a Book takes the same color and lane in
/// the app as on the web.
final class ReadingCalendarLayoutTests: XCTestCase {
    private func days(_ entries: [(String, [(String, Int, Bool)])]) -> [String: Shishoapp.StatsCalendarDay] {
        Dictionary(uniqueKeysWithValues: entries.map { day, books in
            (day, Shishoapp.StatsCalendarDay(day: day, books: books.map {
                Shishoapp.StatsCalendarDayBook(key: $0.0, seconds: $0.1, finished: $0.2)
            }))
        })
    }

    // MARK: - Book colors

    func testABookTakesTheWebsColor() {
        // The web's bookColor for each key, run through its own code.
        XCTAssertEqual(CalendarLayout.bookColor("book-1"), "#23822b")
        XCTAssertEqual(CalendarLayout.bookColor("book-2"), "#c22b2b")
        XCTAssertEqual(CalendarLayout.bookColor("book-12"), "#7445b0")
        XCTAssertEqual(CalendarLayout.bookColor("book-42"), "#facc15")
        XCTAssertEqual(CalendarLayout.bookColor("read-through-7"), "#11808f")
        XCTAssertEqual(CalendarLayout.bookColor("session-3"), "#38bdf8")
        // Hashed by UTF-16 code units, as charCodeAt reads them.
        XCTAssertEqual(CalendarLayout.bookColor("é-book"), "#b8337f")
        XCTAssertEqual(CalendarLayout.bookColor("📚"), "#b8337f")
    }

    func testBookColorsSpreadOverThePalette() {
        XCTAssertEqual(CalendarLayout.bookColor("book-12"), CalendarLayout.bookColor("book-12"))
        let colors = Set((0..<40).map { CalendarLayout.bookColor("book-\($0)") })
        XCTAssertGreaterThan(colors.count, CalendarLayout.bookColors.count / 2)
    }

    func testEveryBookColorCarriesTextThatMeets4_5To1() {
        func luminance(_ hex: String) -> Double {
            let value = Int(hex.dropFirst(), radix: 16) ?? 0
            let channels = [(value >> 16) & 0xFF, (value >> 8) & 0xFF, value & 0xFF].map { channel -> Double in
                let c = Double(channel) / 255
                return c <= 0.03928 ? c / 12.92 : pow((c + 0.055) / 1.055, 2.4)
            }
            return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
        }
        for color in CalendarLayout.bookColors {
            let text = CalendarLayout.textOn(color)
            let pair = [luminance(color), luminance(text == "white" ? "#ffffff" : text)].sorted(by: >)
            XCTAssertGreaterThanOrEqual((pair[0] + 0.05) / (pair[1] + 0.05), 4.5, color)
        }
    }

    func testBooksShownTogetherTakeDifferentColorsAsOnTheWeb() {
        let keys = (0..<12).map { "book-\($0)" }
        let colors = CalendarLayout.assignBookColors(keys)
        XCTAssertEqual(Set(keys.compactMap { colors[$0] }).count, keys.count)
        XCTAssertEqual(colors, [
            "book-0": "#c2570c", "book-1": "#23822b", "book-2": "#c22b2b", "book-3": "#7445b0",
            "book-4": "#facc15", "book-5": "#b8337f", "book-6": "#566570", "book-7": "#84cc16",
            "book-8": "#11808f", "book-9": "#3445a6", "book-10": "#38bdf8", "book-11": "#1f6fb2",
        ])
    }

    func testABookKeepsItsColorUnlessAnotherHereHasIt() {
        // book-0 and book-11 share a color; book-1 doesn't.
        XCTAssertEqual(CalendarLayout.bookColor("book-0"), CalendarLayout.bookColor("book-11"))
        XCTAssertEqual(CalendarLayout.assignBookColors(["book-0", "book-1"])["book-0"], CalendarLayout.bookColor("book-0"))
        XCTAssertEqual(CalendarLayout.assignBookColors(["book-11", "book-1"])["book-11"], CalendarLayout.bookColor("book-11"))
        XCTAssertEqual(CalendarLayout.assignBookColors(["book-11", "book-1", "book-0"]),
                       ["book-0": "#c2570c", "book-1": "#23822b", "book-11": "#c22b2b"])
    }

    func testColorsDontDependOnTheOrderOfBooks() {
        let keys = (0..<12).map { "book-\($0 * 7)" }
        XCTAssertEqual(CalendarLayout.assignBookColors(keys), CalendarLayout.assignBookColors(keys.reversed()))
        // JavaScript's sort puts "book-10" before "book-100" before "book-9".
        XCTAssertEqual(CalendarLayout.assignBookColors(["book-9", "book-100", "book-10"]),
                       ["book-10": "#23822b", "book-100": "#b8337f", "book-9": "#3445a6"])
    }

    // MARK: - Days and weeks

    func testShiftDayStepsAcrossMonthAndYearEnds() {
        XCTAssertEqual(CalendarLayout.shiftDay("2026-02-28", by: 1), "2026-03-01")
        XCTAssertEqual(CalendarLayout.shiftDay("2026-01-01", by: -1), "2025-12-31")
    }

    func testAMonthIsSundayFirstWeeksWithBlanksOutsideIt() {
        // April 2026 starts on a Wednesday and ends on a Thursday.
        let weeks = CalendarLayout.monthWeeks(year: 2026, month: 4)
        XCTAssertEqual(weeks.count, 5)
        XCTAssertEqual(weeks[0], [nil, nil, nil, "2026-04-01", "2026-04-02", "2026-04-03", "2026-04-04"])
        XCTAssertEqual(weeks[4], ["2026-04-26", "2026-04-27", "2026-04-28", "2026-04-29", "2026-04-30", nil, nil])
    }

    func testTheHeatmapCoversItsDaysInSundayFirstColumns() {
        // 2025-11-14 is a Friday and 2026-11-13 a Friday.
        let weeks = CalendarLayout.heatmapWeeks(from: "2025-11-14", to: "2026-11-13")
        XCTAssertEqual(weeks.first, [nil, nil, nil, nil, nil, "2025-11-14", "2025-11-15"])
        XCTAssertEqual(weeks.flatMap { $0 }.compactMap { $0 }.count, 365)
        XCTAssertEqual(weeks.last, ["2026-11-08", "2026-11-09", "2026-11-10", "2026-11-11", "2026-11-12", "2026-11-13", nil])
    }

    // MARK: - A week's bars

    private let week = CalendarLayout.monthWeeks(year: 2026, month: 4)[1] // April 5 to 11

    func testConsecutiveDaysOfABookJoinIntoOneBar() {
        let layout = CalendarLayout.layoutWeek(week, days: days([
            ("2026-04-06", [("book-1", 600, false)]),
            ("2026-04-07", [("book-1", 900, false)]),
            ("2026-04-08", [("book-1", 300, false)]),
            ("2026-04-10", [("book-1", 300, false)]),
        ]))
        XCTAssertEqual(layout.items.map { [$0.start, $0.end, $0.lane] }, [[1, 3, 0], [5, 5, 0]])
        XCTAssertEqual(layout.items.map(\.key), ["book-1", "book-1"])
    }

    func testABarStaysOpenWhereItsRunCarriesIntoAnotherWeek() {
        let layout = CalendarLayout.layoutWeek(week, days: days([
            ("2026-04-04", [("book-1", 600, false)]),
            ("2026-04-05", [("book-1", 600, false)]),
            ("2026-04-11", [("book-1", 600, false)]),
            ("2026-04-12", [("book-1", 600, false)]),
        ]))
        XCTAssertEqual(layout.items[0].start, 0)
        XCTAssertTrue(layout.items[0].continuesBefore)
        XCTAssertFalse(layout.items[0].continuesAfter)
        XCTAssertEqual(layout.items[1].start, 6)
        XCTAssertTrue(layout.items[1].continuesAfter)
        XCTAssertFalse(layout.items[1].continuesBefore)
    }

    func testOverlappingBooksStackAndFinishesAreMarked() {
        let layout = CalendarLayout.layoutWeek(week, days: days([
            ("2026-04-06", [("book-1", 600, false), ("book-2", 600, false)]),
            ("2026-04-07", [("book-1", 600, true), ("book-2", 600, false)]),
            ("2026-04-08", [("book-3", 0, true)]),
        ]))
        XCTAssertEqual(layout.items.map(\.key), ["book-1", "book-2", "book-3"])
        XCTAssertEqual(layout.items.map(\.lane), [0, 1, 0])
        XCTAssertEqual(layout.items.map(\.finishes), [[2], [], [3]])
        // A finish with no reading is a marker on its own.
        XCTAssertEqual(layout.items.map(\.markerOnly), [false, false, true])
    }

    func testWhatDoesntFitInTheLanesIsCountedUnderEachDay() {
        let layout = CalendarLayout.layoutWeek(week, days: days([
            ("2026-04-06", ["book-1", "book-2", "book-3", "book-4", "book-5"].map { ($0, 60, false) }),
        ]), maxLanes: 3)
        XCTAssertEqual(layout.items.count, 3)
        XCTAssertEqual(layout.hidden, [0, 2, 0, 0, 0, 0, 0])
    }

    func testAFinishIsNeverHiddenInTheOverflow() {
        let layout = CalendarLayout.layoutWeek(week, days: days([
            ("2026-04-07", [("book-9", 0, true)]),
            ("2026-04-06", ["book-1", "book-2", "book-3", "book-4"].map { ($0, 60, false) } + [("book-5", 60, true)]),
        ]), maxLanes: 3)
        XCTAssertTrue(layout.items.contains { $0.key == "book-5" })
        XCTAssertTrue(layout.items.contains { $0.key == "book-9" })
        XCTAssertEqual(layout.hidden[1], 2)
    }

    // MARK: - The heatmap

    func testTheDaysReadMostAreShadedDarkest() {
        let level = CalendarLayout.heatLevels([0, 60, 600, 1200, 3600, 7200])
        XCTAssertEqual(level(0), 0)
        XCTAssertEqual(level(60), 1)
        XCTAssertEqual(level(7200), 4)
        XCTAssertGreaterThan(level(3600), level(600))
    }

    func testEveryDayIsAtTheTopLevelWhenTheyreAllTheSame() {
        XCTAssertEqual(CalendarLayout.heatLevels([600, 600])(600), 4)
    }

    func testAPointOnTheGridFindsItsDay() {
        let weeks = CalendarLayout.heatmapWeeks(from: "2025-11-14", to: "2026-11-13")
        // Squares 19 points apart: column 0, row 5 is the first day.
        XCTAssertEqual(CalendarLayout.heatmapDay(at: CGPoint(x: 5, y: 5 * 19 + 3), in: weeks, stride: 19), "2025-11-14")
        XCTAssertEqual(CalendarLayout.heatmapDay(at: CGPoint(x: 19 + 1, y: 1), in: weeks, stride: 19), "2025-11-16")
        // A blank square, past the grid, or off its edges finds none.
        XCTAssertNil(CalendarLayout.heatmapDay(at: CGPoint(x: 5, y: 5), in: weeks, stride: 19))
        XCTAssertNil(CalendarLayout.heatmapDay(at: CGPoint(x: CGFloat(weeks.count) * 19 + 1, y: 5), in: weeks, stride: 19))
        XCTAssertNil(CalendarLayout.heatmapDay(at: CGPoint(x: 5, y: 7 * 19 + 1), in: weeks, stride: 19))
        XCTAssertNil(CalendarLayout.heatmapDay(at: CGPoint(x: -1, y: 5), in: weeks, stride: 19))
        XCTAssertNil(CalendarLayout.heatmapDay(at: CGPoint(x: 5, y: 5), in: weeks, stride: 0))
    }

    func testATooltipSitsAboveItsSquareWhenThereIsRoom() {
        let bounds = CGRect(x: 0, y: 0, width: 300, height: 200)
        let size = CGSize(width: 100, height: 40)
        let origin = CalendarLayout.tooltipOrigin(size: size, anchor: CGRect(x: 100, y: 100, width: 16, height: 16), in: bounds, gap: 6)
        XCTAssertEqual(origin, CGPoint(x: 58, y: 54), "centered above, a gap clear of the square")
        // Near an edge it's held inside.
        let left = CalendarLayout.tooltipOrigin(size: size, anchor: CGRect(x: 2, y: 100, width: 16, height: 16), in: bounds, gap: 6)
        XCTAssertEqual(left.x, 0)
    }

    func testATooltipGoesBesideItsSquareOnTheRoomierSideWithoutRoomAbove() {
        let bounds = CGRect(x: 0, y: 0, width: 300, height: 200)
        let size = CGSize(width: 100, height: 40)
        let right = CalendarLayout.tooltipOrigin(size: size, anchor: CGRect(x: 40, y: 10, width: 16, height: 16), in: bounds, gap: 6)
        XCTAssertEqual(right, CGPoint(x: 62, y: 10))
        let left = CalendarLayout.tooltipOrigin(size: size, anchor: CGRect(x: 250, y: 10, width: 16, height: 16), in: bounds, gap: 6)
        XCTAssertEqual(left, CGPoint(x: 144, y: 10))
        // Held inside the bottom edge.
        let low = CalendarLayout.tooltipOrigin(size: CGSize(width: 100, height: 300), anchor: CGRect(x: 40, y: 10, width: 16, height: 16),
                                               in: bounds, gap: 6)
        XCTAssertEqual(low.y, 0)
    }

    // MARK: - A day's timeline

    private func session(_ id: Int, _ key: String, _ start: String, _ end: String, _ seconds: Int) -> Shishoapp.StatsDaySession {
        Shishoapp.StatsDaySession(id: id, book: Shishoapp.StatsBookRef(key: key, title: key), startedAt: start, endedAt: end,
                                  seconds: seconds, kind: .reading, source: .webReader, measured: true)
    }

    private let chicago = TimeZone(identifier: "America/Chicago")!

    func testEachBookGetsALaneInTheOrderFirstRead() {
        // Chicago is UTC-6 in November.
        let lanes = CalendarLayout.dayLanes([
            session(1, "book-1", "2026-11-08T13:15:00Z", "2026-11-08T13:40:00Z", 1500),
            session(2, "book-2", "2026-11-08T14:15:00Z", "2026-11-08T15:12:00Z", 3420),
            session(3, "book-1", "2026-11-08T21:30:00Z", "2026-11-08T22:20:00Z", 3000),
        ], day: "2026-11-08", zone: chicago)
        XCTAssertEqual(lanes.map(\.book.key), ["book-1", "book-2"])
        XCTAssertEqual(lanes.map(\.seconds), [4500, 3420])
        XCTAssertEqual(lanes[0].sessions.map(\.id), [1, 3])
        XCTAssertEqual(lanes[0].sessions.map(\.from), [7 * 60 + 15, 15 * 60 + 30])
        XCTAssertEqual(lanes[0].sessions.map(\.to), [7 * 60 + 40, 16 * 60 + 20])
        XCTAssertEqual(lanes[1].sessions.map(\.from), [8 * 60 + 15])
        XCTAssertEqual(lanes[1].sessions.map(\.to), [9 * 60 + 12])
    }

    func testASessionAcrossADaylightSavingChangeIsPlacedByTheTimeItTook() throws {
        // November 1, 2026 is 25 hours long in Chicago.
        let lanes = CalendarLayout.dayLanes([session(1, "book-1", "2026-11-01T06:50:00Z", "2026-11-01T07:10:00Z", 1200)],
                                            day: "2026-11-01", zone: chicago)
        let placed = try XCTUnwrap(lanes.first?.sessions.first)
        let scale = 1440.0 / (25 * 60)
        XCTAssertEqual(placed.from, 110 * scale, accuracy: 0.001)
        XCTAssertEqual(placed.to, 130 * scale, accuracy: 0.001)
    }

    func testASessionPastMidnightIsCutAtTheEdgeOfTheDay() throws {
        let night = session(1, "book-1", "2026-11-11T05:40:00Z", "2026-11-11T06:20:00Z", 1200)
        let before = try XCTUnwrap(CalendarLayout.dayLanes([night], day: "2026-11-10", zone: chicago).first?.sessions.first)
        XCTAssertEqual(before.from, 23 * 60 + 40, accuracy: 0.001)
        XCTAssertEqual(before.to, 24 * 60, accuracy: 0.001)
        let after = try XCTUnwrap(CalendarLayout.dayLanes([night], day: "2026-11-11", zone: chicago).first?.sessions.first)
        XCTAssertEqual(after.from, 0, accuracy: 0.001)
        XCTAssertEqual(after.to, 20, accuracy: 0.001)
    }
}
