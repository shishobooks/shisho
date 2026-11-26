import Foundation
import ShishoAPI
import SwiftUI

/// The web's Reading Stats calendar helpers (`app/utils/readingCalendar.ts`),
/// ported so a Book takes the same color and the same lane in the app as on
/// the web. Days are YYYY-MM-DD strings, as the server sends them.
enum CalendarLayout {
    // MARK: - Book colors

    /// The colors a Book's bars take, in the web's order: twelve bright ones
    /// that differ in lightness as well as hue. The light ones carry dark
    /// text (`textOn`).
    static let bookColors: [String] = [
        "#1f6fb2", // blue
        "#c2570c", // orange
        "#23822b", // green
        "#c22b2b", // red
        "#7445b0", // purple
        "#facc15", // yellow
        "#b8337f", // pink
        "#566570", // slate
        "#84cc16", // lime
        "#11808f", // teal
        "#3445a6", // indigo
        "#38bdf8", // sky
    ]

    static let darkText = "#1a1a1a"
    private static let takesDarkText: Set<String> = ["#facc15", "#84cc16", "#38bdf8"]

    /// A Book's color, or its text color, as SwiftUI draws it.
    static func color(_ value: String) -> Color {
        value == "white" ? .white : Color(uiColor: UIColor(hex: value))
    }

    /// The text color that reads on a Book's color: white, or near-black on
    /// the light ones.
    static func textOn(_ background: String) -> String {
        takesDarkText.contains(background) ? darkText : "white"
    }

    /// FNV-1a over the key's UTF-16 code units, as the web's `charCodeAt`
    /// loop with `Math.imul`.
    static func colorIndex(_ key: String) -> Int {
        var hash: UInt32 = 0x811C_9DC5
        for unit in key.utf16 {
            hash ^= UInt32(unit)
            hash = hash &* 0x0100_0193
        }
        return Int(hash % UInt32(bookColors.count))
    }

    /// A Book's own color, from its calendar key.
    static func bookColor(_ key: String) -> String {
        bookColors[colorIndex(key)]
    }

    /// JavaScript's default sort order: by UTF-16 code units.
    static func jsLess(_ a: String, _ b: String) -> Bool {
        a.utf16.lexicographicallyPrecedes(b.utf16)
    }

    /// Colors for the Books shown together, by key: each keeps its own,
    /// except that when two share one, all but the first by key take the
    /// next free color. The result doesn't depend on the order of keys.
    static func assignBookColors(_ keys: [String]) -> [String: String] {
        let sorted = Set(keys).sorted(by: jsLess)
        var taken: [Int: String] = [:]
        var displaced: [String] = []
        for key in sorted {
            let index = colorIndex(key)
            if taken[index] != nil {
                displaced.append(key)
            } else {
                taken[index] = key
            }
        }
        var colors: [String: String] = [:]
        for (index, key) in taken { colors[key] = bookColors[index] }
        for key in displaced {
            var index = colorIndex(key)
            var step = 0
            while step < bookColors.count, taken[index] != nil {
                index = (index + 1) % bookColors.count
                step += 1
            }
            taken[index] = key
            colors[key] = bookColors[index]
        }
        return colors
    }

    // MARK: - Days and weeks

    private static let utc: Calendar = {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC")!
        return calendar
    }()

    /// The date of day at UTC midnight, or nil for something that isn't a
    /// real day, such as June 31.
    static func date(_ day: String) -> Date? {
        let parts = day.split(separator: "-").compactMap { Int($0) }
        guard parts.count == 3,
              let date = utc.date(from: DateComponents(year: parts[0], month: parts[1], day: parts[2])),
              format(date) == day
        else { return nil }
        return date
    }

    static func format(_ date: Date) -> String {
        let parts = utc.dateComponents([.year, .month, .day], from: date)
        return String(format: "%04d-%02d-%02d", parts.year ?? 0, parts.month ?? 0, parts.day ?? 0)
    }

    /// The calendar day by days after day.
    static func shiftDay(_ day: String, by days: Int) -> String {
        guard let date = date(day), let shifted = utc.date(byAdding: .day, value: days, to: date) else { return day }
        return format(shifted)
    }

    /// The year and month of a day.
    static func yearMonth(_ day: String) -> YearMonth {
        let parts = day.split(separator: "-").compactMap { Int($0) }
        return YearMonth(year: parts.first ?? 1970, month: parts.count > 1 ? parts[1] : 1)
    }

    /// A week, Sunday first, with nil for days outside the month or year.
    typealias Week = [String?]

    private static func weeksBetween(_ first: Date, _ last: Date) -> [Week] {
        var weeks: [Week] = []
        let weekday = utc.component(.weekday, from: first) - 1
        guard var cursor = utc.date(byAdding: .day, value: -weekday, to: first) else { return [] }
        while cursor <= last {
            var week: Week = []
            for _ in 0..<7 {
                week.append(cursor >= first && cursor <= last ? format(cursor) : nil)
                cursor = utc.date(byAdding: .day, value: 1, to: cursor) ?? cursor
            }
            weeks.append(week)
        }
        return weeks
    }

    /// The weeks of a month, 1 to 12, Sunday first.
    static func monthWeeks(year: Int, month: Int) -> [Week] {
        guard let first = utc.date(from: DateComponents(year: year, month: month, day: 1)),
              let next = utc.date(byAdding: .month, value: 1, to: first),
              let last = utc.date(byAdding: .day, value: -1, to: next)
        else { return [] }
        return weeksBetween(first, last)
    }

    /// The weeks from from to to, Sunday first, as the heatmap's columns.
    static func heatmapWeeks(from: String, to: String) -> [Week] {
        guard let first = date(from), let last = date(to) else { return [] }
        return weeksBetween(first, last)
    }

    // MARK: - A week's bars

    /// One bar, or a lone finish marker, in a week of the calendar.
    struct WeekItem: Equatable {
        let key: String
        /// The first and last columns it covers, 0 (Sunday) to 6.
        let start: Int
        let end: Int
        var lane: Int
        /// The run of reading carries on from the week before, or into the
        /// next.
        let continuesBefore: Bool
        let continuesAfter: Bool
        /// Columns where a Read-through of the Book finished.
        let finishes: [Int]
        /// A finish with no reading that day, which shows only its marker.
        let markerOnly: Bool
    }

    private static func readOn(_ days: [String: Shishoapp.StatsCalendarDay], _ day: String?, _ key: String) -> Bool {
        guard let day else { return false }
        return days[day]?.books.contains { $0.key == key && $0.seconds > 0 } ?? false
    }

    /// Lays out a week of the calendar: a bar for each run of consecutive
    /// days a Book was read, and a marker for a finish on a day it wasn't,
    /// each in the lowest lane free across its days. Items past maxLanes are
    /// left out and counted under each day they'd cover, in hidden; an item
    /// with a finish is placed first.
    static func layoutWeek(_ week: Week, days: [String: Shishoapp.StatsCalendarDay],
                           maxLanes: Int = .max) -> (items: [WeekItem], hidden: [Int]) {
        var keys: [String] = []
        for day in week {
            guard let day else { continue }
            for book in days[day]?.books ?? [] where !keys.contains(book.key) {
                keys.append(book.key)
            }
        }

        var items: [WeekItem] = []
        for key in keys {
            func entry(_ col: Int) -> Shishoapp.StatsCalendarDayBook? {
                guard let day = week[col] else { return nil }
                return days[day]?.books.first { $0.key == key }
            }
            var col = 0
            while col < 7 {
                guard let book = entry(col) else {
                    col += 1
                    continue
                }
                if book.seconds == 0 {
                    // Only a finish: a marker on its own.
                    if book.finished {
                        items.append(WeekItem(key: key, start: col, end: col, lane: 0, continuesBefore: false,
                                              continuesAfter: false, finishes: [col], markerOnly: true))
                    }
                    col += 1
                    continue
                }
                let start = col
                while col + 1 < 7, (entry(col + 1)?.seconds ?? 0) > 0 { col += 1 }
                let end = col
                let finishes = (start...end).filter { entry($0)?.finished == true }
                items.append(WeekItem(
                    key: key, start: start, end: end, lane: 0,
                    continuesBefore: start == 0 && week[0] != nil && readOn(days, week[0].map { shiftDay($0, by: -1) }, key),
                    continuesAfter: end == 6 && week[6] != nil && readOn(days, week[6].map { shiftDay($0, by: 1) }, key),
                    finishes: finishes, markerOnly: false
                ))
                col += 1
            }
        }

        // Finishes first, so a marker is never what spills past the last
        // lane; then earliest, then longest, so long runs settle in the top
        // lanes. Keys are ASCII ("book-12"), where the web's localeCompare
        // and code unit order agree.
        items.sort { a, b in
            let aFinishes = !a.finishes.isEmpty, bFinishes = !b.finishes.isEmpty
            if aFinishes != bFinishes { return aFinishes }
            if a.start != b.start { return a.start < b.start }
            if a.end != b.end { return a.end > b.end }
            return jsLess(a.key, b.key)
        }
        var lanes: [[Bool]] = []
        var placed: [WeekItem] = []
        var hidden = Array(repeating: 0, count: 7)
        for var item in items {
            let cols = Array(item.start...item.end)
            var lane = lanes.firstIndex { taken in cols.allSatisfy { !taken[$0] } } ?? -1
            if lane == -1 {
                lanes.append(Array(repeating: false, count: 7))
                lane = lanes.count - 1
            }
            for c in cols { lanes[lane][c] = true }
            if lane < maxLanes {
                item.lane = lane
                placed.append(item)
            } else {
                for c in cols { hidden[c] += 1 }
            }
        }
        placed.sort { $0.start != $1.start ? $0.start < $1.start : $0.lane < $1.lane }
        return (placed, hidden)
    }

    // MARK: - The heatmap

    /// Sorts a day's time into one of five shades: 0 for none, 4 for the
    /// most, and 1 to 3 by the quartiles of the days with any.
    static func heatLevels(_ values: [Int]) -> (Int) -> Int {
        let sorted = values.filter { $0 > 0 }.sorted()
        func at(_ q: Double) -> Int {
            guard !sorted.isEmpty else { return 0 }
            return sorted[max(0, Int((q * Double(sorted.count)).rounded(.up)) - 1)]
        }
        let q1 = at(0.25), q2 = at(0.5), q3 = at(0.75)
        let maximum = sorted.last ?? 0
        return { value in
            if value <= 0 { return 0 }
            if value >= maximum { return 4 }
            if value <= q1 { return 1 }
            if value <= q2 { return 2 }
            if value <= q3 { return 3 }
            return 4
        }
    }

    /// The heatmap's day at point, measured from the grid's top left corner
    /// with squares stride apart, or nil past the grid or on a blank square.
    static func heatmapDay(at point: CGPoint, in weeks: [Week], stride: CGFloat) -> String? {
        guard stride > 0, point.x >= 0, point.y >= 0 else { return nil }
        let col = Int(point.x / stride), row = Int(point.y / stride)
        guard col < weeks.count, row < 7 else { return nil }
        return weeks[col][row]
    }

    /// Where a tooltip of size goes for the square at anchor: above it,
    /// centered, when there's room, else beside it on the side with more
    /// room, so the finger on the square doesn't cover it. It stays inside
    /// bounds either way.
    static func tooltipOrigin(size: CGSize, anchor: CGRect, in bounds: CGRect, gap: CGFloat) -> CGPoint {
        func clamp(_ value: CGFloat, _ low: CGFloat, _ high: CGFloat) -> CGFloat {
            min(max(value, low), max(high, low))
        }
        if anchor.minY - gap - size.height >= bounds.minY {
            return CGPoint(x: clamp(anchor.midX - size.width / 2, bounds.minX, bounds.maxX - size.width),
                           y: anchor.minY - gap - size.height)
        }
        let roomAfter = bounds.maxX - anchor.maxX, roomBefore = anchor.minX - bounds.minX
        let x = roomAfter >= roomBefore ? anchor.maxX + gap : anchor.minX - gap - size.width
        return CGPoint(x: clamp(x, bounds.minX, bounds.maxX - size.width),
                       y: clamp(anchor.minY, bounds.minY, bounds.maxY - size.height))
    }

    // MARK: - A day's timeline

    static let minutesInDay = 24.0 * 60

    /// A Session placed on the day's timeline, in minutes from midnight.
    struct TimelineSession: Equatable, Identifiable {
        let session: Shishoapp.StatsDaySession
        let from: Double
        let to: Double
        var id: Int { session.id }
    }

    /// One Book's line on a day's timeline, and its time that day.
    struct DayLane: Equatable, Identifiable {
        let book: Shishoapp.StatsBookRef
        var seconds: Int
        var sessions: [TimelineSession]
        var id: String { book.key }
    }

    /// The instant day begins in zone: its local midnight.
    static func startOfDay(_ day: String, zone: TimeZone) -> Date? {
        let parts = day.split(separator: "-").compactMap { Int($0) }
        guard parts.count == 3 else { return nil }
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = zone
        guard let noon = calendar.date(from: DateComponents(year: parts[0], month: parts[1], day: parts[2], hour: 12)) else {
            return nil
        }
        return calendar.startOfDay(for: noon)
    }

    /// An instant's place on day's 24-hour strip in zone, in minutes: the
    /// time since local midnight, scaled so a 23- or 25-hour day still fills
    /// the strip, and held to the day's edges.
    static func minutesInto(_ instant: Date, day: String, zone: TimeZone) -> Double {
        guard let start = startOfDay(day, zone: zone),
              let next = startOfDay(shiftDay(day, by: 1), zone: zone) else { return 0 }
        let dayMinutes = next.timeIntervalSince(start) / 60
        let minutes = instant.timeIntervalSince(start) / 60
        return min(max(minutes, 0), dayMinutes) * (minutesInDay / dayMinutes)
    }

    static func instant(_ value: String) -> Date? {
        CatalogStore.timestamp(value).map { Date(timeIntervalSince1970: $0) }
    }

    /// A day's Sessions as a line per Book, in the order each was first
    /// read, each Session placed in minutes from local midnight in zone and
    /// cut at the day's edges.
    static func dayLanes(_ sessions: [Shishoapp.StatsDaySession], day: String, zone: TimeZone) -> [DayLane] {
        let sorted = sessions.enumerated().sorted { a, b in
            let at = instant(a.element.startedAt) ?? .distantPast, bt = instant(b.element.startedAt) ?? .distantPast
            return at != bt ? at < bt : a.offset < b.offset
        }.map(\.element)
        var lanes: [DayLane] = []
        var index: [String: Int] = [:]
        for session in sorted {
            let i: Int
            if let found = index[session.book.key] {
                i = found
            } else {
                i = lanes.count
                index[session.book.key] = i
                lanes.append(DayLane(book: session.book, seconds: 0, sessions: []))
            }
            lanes[i].seconds += session.seconds
            lanes[i].sessions.append(TimelineSession(
                session: session,
                from: instant(session.startedAt).map { minutesInto($0, day: day, zone: zone) } ?? 0,
                to: instant(session.endedAt).map { minutesInto($0, day: day, zone: zone) } ?? 0
            ))
        }
        return lanes
    }
}

/// A calendar month, 1 to 12.
struct YearMonth: Hashable, Sendable, Comparable {
    let year: Int
    let month: Int

    var index: Int { year * 12 + month - 1 }

    init(year: Int, month: Int) {
        self.year = year
        self.month = month
    }

    init(index: Int) {
        year = Int((Double(index) / 12).rounded(.down))
        month = index - year * 12 + 1
    }

    static func < (lhs: YearMonth, rhs: YearMonth) -> Bool { lhs.index < rhs.index }
}
