import Foundation
import GRDB
import ShishoAPI

/// One Read-through on Reading History.
struct HistoryEntry: Identifiable, Hashable, Sendable {
    let id: Int
    /// The Book when the device has it; nil once it's deleted or out of
    /// reach, when the entry keeps its title.
    let book: BookSummary?
    let title: String
    let authors: String
    let outcome: String
    let paused: Bool
    let beginDay: String?
    let endDay: String?
    /// Holds a change or reading the server hasn't confirmed.
    let pending: Bool
}

/// A List as the Add to List sheet offers it for one Book.
struct ListChoice: Identifiable, Hashable, Sendable {
    let id: Int
    let name: String
    let contains: Bool
}

/// Reading Stats as the server last sent them for one range, with this
/// device's reading not yet counted laid over them.
struct ReadingStats: Equatable, Sendable {
    var stats: Shishoapp.StatsResponse
    /// When the copy was fetched.
    let fetchedAt: Date
    /// Reading and listening time laid over the copy, in seconds.
    var unsyncedSeconds = 0
}

extension CatalogStore {
    // MARK: - Currently Reading and To Read

    /// The Books being read, the one read most recently first, as the web's
    /// Currently Reading page lists them: a paused one waits until it's
    /// resumed, and a Reference Book never has a Read-through underway.
    static func currentlyReading(_ db: Database) throws -> [BookSummary] {
        try summaries(db, rows: Row.fetchAll(db, sql: summarySQL + """
             JOIN read_throughs rt ON rt.book_id = b.id AND rt.outcome = 'underway' AND NOT rt.paused
            ORDER BY MAX(COALESCE(rt.last_read_at, 0),
                COALESCE((SELECT MAX(lp.updated_at) FROM local_positions lp JOIN book_files f ON f.id = lp.file_id WHERE f.book_id = b.id), 0)) DESC,
                rt.begin_day DESC, b.id
            """))
    }

    /// The Books on To Read, in order.
    static func toReadBooks(_ db: Database) throws -> [BookSummary] {
        try summaries(db, rows: Row.fetchAll(db, sql: summarySQL + """
             JOIN user_books ub ON ub.book_id = b.id AND ub.to_read_position IS NOT NULL
            ORDER BY ub.to_read_position, b.id
            """))
    }

    /// Whether the Book has a Read-through underway that this device made
    /// and the server hasn't confirmed.
    static func underwayReadThrough(_ db: Database, bookID: Int) throws -> (id: Int, pending: Bool)? {
        try Row.fetchOne(db, sql: "SELECT id, pending FROM read_throughs WHERE book_id = ? AND outcome = 'underway'", arguments: [bookID])
            .map { ($0["id"], $0["pending"]) }
    }

    /// Whether the Book was read through before: a Read-through that ended.
    static func readBefore(_ db: Database, bookID: Int) throws -> Bool {
        try Bool.fetchOne(db, sql: "SELECT EXISTS (SELECT 1 FROM read_throughs WHERE book_id = ? AND outcome != 'underway')",
                          arguments: [bookID]) ?? false
    }

    /// The Book's place on To Read, from 1, or nil when it isn't on it.
    static func toReadPlace(_ db: Database, bookID: Int) throws -> Int? {
        guard try Bool.fetchOne(db, sql: "SELECT to_read_position IS NOT NULL FROM user_books WHERE book_id = ?",
                                arguments: [bookID]) == true else { return nil }
        return try Int.fetchOne(db, sql: """
            SELECT COUNT(*) + 1 FROM user_books o JOIN books b ON b.id = o.book_id, user_books t
            WHERE t.book_id = ? AND o.to_read_position IS NOT NULL
                AND (o.to_read_position < t.to_read_position OR (o.to_read_position = t.to_read_position AND o.book_id < t.book_id))
            """, arguments: [bookID])
    }

    // MARK: - Home

    /// A Book Home opens straight into the player or a reader, with the File
    /// it goes on in: the one read or listened to last, on any device, and
    /// how far through that File the user is.
    struct ContinueItem: Identifiable, Hashable, Sendable {
        let book: BookSummary
        let fileID: Int
        let listens: Bool
        var progress: Double?
        /// The File's type and cover version, so a card shows the cover of
        /// the File it opens, square for an audiobook.
        var fileType = ""
        var coverVersion = ""
        var id: Int { book.id }
    }

    static func continueItems(_ db: Database) throws -> [ContinueItem] {
        try resumeItems(db, books: currentlyReading(db))
    }

    /// The Recent Reference Books, each with the File it opens in.
    static func recentReferenceItems(_ db: Database, limit: Int? = nil) throws -> [ContinueItem] {
        try resumeItems(db, books: recentReference(db, limit: limit))
    }

    /// books with the File each goes on in, leaving out a Book with none the
    /// app can open.
    static func resumeItems(_ db: Database, books: [BookSummary]) throws -> [ContinueItem] {
        let types = (readableTypes.union(["m4b"])).map { "'\($0)'" }.joined(separator: ",")
        return try books.compactMap { book in
            guard let row = try Row.fetchOne(db, sql: """
                SELECT f.id, f.file_type, f.cover_version, COALESCE(lp.percentage, p.percentage) AS percentage FROM book_files f
                LEFT JOIN local_positions l ON l.file_id = f.id
                LEFT JOIN local_progress lp ON lp.file_id = f.id
                LEFT JOIN reading_positions p ON p.file_id = f.id
                WHERE f.book_id = ? AND f.file_role = 'main' AND f.file_type IN (\(types))
                ORDER BY MAX(COALESCE(l.updated_at, 0), COALESCE(p.occurred_at, 0)) DESC, f.position
                LIMIT 1
                """, arguments: [book.id]) else { return nil }
            return ContinueItem(book: book, fileID: row["id"], listens: (row["file_type"] as String) == "m4b",
                                progress: row["percentage"], fileType: row["file_type"], coverVersion: row["cover_version"] ?? "")
        }
    }

    /// Notes that a File's Book was opened now, for Recent Reference.
    func noteOpened(fileID: Int, at date: Date = Date()) throws {
        try pool.write { db in
            try db.execute(sql: """
                INSERT INTO recent_opens (book_id, opened_at) SELECT book_id, ? FROM book_files WHERE id = ?
                ON CONFLICT (book_id) DO UPDATE SET opened_at = excluded.opened_at
                """, arguments: [date.timeIntervalSince1970, fileID])
        }
    }

    /// The Reference Books opened on this device, the latest first: there's
    /// no Read-through to say one is underway, so this keeps them handy.
    static func recentReference(_ db: Database, limit: Int? = nil) throws -> [BookSummary] {
        var sql = summarySQL + """
             JOIN recent_opens ro ON ro.book_id = b.id
            WHERE EXISTS (SELECT 1 FROM user_books ub WHERE ub.book_id = b.id AND ub.reference)
            ORDER BY ro.opened_at DESC, b.id
            """
        if let limit { sql += " LIMIT \(limit)" }
        return try summaries(db, rows: Row.fetchAll(db, sql: sql))
    }

    /// Takes Books off Recent Reference, or every one with nil.
    func clearRecent(bookIDs: [Int]?) throws {
        try pool.write { db in
            if let bookIDs {
                try db.execute(sql: "DELETE FROM recent_opens WHERE book_id IN (\(bookIDs.map(String.init).joined(separator: ",")))")
            } else {
                try db.execute(sql: "DELETE FROM recent_opens WHERE book_id IN (SELECT book_id FROM user_books WHERE reference)")
            }
        }
    }

    // MARK: - Lists

    /// The Lists the user can put Books in, with whether each holds the Book.
    static func listChoices(_ db: Database, bookID: Int) throws -> [ListChoice] {
        try Row.fetchAll(db, sql: """
            SELECT l.id, l.name, EXISTS (SELECT 1 FROM list_books lb WHERE lb.list_id = l.id AND lb.book_id = ?) AS contains
            FROM lists l WHERE l.permission IN ('owner', 'editor', 'manager')
            ORDER BY l.name COLLATE NOCASE, l.id
            """, arguments: [bookID]).map { ListChoice(id: $0["id"], name: $0["name"], contains: $0["contains"]) }
    }

    /// The ids of a List's Books in its manual order, for reordering.
    static func listOrder(_ db: Database, listID: Int) throws -> [Int] {
        try Int.fetchAll(db, sql: """
            SELECT lb.book_id FROM list_books lb JOIN books b ON b.id = lb.book_id WHERE lb.list_id = ?
            ORDER BY lb.sort_order IS NULL, lb.sort_order, lb.added_at DESC, lb.position
            """, arguments: [listID])
    }

    /// Whether the List is ordered, and what the user may do with it.
    static func listAbilities(_ db: Database, listID: Int) throws -> (ordered: Bool, permission: String)? {
        try Row.fetchOne(db, sql: "SELECT is_ordered, permission FROM lists WHERE id = ?", arguments: [listID])
            .map { ($0["is_ordered"], $0["permission"]) }
    }

    // MARK: - Reading History

    /// Every Read-through, the ones underway first, then by the day each
    /// ended, as the web's Reading History lists them.
    static func history(_ db: Database) throws -> [HistoryEntry] {
        let rows = try Row.fetchAll(db, sql: """
            SELECT rt.id, rt.book_id, rt.outcome, rt.paused, rt.begin_day, rt.end_day, rt.book_title, rt.book_authors,
                rt.pending OR (rt.outcome = 'underway' AND EXISTS (
                    SELECT 1 FROM journal j JOIN book_files f ON f.id = j.file_id
                    WHERE j.type = 'reading_session' AND f.book_id = rt.book_id)) AS pending,
                b.id AS present
            FROM read_throughs rt LEFT JOIN books b ON b.id = rt.book_id
            ORDER BY rt.outcome = 'underway' DESC, rt.end_day IS NULL, rt.end_day DESC, rt.begin_day DESC, rt.id DESC
            """)
        return try historyEntries(db, rows: rows)
    }

    /// Read-through rows as Reading History lists them, each with its Book
    /// when the device has it, or the title and authors it kept. rows have
    /// the Read-through's columns and the Book's id as present, or null.
    static func historyEntries(_ db: Database, rows: [Row]) throws -> [HistoryEntry] {
        let ids = rows.compactMap { $0["present"] as Int? }
        var books: [Int: BookSummary] = [:]
        if !ids.isEmpty {
            for book in try summaries(db, rows: Row.fetchAll(db, sql: summarySQL + " WHERE b.id IN (\(ids.map(String.init).joined(separator: ",")))")) {
                books[book.id] = book
            }
        }
        return rows.map { row in
            let book = (row["present"] as Int?).flatMap { books[$0] }
            let authors = (row["book_authors"] as String?).map { names($0).joined(separator: ", ") }
            return HistoryEntry(id: row["id"], book: book, title: book?.title ?? row["book_title"] ?? String(localized: "Unknown Book"),
                                authors: book?.authors ?? authors ?? "", outcome: row["outcome"], paused: row["paused"],
                                beginDay: row["begin_day"], endDay: row["end_day"], pending: row["pending"])
        }
    }

    // MARK: - Reading Stats

    /// Keeps the server's Reading Stats for a range, fetched at fetchedAt.
    /// Sessions the server took in before the fetch began are counted in it
    /// now, so they stop being laid over it.
    func saveStats(_ stats: Shishoapp.StatsResponse, year: Int?, fetchedAt: Date) throws {
        let body = try JSONEncoder().encode(stats)
        try pool.write { db in
            try db.execute(sql: "INSERT OR REPLACE INTO stats_cache (range_key, body, fetched_at) VALUES (?, ?, ?)",
                           arguments: [year ?? 0, body, fetchedAt.timeIntervalSince1970])
        }
    }

    /// Folds the sessions every copy counts into counted_sessions: those the
    /// server took in before the oldest copy of Reading Stats, and, while
    /// the Calendar shows (calendarHolds), of the Calendar, was fetched. The
    /// sync fetches every Calendar copy again after sending what can change
    /// it (SyncEngine.refreshCalendar), and the device keeps few, so a copy
    /// holds sessions back only until a sync reaches it.
    func statsCaughtUp(calendarHolds: Bool = true) throws {
        try pool.write { db in
            let decoder = makeShishoDecoder()
            guard let statsCutoff = try Double.fetchOne(db, sql: "SELECT MIN(fetched_at) FROM stats_cache") else { return }
            let calendarCutoff = calendarHolds ? try Double.fetchOne(db, sql: "SELECT MIN(fetched_at) FROM calendar_cache") : nil
            let cutoff = min(statsCutoff, calendarCutoff ?? statsCutoff)
            for row in try Row.fetchAll(db, sql: "SELECT payload FROM unstated_sessions WHERE accepted_at < ?", arguments: [cutoff]) {
                guard let session = try? decoder.decode(Shishoapp.JournalSessionPayload.self, from: row["payload"] as Data),
                      !session.sessionId.isEmpty else { continue }
                try db.execute(sql: """
                    INSERT INTO counted_sessions (session_id, seconds, counted_at) VALUES (?, ?, ?)
                    ON CONFLICT (session_id) DO UPDATE SET seconds = MAX(seconds, excluded.seconds), counted_at = excluded.counted_at
                    """, arguments: [session.sessionId, CatalogStore.seconds(session), cutoff])
            }
            try db.execute(sql: "DELETE FROM unstated_sessions WHERE accepted_at < ?", arguments: [cutoff])
            // A sitting lasts at most a day, so an older one never sends
            // another snapshot.
            try db.execute(sql: "DELETE FROM counted_sessions WHERE counted_at < ?", arguments: [cutoff - 3 * 86400])
        }
    }

    /// The ranges with a copy, for fetching each again after a sync.
    func statsRanges() throws -> [Int?] {
        try pool.read { db in
            try Int.fetchAll(db, sql: "SELECT range_key FROM stats_cache ORDER BY range_key").map { $0 == 0 ? nil : $0 }
        }
    }

    static func seconds(_ session: Shishoapp.JournalSessionPayload) -> Double {
        if let duration = session.duration { return Double(duration) }
        guard let start = timestamp(session.startedAt), let end = timestamp(session.endedAt) else { return 0 }
        return max(0, end - start)
    }

    /// Reading Stats for year, or all time, as last fetched, with this
    /// device's reading and finishes the server hasn't counted laid over
    /// them: time, periods, places, and Finished, Did Not Finish, and
    /// reread counts. Breakdowns by Book, pace, words read, and the average
    /// Rating catch up at the next sync. Nil before the range was fetched.
    static func stats(_ db: Database, year: Int?, ownDeviceID: String, now: Date = Date()) throws -> ReadingStats? {
        guard let row = try Row.fetchOne(db, sql: "SELECT body, fetched_at FROM stats_cache WHERE range_key = ?", arguments: [year ?? 0]),
              let cached = try? makeShishoDecoder().decode(Shishoapp.StatsResponse.self, from: row["body"] as Data)
        else { return nil }
        var result = ReadingStats(stats: cached, fetchedAt: Date(timeIntervalSince1970: row["fetched_at"]))
        let zone = TimeZone(identifier: cached.timeZone) ?? .current

        // Sessions not yet counted: the latest snapshot of each sitting,
        // less what the copy already counts of it (UnsyncedReading).
        let uncounted = UnsyncedReading.uncounted(try sessionSnapshots(db), counted: try countedSessions(db),
                                                  fetchedAt: row["fetched_at"])
        let listening = try listenedFiles(db, ids: Set(uncounted.map(\.payload.fileId)))
        var stats = result.stats
        func add(_ seconds: Int, of session: Shishoapp.JournalSessionPayload, year sessionYear: Int, month: Int) {
            let listened = listening[session.fileId] ?? false
            result.unsyncedSeconds += seconds
            if listened { stats.totals.listeningSeconds += seconds } else { stats.totals.readingSeconds += seconds }
            addToPeriod(&stats, year: sessionYear, month: year == nil ? nil : month) { period in
                if listened { period.listeningSeconds += seconds } else { period.readingSeconds += seconds }
            }
            if let index = stats.places.firstIndex(where: { $0.deviceId == ownDeviceID && !ownDeviceID.isEmpty }) {
                if listened { stats.places[index].listeningSeconds += seconds } else { stats.places[index].readingSeconds += seconds }
            } else {
                stats.places.append(Shishoapp.StatsPlace(source: .shishoApp, deviceId: ownDeviceID, deviceName: nil,
                                                         readingSeconds: listened ? 0 : seconds, listeningSeconds: listened ? seconds : 0))
            }
            if !stats.years.contains(sessionYear) {
                stats.years.append(sessionYear)
                stats.years.sort(by: >)
            }
        }
        // Each sitting's time is split at midnight and every part counted on
        // its own day, as the server counts it, so one across New Year's
        // Eve doesn't move between years once it syncs.
        for sitting in uncounted {
            let session = sitting.payload
            let added = sitting.seconds - sitting.counted
            guard added > 0, let started = timestamp(session.startedAt) else { continue }
            let start = Date(timeIntervalSince1970: started)
            let end = timestamp(session.endedAt).map(Date.init(timeIntervalSince1970:)) ?? start
            let from = UnsyncedReading.uncountedStart(start: start, end: end, seconds: sitting.seconds, counted: sitting.counted)
            for part in UnsyncedReading.splitByDay(start: from, end: end, seconds: added, zone: zone) {
                let day = part.day.split(separator: "-").compactMap { Int($0) }
                let seconds = Int(part.seconds.rounded())
                guard day.count == 3, year == nil || year == day[0], seconds > 0 else { continue }
                add(seconds, of: session, year: day[0], month: day[1])
            }
        }
        stats.places.sort { ($0.readingSeconds + $0.listeningSeconds) > ($1.readingSeconds + $1.listeningSeconds) }

        // Read-throughs ended here and not yet confirmed.
        for row in try Row.fetchAll(db, sql: """
            SELECT rt.outcome, rt.end_day, EXISTS (
                SELECT 1 FROM synced_read_throughs o WHERE o.book_id = rt.book_id AND o.outcome = 'finished' AND o.id != rt.id
                    AND (o.end_day IS NULL OR rt.end_day IS NULL OR o.end_day <= rt.end_day)) AS reread
            FROM read_throughs rt WHERE rt.ended_here AND rt.end_day IS NOT NULL
            """) {
            let endDay: String = row["end_day"]
            let parts = endDay.split(separator: "-").compactMap { Int($0) }
            guard parts.count == 3, year == nil || year == parts[0] else { continue }
            if (row["outcome"] as String) == "finished" {
                stats.totals.finished += 1
                if row["reread"] as Bool { stats.totals.rereads += 1 }
                addToPeriod(&stats, year: parts[0], month: year == nil ? nil : parts[1]) { $0.finished += 1 }
            } else {
                stats.totals.didNotFinish += 1
            }
            if !stats.years.contains(parts[0]) {
                stats.years.append(parts[0])
                stats.years.sort(by: >)
            }
        }
        result.stats = stats
        return result
    }

    /// Adds to the period for year and month (nil for the whole year),
    /// making it when the copy has none.
    private static func addToPeriod(_ stats: inout Shishoapp.StatsResponse, year: Int, month: Int?,
                                    change: (inout Shishoapp.StatsPeriod) -> Void) {
        if let index = stats.periods.firstIndex(where: { $0.year == year && $0.month == month }) {
            change(&stats.periods[index])
            return
        }
        var period = Shishoapp.StatsPeriod(year: year, month: month)
        change(&period)
        stats.periods.append(period)
        stats.periods.sort { ($0.year, $0.month ?? 0) < ($1.year, $1.month ?? 0) }
    }
}
