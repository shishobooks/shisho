import Foundation
import ShishoAPI

/// A sitting of this device's reading that a copy of the calendar doesn't
/// count all of (UnsyncedReading.uncounted), with its Book as the calendar
/// names it.
struct UnsyncedCalendarSession: Equatable, Sendable {
    let book: Shishoapp.StatsBookRef
    let listening: Bool
    let startedAt: Date
    let endedAt: Date
    /// Its time: the duration recorded, or the time between its ends.
    let seconds: Double
    /// The seconds of it the copy already counts, from its start.
    var counted: Double = 0
    var startPercentage: Double = 0
    var endPercentage: Double = 0

    /// The time the copy doesn't count.
    var uncountedSeconds: Double { max(0, seconds - counted) }

    /// Where the time the copy doesn't count begins: past the share of the
    /// sitting an earlier snapshot already brought.
    /// How far through the File the time the copy doesn't count begins, in
    /// proportion, as uncountedStart places it in time, so its row in the
    /// day doesn't overlap the copy's.
    var uncountedStartPercentage: Double {
        guard seconds > 0, counted > 0 else { return startPercentage }
        return startPercentage + (endPercentage - startPercentage) * min(counted / seconds, 1)
    }

    var uncountedStart: Date {
        UnsyncedReading.uncountedStart(start: startedAt, end: endedAt, seconds: seconds, counted: counted)
    }
}

/// A Read-through finished on this device that the server hasn't confirmed.
struct UnsyncedFinish: Equatable, Sendable {
    let book: Shishoapp.StatsBookRef
    let day: String
}

/// One day of the calendar as shown, with which of its Sessions and
/// finishes are this device's, not synced yet.
struct CalendarDayDetail: Equatable, Sendable {
    var response: Shishoapp.StatsDayResponse
    /// Ids of the Sessions laid over the copy; they're negative, so they
    /// never meet a server's.
    var pendingSessions: Set<Int> = []
    /// Keys of the Books whose finish was laid over the copy.
    var pendingFinished: Set<String> = []
}

/// A part of the calendar as shown: the copy last fetched with this device's
/// reading not synced yet laid over it, or, for a part never fetched, that
/// reading alone (deviceOnly).
struct CalendarShown<Value: Sendable & Equatable>: Sendable, Equatable {
    let value: Value
    let deviceOnly: Bool
}

/// Lays this device's reading the server hasn't counted over the copies of
/// Reading Stats' Calendar, as the Overview lays it over Reading Stats:
/// Session time on the days of zone it fell on, split at midnight as the
/// server splits it, and finishes on their days.
enum CalendarLayering {
    /// The days the uncounted time of session falls on in zone.
    static func days(_ session: UnsyncedCalendarSession, zone: TimeZone) -> [(day: String, seconds: Double)] {
        let added = session.uncountedSeconds
        guard added > 0 else { return [] }
        return UnsyncedReading.splitByDay(start: session.uncountedStart, end: session.endedAt, seconds: added, zone: zone)
    }

    /// The days of zone the changes in rows touch on the calendar: every day
    /// a Reading Session falls on, split at midnight as the server splits
    /// it, and the day a Read-through is finished or abandoned on, or, for
    /// one deleted, the day it ended (endDays, by its id).
    static func touchedDays(_ rows: [JournalRow], zone: TimeZone, endDays: [Int: String]) -> Set<String> {
        let decoder = makeShishoDecoder()
        var days = Set<String>()
        for row in rows {
            switch Shishoapp.JournalEventType(rawValue: row.type) {
            case .readingSession:
                guard let session = try? decoder.decode(Shishoapp.JournalSessionPayload.self, from: row.payload),
                      let started = CatalogStore.timestamp(session.startedAt) else { continue }
                let start = Date(timeIntervalSince1970: started)
                let end = CatalogStore.timestamp(session.endedAt).map(Date.init(timeIntervalSince1970:)) ?? start
                for part in UnsyncedReading.splitByDay(start: start, end: end, seconds: 1, zone: zone) {
                    days.insert(part.day)
                }
            case .readThroughFinish, .readThroughAbandon:
                // The phone's own day, as the server keeps it.
                guard let change = try? decoder.decode(Shishoapp.JournalBookPayload.self, from: row.payload) else { continue }
                days.insert(change.day.isEmpty ? CatalogStore.day(row.at) : change.day)
            case .readThroughDelete:
                guard let change = try? decoder.decode(Shishoapp.JournalReadThroughDeletePayload.self, from: row.payload),
                      let id = change.readThroughId, let day = endDays[id] else { continue }
                days.insert(day)
            default:
                continue
            }
        }
        return days
    }

    /// The heatmap's days for year, or for the 12 months to today when nil,
    /// as the server picks them.
    static func window(year: Int?, today: String) -> (from: String, to: String) {
        if let year {
            return (String(format: "%04d-01-01", year), String(format: "%04d-12-31", year))
        }
        // A year back from tomorrow, and from March 1 when that would be
        // February 29 of a year without one, as Go's AddDate does.
        let next = CalendarLayout.shiftDay(today, by: 1)
        let parts = next.split(separator: "-").compactMap { Int($0) }
        guard parts.count == 3 else { return (today, today) }
        let from = String(format: "%04d-%02d-%02d", parts[0] - 1, parts[1], parts[2])
        return (CalendarLayout.date(from) == nil ? String(format: "%04d-03-01", parts[0] - 1) : from, today)
    }

    // MARK: - The heatmap

    /// The heatmap for year (nil for the last 12 months) with sessions laid
    /// over copy, reading and listening apart. today is the day it is now in
    /// zone: a copy fetched on an earlier day moves on to it, the last 12
    /// months sliding with it. Without a copy, the heatmap holds sessions
    /// alone, or is nil when there's nothing unsynced to show.
    static func heatmap(_ copy: Shishoapp.StatsHeatmapResponse?, year: Int?, today: String,
                        sessions: [UnsyncedCalendarSession], finishes: [UnsyncedFinish],
                        zone: TimeZone) -> Shishoapp.StatsHeatmapResponse? {
        let placed = sessions.map { days($0, zone: zone) }
        let anything = placed.contains { !$0.isEmpty } || !finishes.isEmpty
        var shown: Shishoapp.StatsHeatmapResponse
        if let copy {
            shown = copy
            if today > copy.today {
                shown.today = today
                if year == nil {
                    (shown.from, shown.to) = window(year: nil, today: today)
                    shown.days = shown.days.filter { $0.day >= shown.from && $0.day <= shown.to }
                }
            }
        } else {
            guard anything else { return nil }
            let (from, to) = window(year: year, today: today)
            shown = Shishoapp.StatsHeatmapResponse(today: today, from: from, to: to, firstDay: nil, days: [])
        }

        var reading: [String: Double] = [:], listening: [String: Double] = [:]
        var first = shown.firstDay
        for (session, parts) in zip(sessions, placed) {
            for part in parts {
                if first.map({ part.day < $0 }) ?? true { first = part.day }
                guard part.day >= shown.from, part.day <= shown.to else { continue }
                if session.listening { listening[part.day, default: 0] += part.seconds } else { reading[part.day, default: 0] += part.seconds }
            }
        }
        for finish in finishes where first.map({ finish.day < $0 }) ?? true { first = finish.day }
        shown.firstDay = first

        var byDay = Dictionary(shown.days.map { ($0.day, $0) }, uniquingKeysWith: { first, _ in first })
        for day in Set(reading.keys).union(listening.keys) {
            let addReading = Int((reading[day] ?? 0).rounded()), addListening = Int((listening[day] ?? 0).rounded())
            guard addReading + addListening > 0 else { continue }
            var row = byDay[day] ?? Shishoapp.StatsDay(day: day)
            row.readingSeconds += addReading
            row.listeningSeconds += addListening
            byDay[day] = row
        }
        shown.days = byDay.values.sorted { $0.day < $1.day }
        return shown
    }

    // MARK: - The month

    /// The month's calendar with sessions laid over copy: each Book's time
    /// on each day it fell on, and finishes on their days. Without a copy,
    /// the month holds them alone, or is nil when none falls in it.
    static func month(_ copy: Shishoapp.StatsCalendarResponse?, year: Int, month: Int,
                      sessions: [UnsyncedCalendarSession], finishes: [UnsyncedFinish],
                      zone: TimeZone) -> Shishoapp.StatsCalendarResponse? {
        let prefix = String(format: "%04d-%02d-", year, month)
        var added: [String: [String: Double]] = [:]
        var refs: [String: Shishoapp.StatsBookRef] = [:]
        for session in sessions {
            for part in days(session, zone: zone) where part.day.hasPrefix(prefix) {
                added[part.day, default: [:]][session.book.key, default: 0] += part.seconds
                refs[session.book.key] = refs[session.book.key] ?? session.book
            }
        }
        let finished = finishes.filter { $0.day.hasPrefix(prefix) }
        guard copy != nil || !added.isEmpty || !finished.isEmpty else { return nil }

        var shown = copy ?? Shishoapp.StatsCalendarResponse(year: year, month: month, books: [], days: [])
        for ref in shown.books { refs[ref.key] = ref }
        var byDay = Dictionary(shown.days.map { ($0.day, $0) }, uniquingKeysWith: { first, _ in first })
        var shownKeys = Set(shown.books.map(\.key))
        func change(_ day: String, _ key: String, _ update: (inout Shishoapp.StatsCalendarDayBook) -> Void) {
            var row = byDay[day] ?? Shishoapp.StatsCalendarDay(day: day)
            if let index = row.books.firstIndex(where: { $0.key == key }) {
                update(&row.books[index])
            } else {
                var book = Shishoapp.StatsCalendarDayBook(key: key)
                update(&book)
                row.books.append(book)
            }
            byDay[day] = row
            if !shownKeys.contains(key), let ref = refs[key] {
                shown.books.append(ref)
                shownKeys.insert(key)
            }
        }
        for (day, books) in added {
            for (key, seconds) in books {
                let rounded = Int(seconds.rounded())
                guard rounded > 0 else { continue }
                change(day, key) { $0.seconds += rounded }
            }
        }
        for finish in finished {
            refs[finish.book.key] = refs[finish.book.key] ?? finish.book
            change(finish.day, finish.book.key) { $0.finished = true }
        }

        func ordered(_ a: String, _ b: String) -> Bool {
            let at = (refs[a]?.title ?? "").lowercased(), bt = (refs[b]?.title ?? "").lowercased()
            return at != bt ? at < bt : a < b
        }
        shown.days = byDay.values.map { day in
            var day = day
            day.books.sort { ordered($0.key, $1.key) }
            return day
        }.sorted { $0.day < $1.day }
        shown.books.sort { ordered($0.key, $1.key) }
        return shown
    }

    // MARK: - The day

    /// The day's detail with sessions laid over copy, each listed with its
    /// time on the day as this device's (deviceID, deviceName), and
    /// finishes the copy doesn't list. A copy's own time zone places them;
    /// zone does without one. Nil without a copy when nothing unsynced
    /// falls on the day.
    static func day(_ copy: Shishoapp.StatsDayResponse?, day: String, sessions: [UnsyncedCalendarSession],
                    finishes: [UnsyncedFinish], zone: TimeZone, deviceID: String, deviceName: String?) -> CalendarDayDetail? {
        let zone = copy.flatMap { TimeZone(identifier: $0.timeZone) } ?? zone
        var detail = CalendarDayDetail(response: copy ?? Shishoapp.StatsDayResponse(day: day, timeZone: zone.identifier))
        for session in sessions {
            guard let part = days(session, zone: zone).first(where: { $0.day == day }) else { continue }
            let id = -(detail.pendingSessions.count + 1)
            detail.response.sessions.append(Shishoapp.StatsDaySession(
                id: id, book: session.book,
                startedAt: session.uncountedStart.formatted(.iso8601), endedAt: session.endedAt.formatted(.iso8601),
                seconds: Int(part.seconds.rounded()),
                startPercentage: session.uncountedStartPercentage, endPercentage: session.endPercentage,
                kind: session.listening ? .listening : .reading, source: .shishoApp, measured: true,
                deviceId: deviceID.isEmpty ? nil : deviceID, deviceName: deviceName
            ))
            detail.pendingSessions.insert(id)
        }
        for finish in finishes where finish.day == day && !detail.response.finished.contains(where: { $0.key == finish.book.key }) {
            detail.response.finished.append(finish.book)
            detail.pendingFinished.insert(finish.book.key)
        }
        guard copy != nil || !detail.pendingSessions.isEmpty || !detail.pendingFinished.isEmpty else { return nil }
        if !detail.pendingSessions.isEmpty {
            detail.response.sessions = detail.response.sessions.enumerated().sorted { a, b in
                let at = CalendarLayout.instant(a.element.startedAt) ?? .distantPast
                let bt = CalendarLayout.instant(b.element.startedAt) ?? .distantPast
                return at != bt ? at < bt : a.offset < b.offset
            }.map(\.element)
        }
        return detail
    }
}
