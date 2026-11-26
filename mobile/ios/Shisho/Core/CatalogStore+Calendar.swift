import Foundation
import GRDB
import ShishoAPI

/// What of Reading Stats' Calendar the device keeps a copy of, so the tab
/// shows the last one offline.
enum CalendarCacheKey: Hashable, Sendable {
    /// The heatmap for a year, or the last 12 months when nil.
    case heatmap(year: Int?)
    /// A month, 1 to 12, of the calendar.
    case month(year: Int, month: Int)
    /// A day's detail, as YYYY-MM-DD.
    case day(String)

    var raw: String {
        switch self {
        case .heatmap(let year): "heatmap:" + (year.map(String.init) ?? "last")
        case .month(let year, let month): "calendar:" + String(format: "%04d-%02d", year, month)
        case .day(let day): "day:" + day
        }
    }
}

/// A response of the calendar's as last fetched.
struct CalendarCopy<Value: Sendable & Equatable>: Sendable, Equatable {
    let value: Value
    let fetchedAt: Date
}

extension CatalogStore {
    /// Reading Stats' Calendar as the server last sent each part of it, by
    /// CalendarCacheKey.
    static func migrateReadingCalendar(_ db: Database) throws {
        try db.create(table: "calendar_cache") { t in
            t.primaryKey("cache_key", .text)
            t.column("body", .blob).notNull()
            t.column("fetched_at", .double).notNull()
        }
    }

    /// Keeps a response of the calendar's, fetched at fetchedAt.
    func saveCalendar(_ value: some Encodable & Sendable, key: CalendarCacheKey, fetchedAt: Date) throws {
        let body = try JSONEncoder().encode(value)
        try pool.write { db in
            try db.execute(sql: "INSERT OR REPLACE INTO calendar_cache (cache_key, body, fetched_at) VALUES (?, ?, ?)",
                           arguments: [key.raw, body, fetchedAt.timeIntervalSince1970])
        }
    }

    /// The copy kept for key, or nil before it was fetched.
    static func calendarCopy<Value: Decodable & Sendable & Equatable>(
        _ db: Database, key: CalendarCacheKey, as _: Value.Type
    ) throws -> CalendarCopy<Value>? {
        guard let row = try Row.fetchOne(db, sql: "SELECT body, fetched_at FROM calendar_cache WHERE cache_key = ?",
                                         arguments: [key.raw]),
              let value = try? makeShishoDecoder().decode(Value.self, from: row["body"] as Data)
        else { return nil }
        return CalendarCopy(value: value, fetchedAt: Date(timeIntervalSince1970: row["fetched_at"]))
    }

    /// The Books in the catalog among ids, by id, for the calendar's links.
    static func calendarBooks(_ db: Database, ids: [Int]) throws -> [Int: BookSummary] {
        guard !ids.isEmpty else { return [:] }
        let list = Set(ids).map(String.init).joined(separator: ",")
        let books = try summaries(db, rows: Row.fetchAll(db, sql: summarySQL + " WHERE b.id IN (\(list))"))
        return Dictionary(books.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
    }

    // MARK: - This device's reading not synced yet

    /// How long a calendar copy holds back folding the sessions it doesn't
    /// count into counted_sessions, past the oldest copy of Reading Stats
    /// (statsCaughtUp).
    static let calendarHold: Double = 30 * 86400

    /// The time zone the server groups days in, from Reading Stats' copy,
    /// as the Overview places this device's reading; the device's own
    /// before any copy.
    static func calendarZone(_ db: Database) throws -> TimeZone {
        guard let body = try Data.fetchOne(db, sql: "SELECT body FROM stats_cache ORDER BY range_key LIMIT 1"),
              let stats = try? makeShishoDecoder().decode(Shishoapp.StatsResponse.self, from: body),
              let zone = TimeZone(identifier: stats.timeZone)
        else { return .current }
        return zone
    }

    /// This device's name as the server last said it, in Reading Stats'
    /// places, or nil before it has.
    static func ownDeviceName(_ db: Database, deviceID: String) throws -> String? {
        guard !deviceID.isEmpty else { return nil }
        let decoder = makeShishoDecoder()
        for body in try Data.fetchAll(db, sql: "SELECT body FROM stats_cache ORDER BY range_key") {
            guard let stats = try? decoder.decode(Shishoapp.StatsResponse.self, from: body) else { continue }
            if let name = stats.places.first(where: { $0.deviceId == deviceID })?.deviceName, !name.isEmpty { return name }
        }
        return nil
    }

    /// The sittings of this device's reading a calendar copy fetched at
    /// fetchedAt doesn't count all of, by the Overview's rule
    /// (UnsyncedReading.uncounted), each with its Book named from the
    /// catalog. Without a copy (nil), none of it is counted but what every
    /// copy of Reading Stats counts.
    static func unsyncedCalendarSessions(_ db: Database, fetchedAt: Date?) throws -> [UnsyncedCalendarSession] {
        let uncounted = UnsyncedReading.uncounted(try sessionSnapshots(db), counted: try countedSessions(db),
                                                  fetchedAt: fetchedAt?.timeIntervalSince1970 ?? 0)
            .filter { $0.seconds - $0.counted > 0 }
        guard !uncounted.isEmpty else { return [] }
        let ids = Set(uncounted.map(\.payload.fileId))
        var files: [Int: Row] = [:]
        for row in try Row.fetchAll(db, sql: """
            SELECT f.id, f.file_type, f.book_id, b.title FROM book_files f JOIN books b ON b.id = f.book_id
            WHERE f.id IN (\(ids.map(String.init).joined(separator: ",")))
            """) {
            files[row["id"]] = row
        }
        return uncounted.compactMap { sitting in
            let session = sitting.payload
            guard let started = timestamp(session.startedAt), let ended = timestamp(session.endedAt) else { return nil }
            let file = files[session.fileId]
            // A File no longer in the catalog still counts, under a Book
            // the calendar can't name.
            let book = file.map { Shishoapp.StatsBookRef(key: "book-\($0["book_id"] as Int)", title: $0["title"], bookId: $0["book_id"]) }
                ?? Shishoapp.StatsBookRef(key: "file-\(session.fileId)", removed: true)
            return UnsyncedCalendarSession(book: book, listening: file.map { ($0["file_type"] as String) == "m4b" } ?? false,
                                           startedAt: Date(timeIntervalSince1970: started), endedAt: Date(timeIntervalSince1970: ended),
                                           seconds: sitting.seconds, counted: sitting.counted,
                                           startPercentage: session.startPercentage, endPercentage: session.endPercentage)
        }
    }

    /// The Read-throughs finished on this device that the server hasn't
    /// confirmed, as the Overview counts them, each on its end day.
    static func unsyncedFinishes(_ db: Database) throws -> [UnsyncedFinish] {
        try Row.fetchAll(db, sql: """
            SELECT rt.id, rt.book_id, rt.end_day, rt.book_title, b.id AS present, b.title FROM read_throughs rt
            LEFT JOIN books b ON b.id = rt.book_id
            WHERE rt.ended_here AND rt.outcome = 'finished' AND rt.end_day IS NOT NULL
            ORDER BY rt.end_day, rt.id
            """).map { row in
            let book: Shishoapp.StatsBookRef
            if let bookID = row["book_id"] as Int? {
                let present = row["present"] as Int?
                book = Shishoapp.StatsBookRef(key: "book-\(bookID)", title: row["title"] ?? row["book_title"] ?? "",
                                              removed: present == nil, bookId: present)
            } else {
                book = Shishoapp.StatsBookRef(key: "read-through-\(row["id"] as Int)", title: row["book_title"] ?? "", removed: true)
            }
            return UnsyncedFinish(book: book, day: row["end_day"])
        }
    }

    /// The heatmap for year, or the last 12 months, as Reading Stats'
    /// Calendar shows it: the copy with this device's reading not synced
    /// yet laid over it, or that reading alone before any copy.
    static func calendarHeatmap(_ db: Database, year: Int?, now: Date = Date()) throws -> CalendarShown<Shishoapp.StatsHeatmapResponse>? {
        let copy = try calendarCopy(db, key: .heatmap(year: year), as: Shishoapp.StatsHeatmapResponse.self)
        let zone = try calendarZone(db)
        guard let shown = CalendarLayering.heatmap(copy?.value, year: year, today: UnsyncedReading.day(now, zone: zone),
                                                   sessions: try unsyncedCalendarSessions(db, fetchedAt: copy?.fetchedAt),
                                                   finishes: try unsyncedFinishes(db), zone: zone)
        else { return nil }
        return CalendarShown(value: shown, deviceOnly: copy == nil)
    }

    /// A month of the calendar as shown, as calendarHeatmap.
    static func calendarMonth(_ db: Database, year: Int, month: Int) throws -> CalendarShown<Shishoapp.StatsCalendarResponse>? {
        let copy = try calendarCopy(db, key: .month(year: year, month: month), as: Shishoapp.StatsCalendarResponse.self)
        guard let shown = CalendarLayering.month(copy?.value, year: year, month: month,
                                                 sessions: try unsyncedCalendarSessions(db, fetchedAt: copy?.fetchedAt),
                                                 finishes: try unsyncedFinishes(db), zone: try calendarZone(db))
        else { return nil }
        return CalendarShown(value: shown, deviceOnly: copy == nil)
    }

    /// A day's detail as shown, as calendarHeatmap, its Sessions not
    /// synced yet named for this device: by the server's name for it, or
    /// deviceName before the server has said.
    static func calendarDay(_ db: Database, day: String, ownDeviceID: String,
                            deviceName: String?) throws -> CalendarShown<CalendarDayDetail>? {
        let copy = try calendarCopy(db, key: .day(day), as: Shishoapp.StatsDayResponse.self)
        guard let shown = CalendarLayering.day(copy?.value, day: day,
                                               sessions: try unsyncedCalendarSessions(db, fetchedAt: copy?.fetchedAt),
                                               finishes: try unsyncedFinishes(db), zone: try calendarZone(db),
                                               deviceID: ownDeviceID,
                                               deviceName: try ownDeviceName(db, deviceID: ownDeviceID) ?? deviceName)
        else { return nil }
        return CalendarShown(value: shown, deviceOnly: copy == nil)
    }
}
