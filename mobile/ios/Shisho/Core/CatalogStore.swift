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
    /// The user's Reading Status by the server's name, their Rating in
    /// quarter stars, and how far through a Read-through underway is, from 0
    /// to 1, for the cover's bookmark, Rating badge, and progress bar.
    var status: String?
    var rating: Int?
    var progress: Double?
}

/// A Book as its detail screen shows it.
struct BookDetail: Hashable, Sendable {
    struct Author: Hashable, Sendable {
        var personID = 0
        let name: String
        let role: String?
    }

    struct Series: Hashable, Sendable {
        var seriesID = 0
        let name: String
        let number: Double?
        let numberEnd: Double?
    }

    /// A Genre or Tag the Book names.
    struct Named: Hashable, Identifiable, Sendable {
        let id: Int
        let name: String
    }

    /// A File's identifier, such as its ISBN.
    struct Identifier: Hashable, Codable, Sendable {
        let type: String
        let value: String
    }

    struct File: Hashable, Identifiable, Sendable {
        let id: Int
        let fileType: String
        let fileRole: String
        let sizeBytes: Int
        /// The File's label among its Book's Files: its name, or its
        /// filename. Empty from a server that doesn't send one.
        let displayName: String
        /// Keys the File's own cover; empty when it has none.
        var coverVersion = ""
        /// The File's narrators, in order.
        var narrators: [String] = []
        var durationSeconds: Double?
        var narratorIDs: [Int] = []
        var publisher: String?
        var identifiers: [Identifier] = []
        var pageCount: Int?
        var wordCount: Int?
        var releaseDate: Date?
        var codec: String?
        var bitrate: Int?
        var language: String?
        var abridged: Bool?
        var url: String?
        var readingDirection: String?
        var filepath = ""
        var modifiedAt: Date?
        var addedAt: Date?
        /// How far through the File the user is, from 0 to 1: this device's
        /// place when it has one, or the server's.
        var progress: Double?
    }

    let summary: BookSummary
    let subtitle: String?
    let authors: [Author]
    let series: [Series]
    let files: [File]
    var description: String?
    var genres: [Named] = []
    var tags: [Named] = []
    /// When its newest File with a release date was released.
    var published: Date?
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
    static let feedVersion = 6

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
        // The journal (ADR 0024) and what the feed gained with it: chapters,
        // and the user's own Reading Positions and Read-throughs. The journal
        // can't be downloaded again, so this and every later change migrates.
        migrator.registerMigration("v2: listening") { db in
            // Each File's own cover and narrators, which can differ from
            // its Book's other Files.
            try db.alter(table: "book_files") { t in
                t.add(column: "cover_version", .text).notNull().defaults(to: "")
                t.add(column: "narrators", .text).notNull().defaults(to: "[]")
            }
            try db.create(table: "book_file_chapters") { t in
                t.column("file_id", .integer).notNull().references("book_files", onDelete: .cascade)
                t.column("position", .integer).notNull()
                t.column("chapter_id", .integer).notNull()
                t.column("parent_id", .integer)
                t.column("title", .text).notNull()
                t.column("start_ms", .integer)
                t.column("start_page", .integer)
                t.column("href", .text)
                t.primaryKey(["file_id", "position"])
            }
            // The server's place in each File, from the feed.
            try db.create(table: "reading_positions") { t in
                t.primaryKey("file_id", .integer)
                t.column("cfi", .text)
                t.column("page", .integer)
                t.column("seconds", .double)
                t.column("percentage", .double).notNull()
                t.column("occurred_at", .double).notNull()
                t.column("source", .text).notNull()
                t.column("device_id", .text)
                t.column("generation", .integer).notNull()
            }
            try db.create(table: "read_throughs") { t in
                t.primaryKey("id", .integer)
                t.column("book_id", .integer).indexed()
                t.column("begin_day", .text)
                t.column("end_day", .text)
                t.column("outcome", .text).notNull()
                t.column("paused", .boolean).notNull()
                t.column("generation", .integer).notNull()
            }
            // This installation's own place in each File it played.
            try db.create(table: "local_positions") { t in
                t.primaryKey("file_id", .integer)
                t.column("seconds", .double).notNull()
                t.column("percentage", .double).notNull()
                t.column("updated_at", .double).notNull()
                // When the server's place was from when this device first
                // moved since its last sync.
                t.column("base_occurred_at", .double)
                // Another device's place that arrived while this one had
                // moves of its own to send, offered until the user picks.
                t.column("offer_seconds", .double)
                t.column("offer_occurred_at", .double)
                t.column("offer_source", .text)
                t.column("offer_device_id", .text)
            }
            // Changes made on the device, oldest first, until the server
            // takes them in. held_for is the capability list a server that
            // didn't understand one listed then: it waits until that changes.
            try db.create(table: "journal") { t in
                t.autoIncrementedPrimaryKey("seq")
                t.column("event_id", .text).notNull().unique()
                t.column("type", .text).notNull()
                t.column("at", .double).notNull()
                t.column("payload", .blob).notNull()
                t.column("file_id", .integer)
                // Position events of one sitting replace each other until
                // sent; the end of a File never is replaced.
                t.column("coalesce_key", .text).indexed()
                t.column("held_for", .text)
                t.column("held_reason", .text)
            }
            // The sitting being timed, at most one.
            try db.create(table: "open_sitting") { t in
                t.primaryKey("id", .integer).check { $0 == 1 }
                t.column("sitting", .blob).notNull()
            }
            // Changes the server refused, for the user to read.
            try db.create(table: "journal_notes") { t in
                t.autoIncrementedPrimaryKey("id")
                t.column("at", .double).notNull()
                t.column("event_type", .text).notNull()
                t.column("reason", .text).notNull()
            }
        }
        migrator.registerMigration("v3: full catalog", migrate: CatalogStore.migrateFullCatalog)
        // Bookmarks (#916), from the feed and made on the device. A
        // Bookmark's journal events name it, so deleting one not yet sent
        // drops them, and a note edited here shows until its edit is sent.
        migrator.registerMigration("v4: bookmarks") { db in
            try db.create(table: "bookmarks") { t in
                // The id this installation made it under, or "server-<id>"
                // for one made elsewhere.
                t.primaryKey("id", .text)
                t.column("server_id", .integer).unique()
                t.column("local_id", .text)
                t.column("file_id", .integer).notNull().indexed()
                t.column("seconds", .double).notNull()
                // The server's note, and one edited here, shown while its
                // edit waits in the journal.
                t.column("note", .text)
                t.column("local_note", .text)
                t.column("created_at", .double).notNull()
                // Deleted here, until the server's delete arrives.
                t.column("deleted", .boolean).notNull().defaults(to: false)
                t.column("generation", .integer).notNull()
            }
            try db.alter(table: "journal") { t in
                t.add(column: "bookmark_id", .text).indexed()
            }
        }
        // Reading ebooks (#919): this installation's place in an ebook is a
        // CFI, and so is another device's place offered instead. seconds
        // stays an audiobook's, 0 for an ebook.
        migrator.registerMigration("v5: reading") { db in
            try db.alter(table: "local_positions") { t in
                t.add(column: "cfi", .text)
                t.add(column: "offer_cfi", .text)
                t.add(column: "offer_percentage", .double)
            }
        }
        // The name of the Device a place came from, so an offer can say
        // where it's from. Raising feedVersion downloads names for places
        // already synced.
        migrator.registerMigration("v6: device names") { db in
            try db.alter(table: "reading_positions") { t in
                t.add(column: "device_name", .text)
            }
            try db.alter(table: "local_positions") { t in
                t.add(column: "offer_device_name", .text)
            }
        }
        // The chapter and pages left the reader last showed in a File, for
        // its Book's page, which can't work them out without the book.
        migrator.registerMigration("v7: reader places") { db in
            try db.create(table: "reader_places") { t in
                t.primaryKey("file_id", .integer)
                t.column("chapter", .text)
                t.column("pages_left", .integer)
                t.column("percentage", .double).notNull()
            }
        }
        // Downloads (#915): the Files kept on the device to listen to and
        // read offline, and those on their way; and this device's place to
        // the whole percent, for covers and the Downloads tab.
        migrator.registerMigration("v8: downloads") { db in
            try CatalogStore.createDownloads(db)
            try CatalogStore.createLocalProgress(db)
        }
        // A reflowable ebook's word count, shown beside a File's pages or
        // duration. Raising feedVersion downloads it for Files already
        // synced.
        migrator.registerMigration("v9: word counts") { db in
            try db.alter(table: "book_files") { t in
                t.add(column: "word_count", .integer)
            }
        }
        // Reading comics and PDFs (#921): this installation's place in a
        // Page-based File is a 0-based page, and so is another device's
        // place offered instead. The Page flow the user chose for a Book
        // comes with their record of it, and one chosen here shows until
        // the server takes it in.
        migrator.registerMigration("v10: pages") { db in
            try db.alter(table: "local_positions") { t in
                t.add(column: "page", .integer)
                t.add(column: "offer_page", .integer)
            }
            try db.alter(table: "user_books") { t in
                t.add(column: "page_flow", .text)
            }
            try db.create(table: "chosen_page_flows") { t in
                t.primaryKey("book_id", .integer)
                t.column("page_flow", .text).notNull()
                // The journal event sending it, which clears it once the
                // server has answered.
                t.column("event_id", .text).notNull()
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
                sql: "UPDATE sync_state SET cursor = 0, generation = generation + 1, starting_over = 1, access_known = 0, feed_version = ? WHERE id = 1",
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
    ///
    /// ownDeviceID is this installation's Device, so another device's place
    /// arriving while this one has moves to send is kept as an offer.
    /// access is what the page said the user can access, or nil from a
    /// server that doesn't say: the first page of a download from the start
    /// records it.
    @discardableResult
    func apply(_ page: Shishoapp.FeedResponse, after cursor: Int? = nil, ownDeviceID: String = "",
               access: Shishoapp.FeedAccess? = nil) throws -> Bool {
        try pool.write { db in
            let state = try CatalogStore.syncState(db)
            if let cursor, cursor != state.cursor {
                return false
            }
            let generation = state.generation
            // The first page of a download from the start records the access
            // it began with.
            let accessKnown = try Bool.fetchOne(db, sql: "SELECT access_known FROM sync_state WHERE id = 1") ?? false
            if state.cursor == 0, !accessKnown, let access {
                try CatalogStore.setAccess(access, in: db)
            }
            var touched = Touched()
            for entry in page.entries {
                try CatalogStore.applyEntry(entry, generation: generation, ownDeviceID: ownDeviceID, touched: &touched, in: db)
            }
            try CatalogStore.reindex(touched, in: db)
            try db.execute(sql: "UPDATE sync_state SET cursor = ?, expired_through = ?, last_synced_at = ? WHERE id = 1",
                           arguments: [page.cursor, page.expiredThrough, Date()])
            if !page.hasMore {
                try CatalogStore.dropBookmarksTheServerDeleted(in: db)
            }
            let finishing = try CatalogStore.syncState(db).startingOver
            if !page.hasMore, finishing {
                // The Series of Books going now keep their titles in their
                // search rows until written again.
                var dropped = Touched()
                dropped.series = Set(try Int.fetchAll(db, sql: """
                    SELECT DISTINCT bs.series_id FROM book_series bs JOIN books b ON b.id = bs.book_id WHERE b.generation < ?
                    """, arguments: [generation]))
                try db.execute(sql: "DELETE FROM books WHERE generation < ?", arguments: [generation])
                try db.execute(sql: "DELETE FROM libraries WHERE generation < ?", arguments: [generation])
                try db.execute(sql: "DELETE FROM reading_positions WHERE generation < ?", arguments: [generation])
                try db.execute(sql: "DELETE FROM read_throughs WHERE generation < ?", arguments: [generation])
                try CatalogStore.dropOlderCatalog(before: generation, in: db)
                // Bookmarks the server hasn't had yet stay, with their
                // changes. One it had and didn't send is deleted there, and
                // its changes waiting here are moot.
                let gone = """
                    SELECT id FROM bookmarks WHERE generation < ? AND (server_id IS NOT NULL
                        OR id NOT IN (SELECT bookmark_id FROM journal WHERE bookmark_id IS NOT NULL))
                    """
                try db.execute(sql: "DELETE FROM journal WHERE bookmark_id IN (\(gone))", arguments: [generation])
                try db.execute(sql: "DELETE FROM bookmarks WHERE id IN (\(gone))", arguments: [generation])
                // A place the server reset while its deletion expired goes
                // too, unless this device has moves still to send.
                try db.execute(sql: """
                    DELETE FROM local_positions
                    WHERE file_id NOT IN (SELECT file_id FROM reading_positions)
                        AND file_id NOT IN (SELECT file_id FROM journal WHERE type = 'reading_position' AND held_for IS NULL AND file_id IS NOT NULL)
                    """)
                try db.execute(sql: "UPDATE sync_state SET starting_over = 0 WHERE id = 1")
                try CatalogStore.pruneSearch(in: db)
                try CatalogStore.reindex(dropped, in: db)
            }
            return true
        }
    }

    /// Applies one feed entry of any kind this version knows, noting what it
    /// touched for the search index. Kinds it doesn't know are skipped (ADR
    /// 0023).
    static func applyEntry(_ entry: Shishoapp.FeedEntry, generation: Int, ownDeviceID: String, touched: inout Touched,
                           in db: Database) throws {
        switch entry.kind {
        case .book:
            touched.books.insert(entry.id)
            // The Series it leaves keep its title in their search rows until
            // they're written again.
            try touched.series.formUnion(Int.fetchAll(db, sql: "SELECT series_id FROM book_series WHERE book_id = ?", arguments: [entry.id]))
            if entry.deleted {
                try db.execute(sql: "DELETE FROM books WHERE id = ?", arguments: [entry.id])
                try db.execute(sql: "DELETE FROM books_fts WHERE rowid = ?", arguments: [entry.id])
            } else if let book = entry.book {
                try CatalogStore.upsert(book, generation: generation, in: db)
            }
        case .library:
            if entry.deleted {
                try CatalogStore.dropLibrary(entry.id, in: db)
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
        case .readingPosition:
            if entry.deleted {
                try db.execute(sql: "DELETE FROM reading_positions WHERE file_id = ?", arguments: [entry.id])
                // The server's place was reset, as by reading again
                // on the web: this device's synced place goes too,
                // unless it has moves still to send.
                try db.execute(sql: """
                    DELETE FROM local_positions WHERE file_id = ? AND NOT EXISTS (
                        SELECT 1 FROM journal WHERE type = 'reading_position' AND file_id = ? AND held_for IS NULL)
                    """, arguments: [entry.id, entry.id])
            } else if let position = entry.readingPosition {
                try CatalogStore.upsert(position, generation: generation, in: db)
                try CatalogStore.keepOffer(position, ownDeviceID: ownDeviceID, in: db)
            }
        case .readThrough:
            if entry.deleted {
                try db.execute(sql: "DELETE FROM read_throughs WHERE id = ?", arguments: [entry.id])
            } else if let readThrough = entry.readThrough {
                try db.execute(
                    sql: """
                    INSERT INTO read_throughs (id, book_id, begin_day, end_day, outcome, paused, last_read_at, generation)
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?)
                    ON CONFLICT (id) DO UPDATE SET book_id = excluded.book_id, begin_day = excluded.begin_day,
                        end_day = excluded.end_day, outcome = excluded.outcome, paused = excluded.paused,
                        last_read_at = excluded.last_read_at, generation = excluded.generation
                    """,
                    arguments: [readThrough.id, readThrough.bookId, readThrough.beginDay, readThrough.endDay,
                                readThrough.outcome.rawValue, readThrough.paused,
                                readThrough.lastReadAt.flatMap(CatalogStore.timestamp), generation]
                )
            }
        case .bookmark:
            if entry.deleted {
                try db.execute(sql: "DELETE FROM bookmarks WHERE server_id = ?", arguments: [entry.id])
            } else if let bookmark = entry.bookmark {
                try CatalogStore.upsert(bookmark, generation: generation, in: db)
            }
        default:
            try CatalogStore.applyCatalogEntry(entry, generation: generation, touched: &touched, in: db)
        }
    }

    private static func upsert(_ book: Shishoapp.FeedBook, generation: Int, in db: Database) throws {
        try db.execute(
            sql: """
            INSERT INTO books (id, library_id, title, subtitle, sort_title, authors, cover_version, created_at, description, generation)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT (id) DO UPDATE SET library_id = excluded.library_id, title = excluded.title,
                subtitle = excluded.subtitle, sort_title = excluded.sort_title, authors = excluded.authors,
                cover_version = excluded.cover_version, created_at = excluded.created_at,
                description = excluded.description, generation = excluded.generation
            """,
            arguments: [book.id, book.libraryId, book.title, book.subtitle, book.sortTitle,
                        CatalogStore.galleryAuthors(book), book.coverVersion,
                        CatalogStore.timestamp(book.createdAt) ?? 0, book.description, generation]
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
                INSERT INTO book_files (id, book_id, position, file_type, file_role, size_bytes, display_name, release_date, page_count,
                    duration_seconds, cover_version, narrators, word_count)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                ON CONFLICT (id) DO UPDATE SET book_id = excluded.book_id, position = excluded.position,
                    file_type = excluded.file_type, file_role = excluded.file_role, size_bytes = excluded.size_bytes,
                    display_name = excluded.display_name,
                    release_date = excluded.release_date, page_count = excluded.page_count,
                    duration_seconds = excluded.duration_seconds, cover_version = excluded.cover_version,
                    narrators = excluded.narrators, word_count = excluded.word_count
                """,
                arguments: [file.id, book.id, position, file.fileType.rawValue, file.fileRole.rawValue, file.sizeBytes,
                            file.displayName, file.releaseDate.map(CatalogStore.timestamp), file.pageCount, file.audiobookDurationSeconds,
                            file.coverVersion, CatalogStore.namesJSON(file.narrators.map(\.name)), file.wordCount]
            )
            try db.execute(sql: "DELETE FROM book_file_chapters WHERE file_id = ?", arguments: [file.id])
            for (index, chapter) in file.chapters.enumerated() {
                try db.execute(
                    sql: """
                    INSERT INTO book_file_chapters (file_id, position, chapter_id, parent_id, title, start_ms, start_page, href)
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    arguments: [file.id, index, chapter.id, chapter.parentId, chapter.title, chapter.startTimestampMs,
                                chapter.startPage, chapter.href]
                )
            }
        }
        try CatalogStore.upsertCatalogParts(book, in: db)
    }

    private static func upsert(_ position: Shishoapp.FeedReadingPosition, generation: Int, in db: Database) throws {
        guard let occurredAt = CatalogStore.timestamp(position.occurredAt) else { return }
        try db.execute(
            sql: """
            INSERT INTO reading_positions (file_id, cfi, page, seconds, percentage, occurred_at, source, device_id, device_name,
                generation)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT (file_id) DO UPDATE SET cfi = excluded.cfi, page = excluded.page, seconds = excluded.seconds,
                percentage = excluded.percentage, occurred_at = excluded.occurred_at, source = excluded.source,
                device_id = excluded.device_id, device_name = excluded.device_name, generation = excluded.generation
            """,
            arguments: [position.fileId, position.cfi, position.page, position.seconds, position.percentage, occurredAt,
                        position.source.rawValue, position.deviceId, position.deviceName, generation]
        )
    }

    /// Names stored as a JSON array, as a File's narrators are.
    static func namesJSON(_ names: [String]) -> String {
        (try? String(data: JSONEncoder().encode(names), encoding: .utf8)) ?? "[]"
    }

    static func names(_ json: String?) -> [String] {
        guard let data = json?.data(using: .utf8) else { return [] }
        return (try? JSONDecoder().decode([String].self, from: data)) ?? []
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
    /// filter narrows them as the web gallery's filters do, and sort, in the
    /// sort parameter form, replaces the saved sort when it's set.
    static func books(_ db: Database, libraryID: Int?, filter: LibraryFilter = LibraryFilter(), sort: String? = nil) throws -> [BookSummary] {
        var sql = """
        SELECT b.id, b.library_id, b.title, b.authors, b.cover_version,
            COALESCE(l.cover_aspect_ratio, 'book') AS aspect, \(readingColumns)
        FROM books b
        LEFT JOIN libraries l ON l.id = b.library_id
        """
        var conditions: [String] = []
        var values: [any DatabaseValueConvertible] = []
        if let libraryID {
            conditions.append("b.library_id = ?")
            values.append(libraryID)
        }
        let spec = try savedSort(db, libraryID: libraryID)
        let filtered = condition(filter)
        conditions += filtered.sql
        values += filtered.arguments
        if !conditions.isEmpty {
            sql += " WHERE " + conditions.joined(separator: " AND ")
        }
        let arguments = StatementArguments(values.map { Optional($0) })
        sql += " ORDER BY " + LibrarySort.orderBy(sort ?? spec)
        let rows = try Row.fetchAll(db, sql: sql, arguments: arguments)
        let fileTypes = try mainFileTypes(db, libraryID: libraryID)
        return rows.map { summary($0, fileTypes: fileTypes[$0["id"]] ?? []) }
    }

    static func book(_ db: Database, id: Int) throws -> BookDetail? {
        guard let row = try Row.fetchOne(db, sql: """
            SELECT b.id, b.library_id, b.title, b.authors, b.cover_version, b.subtitle, b.description,
                COALESCE(l.cover_aspect_ratio, 'book') AS aspect, \(readingColumns)
            FROM books b
            LEFT JOIN libraries l ON l.id = b.library_id
            WHERE b.id = ?
            """, arguments: [id])
        else { return nil }
        let authors = try Row.fetchAll(db, sql: "SELECT person_id, name, role FROM book_authors WHERE book_id = ? ORDER BY position", arguments: [id]).map {
            BookDetail.Author(personID: $0["person_id"], name: $0["name"], role: $0["role"])
        }
        let series = try Row.fetchAll(db, sql: "SELECT series_id, name, number, number_end FROM book_series WHERE book_id = ? ORDER BY position", arguments: [id]).map {
            BookDetail.Series(seriesID: $0["series_id"], name: $0["name"], number: $0["number"], numberEnd: $0["number_end"])
        }
        let genres = try Row.fetchAll(db, sql: "SELECT genre_id, name FROM book_genres WHERE book_id = ? ORDER BY position", arguments: [id]).map {
            BookDetail.Named(id: $0["genre_id"], name: $0["name"])
        }
        let tags = try Row.fetchAll(db, sql: "SELECT tag_id, name FROM book_tags WHERE book_id = ? ORDER BY position", arguments: [id]).map {
            BookDetail.Named(id: $0["tag_id"], name: $0["name"])
        }
        let files = try Row.fetchAll(db, sql: """
            SELECT id, file_type, file_role, size_bytes, display_name, cover_version, narrators, duration_seconds,
                publisher, identifiers, page_count, word_count, release_date, codec, bitrate, language, abridged, url,
                reading_direction, filepath, file_modified_at, created_at,
                COALESCE((SELECT percentage FROM local_positions WHERE file_id = book_files.id),
                    (SELECT percentage FROM reading_positions WHERE file_id = book_files.id)) AS progress,
                (SELECT json_group_array(person_id) FROM (SELECT person_id FROM book_narrators WHERE file_id = book_files.id ORDER BY position)) AS narrator_ids
            FROM book_files WHERE book_id = ? ORDER BY position
            """, arguments: [id]).map {
            let identifiers = (($0["identifiers"] as String?)?.data(using: .utf8))
                .flatMap { try? JSONDecoder().decode([BookDetail.Identifier].self, from: $0) } ?? []
            let narratorIDs = (($0["narrator_ids"] as String?)?.data(using: .utf8))
                .flatMap { try? JSONDecoder().decode([Int].self, from: $0) } ?? []
            return BookDetail.File(id: $0["id"], fileType: $0["file_type"], fileRole: $0["file_role"], sizeBytes: $0["size_bytes"],
                                   displayName: $0["display_name"], coverVersion: $0["cover_version"],
                                   narrators: CatalogStore.names($0["narrators"]), durationSeconds: $0["duration_seconds"],
                                   narratorIDs: narratorIDs, publisher: $0["publisher"],
                                   identifiers: identifiers, pageCount: $0["page_count"], wordCount: $0["word_count"],
                                   releaseDate: ($0["release_date"] as Double?).map(Date.init(timeIntervalSince1970:)),
                                   codec: $0["codec"], bitrate: $0["bitrate"], language: $0["language"], abridged: $0["abridged"],
                                   url: $0["url"], readingDirection: $0["reading_direction"], filepath: $0["filepath"],
                                   modifiedAt: ($0["file_modified_at"] as Double?).map(Date.init(timeIntervalSince1970:)),
                                   addedAt: ($0["created_at"] as Double?).map(Date.init(timeIntervalSince1970:)),
                                   progress: $0["progress"])
        }
        let fileTypes = orderedUnique(files.filter { $0.fileRole != "supplement" }.map(\.fileType))
        let published = try Double.fetchOne(db, sql: """
            SELECT release_date FROM book_files WHERE book_id = ? AND release_date IS NOT NULL ORDER BY id DESC LIMIT 1
            """, arguments: [id]).map { Date(timeIntervalSince1970: $0) }
        return BookDetail(summary: summary(row, fileTypes: fileTypes), subtitle: row["subtitle"], authors: authors, series: series,
                          files: files, description: row["description"], genres: genres, tags: tags, published: published)
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
                    coverVersion: row["cover_version"], coverAspectRatio: row["aspect"], fileTypes: fileTypes,
                    status: row["status"], rating: row["rating"], progress: row["progress"])
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
