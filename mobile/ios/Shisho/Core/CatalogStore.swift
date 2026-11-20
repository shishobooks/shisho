import Foundation
import GRDB
import ShishoAPI

/// A Book as the Library grid shows it.
struct BookSummary: Identifiable, Hashable, Sendable {
    let id: Int
    let libraryID: Int
    let title: String
    let authors: String
    let coverVersion: String
    let coverAspectRatio: String
    /// The File Types of its main Files, in their order, each once.
    let fileTypes: [String]
}

/// A Book as its detail screen shows it.
struct BookDetail: Hashable, Sendable {
    struct Author: Hashable, Sendable {
        let name: String
        let role: String?
    }

    struct Series: Hashable, Sendable {
        let name: String
        let number: Double?
        let numberEnd: Double?
    }

    struct File: Hashable, Identifiable, Sendable {
        let id: Int
        let fileType: String
        let fileRole: String
        let sizeBytes: Int
        /// The File's label among its Book's Files: its name, or its
        /// filename. Empty from a server that doesn't send one.
        let displayName: String
    }

    let summary: BookSummary
    let subtitle: String?
    let authors: [Author]
    let series: [Series]
    let files: [File]
}

struct LibrarySummary: Identifiable, Hashable, Sendable {
    let id: Int
    let name: String
}

/// Where the sync is in the change feed (ADR 0023).
struct SyncState: Equatable, Sendable {
    var cursor: Int
    /// Increases each time the catalog is downloaded again from the start.
    var generation: Int
    /// The catalog is being downloaded again: rows from an older generation
    /// are kept on screen until it finishes, then dropped.
    var startingOver: Bool
    var lastSyncedAt: Date?
    /// How far the server's deletions had expired at the last page, which
    /// the next request sends back with the cursor.
    var expiredThrough: Int = 0
}

/// One account's copy of the catalog: every screen reads from it, and the
/// network only updates it (ADR 0021, ADR 0023).
///
/// Schema changes are GRDB migrations, registered in order in `migrator`
/// and never edited once released: a change is a new migration. Until then,
/// edit them in place; debug builds rebuild a database whose migrations
/// changed. The catalog
/// can always be downloaded again, but later tables (the journal of changes
/// made offline) can't, so every change migrates rather than wipes.
final class CatalogStore: Sendable {
    /// The kinds and fields of the feed this version understands. When an
    /// app update understands more, raise it, and the catalog downloads
    /// again from the start in the background.
    static let feedVersion = 1

    let pool: DatabasePool

    init(path: String) throws {
        var configuration = Configuration()
        configuration.foreignKeysEnabled = true
        pool = try DatabasePool(path: path, configuration: configuration)
        try CatalogStore.migrator.migrate(pool)
    }

    static var migrator: DatabaseMigrator {
        var migrator = DatabaseMigrator()
        #if DEBUG
        migrator.eraseDatabaseOnSchemaChange = true
        #endif
        migrator.registerMigration("v1: catalog") { db in
            try db.create(table: "sync_state") { t in
                t.primaryKey("id", .integer).check { $0 == 1 }
                t.column("cursor", .integer).notNull().defaults(to: 0)
                t.column("generation", .integer).notNull().defaults(to: 0)
                t.column("starting_over", .boolean).notNull().defaults(to: false)
                t.column("feed_version", .integer).notNull().defaults(to: 0)
                t.column("last_synced_at", .datetime)
                t.column("expired_through", .integer).notNull().defaults(to: 0)
            }
            try db.execute(sql: "INSERT INTO sync_state (id) VALUES (1)")
            try db.create(table: "libraries") { t in
                t.primaryKey("id", .integer)
                t.column("name", .text).notNull()
                t.column("cover_aspect_ratio", .text).notNull()
                t.column("sort_spec", .text).notNull().defaults(to: "")
                t.column("generation", .integer).notNull()
            }
            try db.create(table: "books") { t in
                t.primaryKey("id", .integer)
                t.column("library_id", .integer).notNull().indexed()
                t.column("title", .text).notNull()
                t.column("subtitle", .text)
                t.column("sort_title", .text).notNull()
                t.column("authors", .text).notNull()
                t.column("cover_version", .text).notNull()
                t.column("created_at", .double).notNull()
                t.column("generation", .integer).notNull()
            }
            try db.create(index: "books_by_date_added", on: "books", columns: ["created_at", "id"])
            try db.create(table: "book_authors") { t in
                t.column("book_id", .integer).notNull().references("books", onDelete: .cascade)
                t.column("position", .integer).notNull()
                t.column("person_id", .integer).notNull()
                t.column("name", .text).notNull()
                t.column("sort_name", .text).notNull()
                t.column("role", .text)
                t.primaryKey(["book_id", "position"])
            }
            try db.create(table: "book_series") { t in
                t.column("book_id", .integer).notNull().references("books", onDelete: .cascade)
                t.column("position", .integer).notNull()
                t.column("series_id", .integer).notNull()
                t.column("name", .text).notNull()
                t.column("sort_name", .text).notNull()
                t.column("number", .double)
                t.column("number_end", .double)
                t.column("unit", .text)
                t.primaryKey(["book_id", "position"])
            }
            try db.create(table: "book_files") { t in
                t.primaryKey("id", .integer)
                t.column("book_id", .integer).notNull().references("books", onDelete: .cascade).indexed()
                t.column("position", .integer).notNull()
                t.column("file_type", .text).notNull()
                t.column("file_role", .text).notNull()
                t.column("size_bytes", .integer).notNull()
                t.column("display_name", .text).notNull()
                t.column("release_date", .double)
                t.column("page_count", .integer)
                t.column("duration_seconds", .double)
            }
        }
        return migrator
    }

    // MARK: - Sync state

    func syncState() throws -> SyncState {
        try pool.read(CatalogStore.syncState)
    }

    static func syncState(_ db: Database) throws -> SyncState {
        let row = try Row.fetchOne(db, sql: "SELECT cursor, generation, starting_over, last_synced_at, expired_through FROM sync_state WHERE id = 1")
        return SyncState(
            cursor: row?["cursor"] ?? 0,
            generation: row?["generation"] ?? 0,
            startingOver: row?["starting_over"] ?? false,
            lastSyncedAt: row?["last_synced_at"],
            expiredThrough: row?["expired_through"] ?? 0
        )
    }

    /// Starts downloading the catalog again from the start when the server
    /// says the cursor is too old, or this version understands more of the
    /// feed than the copy was made with. The current copy stays on screen
    /// until the download finishes.
    ///
    /// When the server said so, at is the cursor it said it about: if the
    /// copy has moved on since, another sync already handled it.
    func startOverIfNeeded(serverSaidSo: Bool = false, at cursor: Int? = nil) throws {
        try pool.write { db in
            let feedVersion = try Int.fetchOne(db, sql: "SELECT feed_version FROM sync_state WHERE id = 1") ?? 0
            let state = try CatalogStore.syncState(db)
            guard serverSaidSo || feedVersion < CatalogStore.feedVersion else { return }
            if let cursor, cursor != state.cursor { return }
            // A copy made from the start that never got its first page is
            // still the one being made.
            if state.startingOver && state.cursor == 0 { return }
            try db.execute(
                sql: "UPDATE sync_state SET cursor = 0, generation = generation + 1, starting_over = 1, feed_version = ? WHERE id = 1",
                arguments: [CatalogStore.feedVersion]
            )
        }
    }

    /// Applies one page of the feed in one transaction, and moves the
    /// cursor past it. Kinds this version doesn't know are skipped (ADR
    /// 0023). The last page of a download from the start drops whatever it
    /// didn't send.
    ///
    /// after is the cursor the page was asked for. A page that doesn't
    /// continue the copy, from a sync that was stopped and answered late, is
    /// dropped and apply returns false. Tests that apply pages in order may
    /// leave it out.
    @discardableResult
    func apply(_ page: Shishoapp.FeedResponse, after cursor: Int? = nil) throws -> Bool {
        try pool.write { db in
            let state = try CatalogStore.syncState(db)
            if let cursor, cursor != state.cursor {
                return false
            }
            let generation = state.generation
            for entry in page.entries {
                switch entry.kind {
                case .book:
                    if entry.deleted {
                        try db.execute(sql: "DELETE FROM books WHERE id = ?", arguments: [entry.id])
                    } else if let book = entry.book {
                        try CatalogStore.upsert(book, generation: generation, in: db)
                    }
                case .library:
                    if entry.deleted {
                        try db.execute(sql: "DELETE FROM books WHERE library_id = ?", arguments: [entry.id])
                        try db.execute(sql: "DELETE FROM libraries WHERE id = ?", arguments: [entry.id])
                    } else if let library = entry.library {
                        try db.execute(
                            sql: """
                            INSERT INTO libraries (id, name, cover_aspect_ratio, sort_spec, generation) VALUES (?, ?, ?, ?, ?)
                            ON CONFLICT (id) DO UPDATE SET name = excluded.name,
                                cover_aspect_ratio = excluded.cover_aspect_ratio, sort_spec = excluded.sort_spec,
                                generation = excluded.generation
                            """,
                            arguments: [library.id, library.name, library.coverAspectRatio.rawValue, library.sortSpec, generation]
                        )
                    }
                case .unknown:
                    continue
                }
            }
            try db.execute(sql: "UPDATE sync_state SET cursor = ?, expired_through = ?, last_synced_at = ? WHERE id = 1",
                           arguments: [page.cursor, page.expiredThrough, Date()])
            if !page.hasMore, try CatalogStore.syncState(db).startingOver {
                try db.execute(sql: "DELETE FROM books WHERE generation < ?", arguments: [generation])
                try db.execute(sql: "DELETE FROM libraries WHERE generation < ?", arguments: [generation])
                try db.execute(sql: "UPDATE sync_state SET starting_over = 0 WHERE id = 1")
            }
            return true
        }
    }

    private static func upsert(_ book: Shishoapp.FeedBook, generation: Int, in db: Database) throws {
        try db.execute(
            sql: """
            INSERT INTO books (id, library_id, title, subtitle, sort_title, authors, cover_version, created_at, generation)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT (id) DO UPDATE SET library_id = excluded.library_id, title = excluded.title,
                subtitle = excluded.subtitle, sort_title = excluded.sort_title, authors = excluded.authors,
                cover_version = excluded.cover_version, created_at = excluded.created_at, generation = excluded.generation
            """,
            arguments: [book.id, book.libraryId, book.title, book.subtitle, book.sortTitle,
                        book.authors.map(\.name).joined(separator: ", "), book.coverVersion,
                        CatalogStore.timestamp(book.createdAt) ?? 0, generation]
        )
        for table in ["book_authors", "book_series", "book_files"] {
            try db.execute(sql: "DELETE FROM \(table) WHERE book_id = ?", arguments: [book.id])
        }
        for (position, author) in book.authors.enumerated() {
            try db.execute(
                sql: "INSERT INTO book_authors (book_id, position, person_id, name, sort_name, role) VALUES (?, ?, ?, ?, ?, ?)",
                arguments: [book.id, position, author.personId, author.name, author.sortName, author.role?.rawValue]
            )
        }
        for (position, series) in book.series.enumerated() {
            try db.execute(
                sql: "INSERT INTO book_series (book_id, position, series_id, name, sort_name, number, number_end, unit) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                arguments: [book.id, position, series.seriesId, series.name, series.sortName, series.number, series.numberEnd, series.unit?.rawValue]
            )
        }
        // A File moved between Books can arrive in its new Book before its
        // old Book's entry, which still lists it, so it moves here.
        for (position, file) in book.files.enumerated() {
            try db.execute(
                sql: """
                INSERT INTO book_files (id, book_id, position, file_type, file_role, size_bytes, display_name, release_date, page_count, duration_seconds)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                ON CONFLICT (id) DO UPDATE SET book_id = excluded.book_id, position = excluded.position,
                    file_type = excluded.file_type, file_role = excluded.file_role, size_bytes = excluded.size_bytes,
                    display_name = excluded.display_name,
                    release_date = excluded.release_date, page_count = excluded.page_count,
                    duration_seconds = excluded.duration_seconds
                """,
                arguments: [file.id, book.id, position, file.fileType.rawValue, file.fileRole.rawValue, file.sizeBytes,
                            file.displayName, file.releaseDate.map(CatalogStore.timestamp), file.pageCount, file.audiobookDurationSeconds]
            )
        }
    }

    /// Seconds since 1970 of an RFC 3339 time from the server, which writes
    /// fractional seconds only when there are any; nil when it can't be read.
    static func timestamp(_ value: String) -> Double? {
        let withFraction = Date.ISO8601FormatStyle(includingFractionalSeconds: true)
        let date = (try? withFraction.parse(value)) ?? (try? Date.ISO8601FormatStyle().parse(value))
        return date?.timeIntervalSince1970
    }

    // MARK: - Reading
    //
    // Each read is a static function of a Database, so a screen can observe
    // it (ValueObservation) and update whenever a sync changes what it read.

    static func libraries(_ db: Database) throws -> [LibrarySummary] {
        try Row.fetchAll(db, sql: "SELECT id, name FROM libraries ORDER BY name COLLATE NOCASE").map {
            LibrarySummary(id: $0["id"], name: $0["name"])
        }
    }

    /// The Books of libraryID, or of every Library, in the user's saved sort
    /// for it, as the web app shows them. Every Library together sorts by
    /// the sort they share, as a single Library does, or newest first.
    static func books(_ db: Database, libraryID: Int?) throws -> [BookSummary] {
        var sql = """
        SELECT b.id, b.library_id, b.title, b.authors, b.cover_version,
            COALESCE(l.cover_aspect_ratio, 'book') AS aspect
        FROM books b
        LEFT JOIN libraries l ON l.id = b.library_id
        """
        var arguments: StatementArguments = []
        var spec = ""
        if let libraryID {
            sql += " WHERE b.library_id = ?"
            arguments = [libraryID]
            spec = try String.fetchOne(db, sql: "SELECT sort_spec FROM libraries WHERE id = ?", arguments: [libraryID]) ?? ""
        } else {
            let specs = try String.fetchAll(db, sql: "SELECT DISTINCT sort_spec FROM libraries")
            spec = specs.count == 1 ? specs[0] : ""
        }
        sql += " ORDER BY " + LibrarySort.orderBy(spec)
        let rows = try Row.fetchAll(db, sql: sql, arguments: arguments)
        let fileTypes = try mainFileTypes(db, libraryID: libraryID)
        return rows.map { summary($0, fileTypes: fileTypes[$0["id"]] ?? []) }
    }

    static func book(_ db: Database, id: Int) throws -> BookDetail? {
        guard let row = try Row.fetchOne(db, sql: """
            SELECT b.id, b.library_id, b.title, b.authors, b.cover_version, b.subtitle,
                COALESCE(l.cover_aspect_ratio, 'book') AS aspect
            FROM books b
            LEFT JOIN libraries l ON l.id = b.library_id
            WHERE b.id = ?
            """, arguments: [id])
        else { return nil }
        let authors = try Row.fetchAll(db, sql: "SELECT name, role FROM book_authors WHERE book_id = ? ORDER BY position", arguments: [id]).map {
            BookDetail.Author(name: $0["name"], role: $0["role"])
        }
        let series = try Row.fetchAll(db, sql: "SELECT name, number, number_end FROM book_series WHERE book_id = ? ORDER BY position", arguments: [id]).map {
            BookDetail.Series(name: $0["name"], number: $0["number"], numberEnd: $0["number_end"])
        }
        let files = try Row.fetchAll(db, sql: "SELECT id, file_type, file_role, size_bytes, display_name FROM book_files WHERE book_id = ? ORDER BY position", arguments: [id]).map {
            BookDetail.File(id: $0["id"], fileType: $0["file_type"], fileRole: $0["file_role"], sizeBytes: $0["size_bytes"], displayName: $0["display_name"])
        }
        let fileTypes = orderedUnique(files.filter { $0.fileRole != "supplement" }.map(\.fileType))
        return BookDetail(summary: summary(row, fileTypes: fileTypes), subtitle: row["subtitle"], authors: authors, series: series, files: files)
    }

    /// Each Book's main File Types, in order, each once.
    /// The main File Types of each Book in libraryID, or in every Library.
    private static func mainFileTypes(_ db: Database, libraryID: Int?) throws -> [Int: [String]] {
        var byBook: [Int: [String]] = [:]
        var sql = "SELECT f.book_id, f.file_type FROM book_files f"
        var arguments: StatementArguments = []
        if let libraryID {
            sql += " JOIN books b ON b.id = f.book_id AND b.library_id = ?"
            arguments = [libraryID]
        }
        sql += " WHERE f.file_role != 'supplement' ORDER BY f.book_id, f.position"
        let rows = try Row.fetchAll(db, sql: sql, arguments: arguments)
        for row in rows {
            let bookID: Int = row["book_id"]
            let fileType: String = row["file_type"]
            if !(byBook[bookID] ?? []).contains(fileType) {
                byBook[bookID, default: []].append(fileType)
            }
        }
        return byBook
    }

    private static func orderedUnique(_ values: [String]) -> [String] {
        var seen = Set<String>()
        return values.filter { seen.insert($0).inserted }
    }

    private static func summary(_ row: Row, fileTypes: [String]) -> BookSummary {
        BookSummary(id: row["id"], libraryID: row["library_id"], title: row["title"], authors: row["authors"],
                    coverVersion: row["cover_version"], coverAspectRatio: row["aspect"], fileTypes: fileTypes)
    }

    // Conveniences for code that reads once rather than observing.

    func libraries() throws -> [LibrarySummary] {
        try pool.read(CatalogStore.libraries)
    }

    func books(libraryID: Int?) throws -> [BookSummary] {
        try pool.read { try CatalogStore.books($0, libraryID: libraryID) }
    }

    func book(id: Int) throws -> BookDetail? {
        try pool.read { try CatalogStore.book($0, id: id) }
    }
}
