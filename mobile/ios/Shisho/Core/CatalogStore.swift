import Foundation
import ShishoAPI

/// A Book as the Library grid shows it.
struct BookSummary: Identifiable, Hashable {
    let id: Int
    let libraryID: Int
    let title: String
    let authors: String
    let coverVersion: String
    let coverAspectRatio: String
}

/// A Book as its detail screen shows it.
struct BookDetail: Hashable {
    struct Author: Hashable {
        let name: String
        let role: String?
    }

    struct Series: Hashable {
        let name: String
        let number: Double?
        let numberEnd: Double?
    }

    struct File: Hashable, Identifiable {
        let id: Int
        let fileType: String
        let fileRole: String
        let sizeBytes: Int
    }

    let summary: BookSummary
    let subtitle: String?
    let authors: [Author]
    let series: [Series]
    let files: [File]
}

struct LibrarySummary: Identifiable, Hashable {
    let id: Int
    let name: String
}

/// Where the sync is in the change feed (ADR 0023).
struct SyncState: Equatable {
    var cursor: Int
    /// Increases each time the catalog is downloaded again from the start.
    var generation: Int
    /// The catalog is being downloaded again: rows from an older generation
    /// are kept on screen until it finishes, then dropped.
    var startingOver: Bool
    var lastSyncedAt: Date?
}

/// One account's copy of the catalog: every screen reads from it, and the
/// network only updates it (ADR 0021, ADR 0023).
final class CatalogStore: @unchecked Sendable {
    /// The kinds and fields of the feed this version understands. When an
    /// app update understands more, raise it, and the catalog downloads
    /// again from the start in the background.
    static let feedVersion = 1

    let database: Database

    init(path: String) throws {
        database = try Database(path: path)
        try migrate()
    }

    private func migrate() throws {
        if database.userVersion < 1 {
            try database.executeScript("""
            CREATE TABLE IF NOT EXISTS sync_state (
                id INTEGER PRIMARY KEY CHECK (id = 1),
                cursor INTEGER NOT NULL DEFAULT 0,
                generation INTEGER NOT NULL DEFAULT 0,
                starting_over INTEGER NOT NULL DEFAULT 0,
                feed_version INTEGER NOT NULL DEFAULT 0,
                last_synced_at REAL
            );
            INSERT OR IGNORE INTO sync_state (id) VALUES (1);
            CREATE TABLE IF NOT EXISTS libraries (
                id INTEGER PRIMARY KEY,
                name TEXT NOT NULL,
                cover_aspect_ratio TEXT NOT NULL,
                generation INTEGER NOT NULL
            );
            CREATE TABLE IF NOT EXISTS books (
                id INTEGER PRIMARY KEY,
                library_id INTEGER NOT NULL,
                title TEXT NOT NULL,
                subtitle TEXT,
                sort_title TEXT NOT NULL,
                authors TEXT NOT NULL,
                cover_version TEXT NOT NULL,
                generation INTEGER NOT NULL
            );
            CREATE INDEX IF NOT EXISTS ix_books_library_id ON books (library_id);
            CREATE TABLE IF NOT EXISTS book_authors (
                book_id INTEGER NOT NULL REFERENCES books (id) ON DELETE CASCADE,
                position INTEGER NOT NULL,
                person_id INTEGER NOT NULL,
                name TEXT NOT NULL,
                sort_name TEXT NOT NULL,
                role TEXT,
                PRIMARY KEY (book_id, position)
            );
            CREATE TABLE IF NOT EXISTS book_series (
                book_id INTEGER NOT NULL REFERENCES books (id) ON DELETE CASCADE,
                position INTEGER NOT NULL,
                series_id INTEGER NOT NULL,
                name TEXT NOT NULL,
                sort_name TEXT NOT NULL,
                number REAL,
                number_end REAL,
                unit TEXT,
                PRIMARY KEY (book_id, position)
            );
            CREATE TABLE IF NOT EXISTS book_files (
                id INTEGER PRIMARY KEY,
                book_id INTEGER NOT NULL REFERENCES books (id) ON DELETE CASCADE,
                position INTEGER NOT NULL,
                file_type TEXT NOT NULL,
                file_role TEXT NOT NULL,
                size_bytes INTEGER NOT NULL
            );
            CREATE INDEX IF NOT EXISTS ix_book_files_book_id ON book_files (book_id);
            """)
            database.userVersion = 1
        }
    }

    // MARK: - Sync state

    func syncState() throws -> SyncState {
        try database.query("SELECT cursor, generation, starting_over, last_synced_at FROM sync_state WHERE id = 1") {
            SyncState(
                cursor: $0.int(0),
                generation: $0.int(1),
                startingOver: $0.int(2) != 0,
                lastSyncedAt: $0.optionalDouble(3).map(Date.init(timeIntervalSince1970:))
            )
        }.first ?? SyncState(cursor: 0, generation: 0, startingOver: false)
    }

    /// Starts downloading the catalog again from the start when the server
    /// says the cursor is too old, or this version understands more of the
    /// feed than the copy was made with. The current copy stays on screen
    /// until the download finishes.
    func startOverIfNeeded(serverSaidSo: Bool = false) throws {
        try database.transaction {
            let feedVersion = try database.query("SELECT feed_version FROM sync_state WHERE id = 1") { $0.int(0) }.first ?? 0
            let state = try syncState()
            guard serverSaidSo || feedVersion < CatalogStore.feedVersion else { return }
            // A copy made from the start that never finished is still the
            // one being made.
            if state.startingOver && state.cursor == 0 { return }
            try database.execute(
                "UPDATE sync_state SET cursor = 0, generation = generation + 1, starting_over = 1, feed_version = ? WHERE id = 1",
                [.int(CatalogStore.feedVersion)]
            )
        }
    }

    /// Applies one page of the feed in one transaction, and moves the
    /// cursor past it. Kinds this version doesn't know are skipped (ADR
    /// 0023). The last page of a download from the start drops whatever it
    /// didn't send.
    func apply(_ page: Shishoapp.FeedResponse) throws {
        try database.transaction {
            let generation = try syncState().generation
            for entry in page.entries {
                switch entry.kind {
                case .book:
                    if entry.deleted {
                        try database.execute("DELETE FROM books WHERE id = ?", [.int(entry.id)])
                    } else if let book = entry.book {
                        try upsert(book, generation: generation)
                    }
                case .library:
                    if entry.deleted {
                        try database.execute("DELETE FROM books WHERE library_id = ?", [.int(entry.id)])
                        try database.execute("DELETE FROM libraries WHERE id = ?", [.int(entry.id)])
                    } else if let library = entry.library {
                        try database.execute(
                            """
                            INSERT INTO libraries (id, name, cover_aspect_ratio, generation) VALUES (?, ?, ?, ?)
                            ON CONFLICT (id) DO UPDATE SET name = excluded.name,
                                cover_aspect_ratio = excluded.cover_aspect_ratio, generation = excluded.generation
                            """,
                            [.int(library.id), .text(library.name), .text(library.coverAspectRatio.rawValue), .int(generation)]
                        )
                    }
                case .unknown:
                    continue
                }
            }
            try database.execute("UPDATE sync_state SET cursor = ?, last_synced_at = ? WHERE id = 1",
                                 [.int(page.cursor), .double(Date().timeIntervalSince1970)])
            if !page.hasMore, try syncState().startingOver {
                try database.execute("DELETE FROM books WHERE generation < ?", [.int(generation)])
                try database.execute("DELETE FROM libraries WHERE generation < ?", [.int(generation)])
                try database.execute("UPDATE sync_state SET starting_over = 0 WHERE id = 1")
            }
        }
    }

    private func upsert(_ book: Shishoapp.FeedBook, generation: Int) throws {
        let authors = book.authors.map(\.name).joined(separator: ", ")
        try database.execute(
            """
            INSERT INTO books (id, library_id, title, subtitle, sort_title, authors, cover_version, generation)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT (id) DO UPDATE SET library_id = excluded.library_id, title = excluded.title,
                subtitle = excluded.subtitle, sort_title = excluded.sort_title, authors = excluded.authors,
                cover_version = excluded.cover_version, generation = excluded.generation
            """,
            [.int(book.id), .int(book.libraryId), .text(book.title), SQLValue(book.subtitle), .text(book.sortTitle),
             .text(authors), .text(book.coverVersion), .int(generation)]
        )
        for table in ["book_authors", "book_series", "book_files"] {
            try database.execute("DELETE FROM \(table) WHERE book_id = ?", [.int(book.id)])
        }
        for (position, author) in book.authors.enumerated() {
            try database.execute(
                "INSERT INTO book_authors (book_id, position, person_id, name, sort_name, role) VALUES (?, ?, ?, ?, ?, ?)",
                [.int(book.id), .int(position), .int(author.personId), .text(author.name), .text(author.sortName),
                 SQLValue(author.role?.rawValue)]
            )
        }
        for (position, series) in book.series.enumerated() {
            try database.execute(
                "INSERT INTO book_series (book_id, position, series_id, name, sort_name, number, number_end, unit) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                [.int(book.id), .int(position), .int(series.seriesId), .text(series.name), .text(series.sortName),
                 SQLValue(series.number), SQLValue(series.numberEnd), SQLValue(series.unit?.rawValue)]
            )
        }
        // A File moved between Books can arrive in its new Book before its
        // old Book's entry, which still lists it, so it moves here.
        for (position, file) in book.files.enumerated() {
            try database.execute(
                """
                INSERT INTO book_files (id, book_id, position, file_type, file_role, size_bytes) VALUES (?, ?, ?, ?, ?, ?)
                ON CONFLICT (id) DO UPDATE SET book_id = excluded.book_id, position = excluded.position,
                    file_type = excluded.file_type, file_role = excluded.file_role, size_bytes = excluded.size_bytes
                """,
                [.int(file.id), .int(book.id), .int(position), .text(file.fileType.rawValue), .text(file.fileRole.rawValue),
                 .int(file.sizeBytes)]
            )
        }
    }

    // MARK: - Reading

    func libraries() throws -> [LibrarySummary] {
        try database.query("SELECT id, name FROM libraries ORDER BY name COLLATE NOCASE") {
            LibrarySummary(id: $0.int(0), name: $0.string(1))
        }
    }

    /// The Books of libraryID, or of every Library, by sort title.
    func books(libraryID: Int?) throws -> [BookSummary] {
        var sql = """
        SELECT b.id, b.library_id, b.title, b.authors, b.cover_version, COALESCE(l.cover_aspect_ratio, 'book')
        FROM books b LEFT JOIN libraries l ON l.id = b.library_id
        """
        var params: [SQLValue] = []
        if let libraryID {
            sql += " WHERE b.library_id = ?"
            params.append(.int(libraryID))
        }
        sql += " ORDER BY b.sort_title COLLATE NOCASE, b.id"
        return try database.query(sql, params, CatalogStore.summary)
    }

    func book(id: Int) throws -> BookDetail? {
        guard let row = try database.query(
            """
            SELECT b.id, b.library_id, b.title, b.authors, b.cover_version, COALESCE(l.cover_aspect_ratio, 'book'), b.subtitle
            FROM books b LEFT JOIN libraries l ON l.id = b.library_id WHERE b.id = ?
            """, [.int(id)], { (CatalogStore.summary($0), $0.optionalString(6)) }
        ).first else { return nil }
        let authors = try database.query("SELECT name, role FROM book_authors WHERE book_id = ? ORDER BY position", [.int(id)]) {
            BookDetail.Author(name: $0.string(0), role: $0.optionalString(1))
        }
        let series = try database.query("SELECT name, number, number_end FROM book_series WHERE book_id = ? ORDER BY position", [.int(id)]) {
            BookDetail.Series(name: $0.string(0), number: $0.optionalDouble(1), numberEnd: $0.optionalDouble(2))
        }
        let files = try database.query("SELECT id, file_type, file_role, size_bytes FROM book_files WHERE book_id = ? ORDER BY position", [.int(id)]) {
            BookDetail.File(id: $0.int(0), fileType: $0.string(1), fileRole: $0.string(2), sizeBytes: $0.int(3))
        }
        return BookDetail(summary: row.0, subtitle: row.1, authors: authors, series: series, files: files)
    }

    private static func summary(_ row: Row) -> BookSummary {
        BookSummary(id: row.int(0), libraryID: row.int(1), title: row.string(2), authors: row.string(3),
                    coverVersion: row.string(4), coverAspectRatio: row.string(5))
    }
}
