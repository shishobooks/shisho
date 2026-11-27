import Foundation
import GRDB
import ShishoAPI

/// What the Library grid is narrowed to, as the web gallery's filters do.
/// Genres and Tags go by name, so a filter carries across Libraries.
struct LibraryFilter: Hashable, Sendable {
    /// Reading Statuses by the server's names, with "none" for a Book
    /// without one.
    var statuses: Set<String> = []
    /// The Rating range, in quarter stars, as the web's slider sets it;
    /// either end nil leaves it open. With neither, unrated Books count too.
    var minRating: Int?
    var maxRating: Int?
    var fileTypes: Set<String> = []
    var genres: Set<String> = []
    var tags: Set<String> = []
    /// A BCP 47 tag, as the Files have it, or nil for every language.
    var language: String?
    /// Only Books with a File downloaded to this device. The app's own: the
    /// web has no Downloads.
    var downloaded = false

    var isEmpty: Bool { self == LibraryFilter() }
}

/// A genre or tag the filters offer, with how many Books have it.
struct FilterOption: Hashable, Sendable, Identifiable {
    let name: String
    let count: Int
    var id: String { name }
}

/// The choices the filters offer for a Library, from what its Books have.
struct FilterOptions: Hashable, Sendable {
    var fileTypes: [String] = []
    var genres: [FilterOption] = []
    var tags: [FilterOption] = []
    var languages: [String] = []
    /// The tag namespaces the tag picker can narrow to.
    var tagNamespaces: [String] = []
}

extension CatalogStore {
    /// The columns a BookSummary reads about the user's own reading, for
    /// `books b`: the Reading Status as the server derives it, the Rating,
    /// and how far through the Book a Read-through underway is: the furthest
    /// place in its main Files, this device's own when it has one.
    /// This device's place comes from local_progress, not local_positions:
    /// the player saves its place every few seconds, and reading that table
    /// would run every list and grid that shows these again each time, all
    /// through a listen.
    static let readingColumns = """
        \(statusExpression("b.id")) AS status,
        (SELECT rating FROM user_books WHERE book_id = b.id) AS rating,
        CASE WHEN EXISTS (SELECT 1 FROM read_throughs WHERE book_id = b.id AND outcome = 'underway') THEN (
            SELECT MAX(COALESCE(lp.percentage, rp.percentage)) FROM book_files f
            LEFT JOIN local_progress lp ON lp.file_id = f.id
            LEFT JOIN reading_positions rp ON rp.file_id = f.id
            WHERE f.book_id = b.id AND f.file_role = 'main'
        ) END AS progress
        """

    /// The creators the grid names under a Book, as the web gallery does:
    /// each person once, in order, and for a comic only its writers and
    /// pencillers, as a comic credits many people.
    static func galleryAuthors(_ book: Shishoapp.FeedBook) -> String {
        let comic = book.files.contains { $0.fileType.rawValue == "cbz" }
        var seen = Set<String>()
        var names: [String] = []
        for author in book.authors {
            if comic, let role = author.role?.rawValue, role != "writer", role != "penciller" { continue }
            if seen.insert(author.name).inserted {
                names.append(author.name)
            }
        }
        return names.joined(separator: ", ")
    }

    /// The SQL condition for filter on `books b`, with its arguments.
    static func condition(_ filter: LibraryFilter) -> (sql: [String], arguments: [any DatabaseValueConvertible]) {
        var sql: [String] = []
        var arguments: [any DatabaseValueConvertible] = []
        func marks(_ count: Int) -> String { Array(repeating: "?", count: count).joined(separator: ",") }
        if !filter.statuses.isEmpty {
            sql.append("COALESCE(\(statusExpression("b.id")), 'none') IN (\(marks(filter.statuses.count)))")
            arguments += filter.statuses.sorted()
        }
        if let minRating = filter.minRating {
            sql.append("(SELECT rating FROM user_books WHERE book_id = b.id) >= ?")
            arguments.append(minRating)
        }
        if let maxRating = filter.maxRating {
            sql.append("(SELECT rating FROM user_books WHERE book_id = b.id) <= ?")
            arguments.append(maxRating)
        }
        if !filter.fileTypes.isEmpty {
            sql.append("EXISTS (SELECT 1 FROM book_files WHERE book_id = b.id AND file_type IN (\(marks(filter.fileTypes.count))))")
            arguments += filter.fileTypes.sorted()
        }
        if !filter.genres.isEmpty {
            sql.append("EXISTS (SELECT 1 FROM book_genres WHERE book_id = b.id AND name COLLATE NOCASE IN (\(marks(filter.genres.count))))")
            arguments += filter.genres.sorted()
        }
        if !filter.tags.isEmpty {
            sql.append("EXISTS (SELECT 1 FROM book_tags WHERE book_id = b.id AND name COLLATE NOCASE IN (\(marks(filter.tags.count))))")
            arguments += filter.tags.sorted()
        }
        if let language = filter.language {
            // The tag or any variant of it ("en" takes in "en-US"), as the
            // server's filter matches.
            sql.append("EXISTS (SELECT 1 FROM book_files WHERE book_id = b.id AND (language = ? OR language LIKE ?))")
            arguments.append(language)
            arguments.append(language + "-%")
        }
        if filter.downloaded {
            sql.append("""
                EXISTS (SELECT 1 FROM downloads d JOIN book_files f ON f.id = d.file_id WHERE f.book_id = b.id AND (d.state = 'done' OR d.replacing))
                """)
        }
        return (sql, arguments)
    }

    static func filterOptions(_ db: Database, libraryID: Int?) throws -> FilterOptions {
        let books = libraryID.map { "SELECT id FROM books WHERE library_id = \($0)" } ?? "SELECT id FROM books"
        return FilterOptions(
            fileTypes: try String.fetchAll(db, sql: "SELECT DISTINCT file_type FROM book_files WHERE book_id IN (\(books)) ORDER BY file_type"),
            genres: try counted("book_genres", books: books, in: db),
            tags: try counted("book_tags", books: books, in: db),
            languages: languageOptions(try String.fetchAll(db, sql: """
                SELECT DISTINCT language FROM book_files WHERE language IS NOT NULL AND book_id IN (\(books)) ORDER BY language
                """)),
            tagNamespaces: try tagNamespaces(db, libraryID: libraryID)
        )
    }

    /// A name as SQLite's NOCASE compares it: ASCII letters folded, nothing
    /// else.
    static func nocase(_ name: String) -> String {
        String(String.UnicodeScalarView(name.unicodeScalars.map { scalar in
            ("A"..."Z").contains(scalar) ? Unicode.Scalar(scalar.value + 32) ?? scalar : scalar
        }))
    }

    /// The languages to offer from the tags the Files have, as the web's
    /// filter groups them: none for a single tag; a bare tag ("en") takes in
    /// its variants ("en-US"), and without one each variant is its own; in
    /// order of their names.
    static func languageOptions(_ tags: [String]) -> [String] {
        guard tags.count >= 2 else { return [] }
        var groups: [String: [String]] = [:]
        for tag in tags {
            groups[String(tag.split(separator: "-").first ?? Substring(tag)), default: []].append(tag)
        }
        let options = groups.flatMap { base, tags in tags.contains(base) ? [base] : tags }
        let name = { (tag: String) in Locale.current.localizedString(forIdentifier: tag) ?? tag }
        return options.sorted { name($0).localizedStandardCompare(name($1)) == .orderedAscending }
    }

    /// The names in a Book join table with how many Books have each, most
    /// first, as the web's pickers list them.
    private static func counted(_ table: String, books: String, in db: Database) throws -> [FilterOption] {
        try Row.fetchAll(db, sql: """
            SELECT name, COUNT(DISTINCT book_id) AS count FROM \(table) WHERE book_id IN (\(books))
            GROUP BY name COLLATE NOCASE ORDER BY count DESC, name COLLATE NOCASE
            """).map { FilterOption(name: $0["name"], count: $0["count"]) }
    }

    /// The sort the user saved on the web for libraryID, or that every
    /// Library shares, in the sort parameter form; empty is newest first.
    static func savedSort(_ db: Database, libraryID: Int?) throws -> String {
        if let libraryID {
            return try String.fetchOne(db, sql: "SELECT sort_spec FROM libraries WHERE id = ?", arguments: [libraryID]) ?? ""
        }
        let specs = try String.fetchAll(db, sql: "SELECT DISTINCT sort_spec FROM libraries")
        return specs.count == 1 ? specs[0] : ""
    }
}
