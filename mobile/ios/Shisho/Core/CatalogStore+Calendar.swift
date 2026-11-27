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

    /// The key whose raw is raw, or nil for one it can't be.
    init?(raw: String) {
        let parts = raw.split(separator: ":", maxSplits: 1).map(String.init)
        guard parts.count == 2 else { return nil }
        switch parts[0] {
        case "heatmap":
            if parts[1] == "last" {
                self = .heatmap(year: nil)
            } else if let year = Int(parts[1]) {
                self = .heatmap(year: year)
            } else {
                return nil
            }
        case "calendar":
            let numbers = parts[1].split(separator: "-").compactMap { Int($0) }
            guard numbers.count == 2 else { return nil }
            self = .month(year: numbers[0], month: numbers[1])
        case "day":
            self = .day(parts[1])
        default:
            return nil
        }
    }

    /// The kind of copy this is, as its raw starts.
    var kind: String { String(raw.prefix(while: { $0 != ":" })) }
}

/// A response of the calendar's as last fetched, and the time zone its days
/// are in, which the device lays its own reading over it in.
struct CalendarCopy<Value: Sendable & Equatable>: Sendable, Equatable {
    let value: Value
    let fetchedAt: Date
    let zone: TimeZone?
}

extension CatalogStore {
    /// Reading Stats' Calendar as the server last sent each part of it, by
    /// CalendarCacheKey, with the time zone its days are in and when the
    /// Calendar last showed it. And when this device sent changes the
    /// copies may not count yet (markCalendarStale).
    static func migrateReadingCalendar(_ db: Database) throws {
        try db.create(table: "calendar_cache") { t in
            t.primaryKey("cache_key", .text)
            t.column("body", .blob).notNull()
            t.column("fetched_at", .double).notNull()
            t.column("time_zone", .text).notNull()
            t.column("viewed_at", .double).notNull()
        }
        try db.create(table: "calendar_stale") { t in
            t.autoIncrementedPrimaryKey("id")
            t.column("marked_at", .double).notNull()
        }
    }

    /// How many copies of each kind the device keeps, the ones the Calendar
    /// showed last, since a sync fetches every copy again (SyncEngine
    /// refreshCalendar).
    static let calendarKept: [String: Int] = ["heatmap": 3, "calendar": 6, "day": 10]

    /// Keeps a response of the calendar's, fetched at fetchedAt, in zone, or
    /// in the time zone Reading Stats last said without one. viewed is false
    /// for a copy a sync fetched again, which leaves when the Calendar last
    /// showed it alone, and which a copy the Calendar fetched since, newer,
    /// isn't replaced by.
    func saveCalendar(_ value: some Encodable & Sendable, key: CalendarCacheKey, fetchedAt: Date,
                      zone: String? = nil, viewed: Bool = true) throws {
        let body = try JSONEncoder().encode(value)
        try pool.write { db in
            let zone = try zone ?? CatalogStore.statsZone(db) ?? ""
            let at = fetchedAt.timeIntervalSince1970
            if viewed {
                try db.execute(sql: """
                    INSERT OR REPLACE INTO calendar_cache (cache_key, body, fetched_at, time_zone, viewed_at) VALUES (?, ?, ?, ?, ?)
                    """, arguments: [key.raw, body, at, zone, Date().timeIntervalSince1970])
                // The kind's oldest-shown copies past what's kept go.
                try db.execute(sql: """
                    DELETE FROM calendar_cache WHERE cache_key LIKE ? AND cache_key NOT IN (
                      SELECT cache_key FROM calendar_cache WHERE cache_key LIKE ? ORDER BY viewed_at DESC LIMIT ?)
                    """, arguments: [key.kind + ":%", key.kind + ":%", CatalogStore.calendarKept[key.kind] ?? 6])
            } else {
                try db.execute(sql: """
                    UPDATE calendar_cache SET body = ?, fetched_at = ?, time_zone = ? WHERE cache_key = ? AND fetched_at < ?
                    """, arguments: [body, at, zone, key.raw, at])
            }
        }
    }

    func deleteCalendar(_ key: CalendarCacheKey) throws {
        try pool.write { db in
            try db.execute(sql: "DELETE FROM calendar_cache WHERE cache_key = ?", arguments: [key.raw])
        }
    }

    // MARK: - Keeping copies current

    /// Whether row can change what the calendar shows. A position near the
    /// end can finish a Read-through, or take back one it finished.
    static func changesCalendar(_ row: JournalRow) -> Bool {
        switch Shishoapp.JournalEventType(rawValue: row.type) {
        case .readingSession, .readThroughStart, .readThroughFinish, .readThroughAbandon, .readThroughPause,
             .readThroughResume, .readThroughDelete, .referenceMark, .referenceRemove:
            return true
        case .readingPosition:
            guard let position = try? makeShishoDecoder().decode(Shishoapp.JournalPositionPayload.self, from: row.payload) else { return true }
            return position.atEnd || position.percentage >= 0.9
        default:
            return false
        }
    }

    /// Notes that rows, about to be sent, can change what the calendar
    /// shows, so the next sync that reaches the Calendar's routes fetches
    /// every copy again (calendarCopiesToRefresh). The server decides which
    /// days they land on, by its clock and rules, so the device doesn't
    /// guess. It's noted before sending, whatever the server lists, so a
    /// sync that stops once the server took them in, or a server that
    /// stops offering the Calendar for a while, still leaves it for later.
    func markCalendarStale(_ rows: [JournalRow], at date: Date = Date()) throws {
        guard rows.contains(where: CatalogStore.changesCalendar) else { return }
        try pool.write { db in
            try db.execute(sql: "INSERT INTO calendar_stale (marked_at) VALUES (?)", arguments: [date.timeIntervalSince1970])
        }
    }

    /// The calendar copies a sync fetches again: every one, once changes
    /// were noted by markCalendarStale, and none otherwise. First, when
    /// Reading Stats now say another time zone than a copy was kept in, the
    /// copy goes, since its days are another zone's.
    func calendarCopiesToRefresh() throws -> [CalendarCacheKey] {
        try pool.write { db in
            if let zone = try CatalogStore.statsZone(db) {
                try db.execute(sql: "DELETE FROM calendar_cache WHERE time_zone != ?", arguments: [zone])
            }
            guard try Bool.fetchOne(db, sql: "SELECT EXISTS (SELECT 1 FROM calendar_stale)") == true else { return [] }
            return try String.fetchAll(db, sql: "SELECT cache_key FROM calendar_cache ORDER BY cache_key")
                .compactMap(CalendarCacheKey.init(raw:))
        }
    }

    /// Forgets the changes noted before date, once a sync has fetched every
    /// copy again or dropped it.
    func calendarRefreshed(before date: Date) throws {
        try pool.write { db in
            try db.execute(sql: "DELETE FROM calendar_stale WHERE marked_at < ?", arguments: [date.timeIntervalSince1970])
        }
    }

    /// The copy kept for key, or nil before it was fetched.
    static func calendarCopy<Value: Decodable & Sendable & Equatable>(
        _ db: Database, key: CalendarCacheKey, as _: Value.Type
    ) throws -> CalendarCopy<Value>? {
        guard let row = try Row.fetchOne(db, sql: "SELECT body, fetched_at, time_zone FROM calendar_cache WHERE cache_key = ?",
                                         arguments: [key.raw]),
              let value = try? makeShishoDecoder().decode(Value.self, from: row["body"] as Data)
        else { return nil }
        return CalendarCopy(value: value, fetchedAt: Date(timeIntervalSince1970: row["fetched_at"]),
                            zone: TimeZone(identifier: row["time_zone"] as String))
    }

    /// The Books in the catalog among ids, by id, for the calendar's links.
    static func calendarBooks(_ db: Database, ids: [Int]) throws -> [Int: BookSummary] {
        guard !ids.isEmpty else { return [:] }
        let list = Set(ids).map(String.init).joined(separator: ",")
        let books = try summaries(db, rows: Row.fetchAll(db, sql: summarySQL + " WHERE b.id IN (\(list))"))
        return Dictionary(books.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
    }

    // MARK: - This device's reading not synced yet

    /// The time zone the server groups days in, from Reading Stats' copy,
    /// as the Overview places this device's reading; the device's own
    /// before any copy.
    static func calendarZone(_ db: Database) throws -> TimeZone {
        try statsZone(db).flatMap(TimeZone.init(identifier:)) ?? .current
    }

    /// The time zone Reading Stats' copy names, as the server wrote it, or
    /// nil before any copy.
    static func statsZone(_ db: Database) throws -> String? {
        guard let body = try Data.fetchOne(db, sql: "SELECT body FROM stats_cache ORDER BY range_key LIMIT 1"),
              let stats = try? makeShishoDecoder().decode(Shishoapp.StatsResponse.self, from: body),
              !stats.timeZone.isEmpty
        else { return nil }
        return stats.timeZone
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
        let zone = try copy?.zone ?? calendarZone(db)
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
                                                 finishes: try unsyncedFinishes(db), zone: try copy?.zone ?? calendarZone(db))
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
                                               finishes: try unsyncedFinishes(db), zone: try copy?.zone ?? calendarZone(db),
                                               deviceID: ownDeviceID,
                                               deviceName: try ownDeviceName(db, deviceID: ownDeviceID) ?? deviceName)
        else { return nil }
        return CalendarShown(value: shown, deviceOnly: copy == nil)
    }
}
