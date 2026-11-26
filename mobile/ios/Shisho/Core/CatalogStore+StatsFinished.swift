import Foundation
import GRDB

/// A row of a Reading Stats breakdown, whose finished Books the Finished
/// page lists, as the web's Finished section filters them. Kind isn't one:
/// the server works a Read-through's kind out from its Sessions, which the
/// device doesn't keep.
enum StatsFinishedFilter: Hashable, Sendable {
    case genre(String)
    case tag(String)
    case author(String)
    /// A Rating in stars, as the breakdown counts it.
    case rating(Double)
}

extension CatalogStore {
    /// The finished Read-throughs that ended in year, or at any time for nil,
    /// whose Book matches filter, the latest finish first, as the server's
    /// `StatsFinished` lists them. Names match ignoring case, as the
    /// breakdowns group them across Libraries; an author is any of the
    /// Book's people, whatever their role; a Rating is the Book's current
    /// one. A detached Read-through has no Book to match, so it's left out.
    static func statsFinished(_ db: Database, year: Int?, filter: StatsFinishedFilter) throws -> [HistoryEntry] {
        var sql = """
            SELECT rt.id, rt.outcome, rt.paused, rt.begin_day, rt.end_day, rt.book_title, rt.book_authors, rt.pending,
                b.id AS present
            FROM read_throughs rt LEFT JOIN books b ON b.id = rt.book_id
            WHERE rt.outcome = 'finished' AND rt.book_id IS NOT NULL
            """
        var arguments: StatementArguments = []
        if let year {
            sql += " AND rt.end_day >= ? AND rt.end_day <= ?"
            arguments += [String(format: "%04d-01-01", year), String(format: "%04d-12-31", year)]
        }
        switch filter {
        case .genre(let name):
            sql += " AND EXISTS (SELECT 1 FROM book_genres g WHERE g.book_id = rt.book_id AND g.name = ? COLLATE NOCASE)"
            arguments += [name]
        case .tag(let name):
            sql += " AND EXISTS (SELECT 1 FROM book_tags t WHERE t.book_id = rt.book_id AND t.name = ? COLLATE NOCASE)"
            arguments += [name]
        case .author(let name):
            sql += " AND EXISTS (SELECT 1 FROM book_authors a WHERE a.book_id = rt.book_id AND a.name = ? COLLATE NOCASE)"
            arguments += [name]
        case .rating(let stars):
            sql += " AND EXISTS (SELECT 1 FROM user_books ub WHERE ub.book_id = rt.book_id AND ub.rating = ?)"
            arguments += [Int((stars * 4).rounded())]
        }
        sql += " ORDER BY rt.end_day IS NULL, rt.end_day DESC, rt.begin_day IS NULL, rt.begin_day DESC, rt.id DESC"

        return try historyEntries(db, rows: Row.fetchAll(db, sql: sql, arguments: arguments))
    }
}
