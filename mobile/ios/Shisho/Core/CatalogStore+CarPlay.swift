import Foundation
import GRDB

/// An audiobook File as CarPlay lists it. A Book with several audiobooks,
/// such as an abridged and an unabridged edition, is listed once for each.
struct CarPlayAudiobook: Equatable, Sendable {
    let fileID: Int
    let bookID: Int
    let title: String
    let authors: String
    /// The File's label, when its row needs one to be told apart: a Book
    /// with more than one audiobook, or a supplement; nil otherwise.
    let label: String?
    /// Keys the File's own cover, the audiobook's, never its Book's, which
    /// can be an ebook's; empty when the File has none.
    let coverVersion: String
    /// How far through the File the user is, from 0 to 1, to the whole
    /// percent; nil for none.
    let progress: Double?
}

/// A row in one of CarPlay's lists: an audiobook to play, or a Library to
/// open.
enum CarPlayRow: Equatable, Sendable {
    case audiobook(CarPlayAudiobook)
    case library(LibrarySummary)
}

extension CatalogStore {
    /// The tables CarPlay's lists read, which their observations track.
    /// They leave out local_positions and the journal, which the player
    /// writes every few seconds: local_progress changes with the percent the
    /// lists show, and the order Recent is in can wait for it.
    static let carPlayTables = [
        "books", "book_files", "book_authors", "book_series", "libraries", "reading_positions", "read_throughs", "user_books",
        "local_progress", "downloads",
    ].map(Table.init)

    /// Each main audiobook File, with its Book, its label, how many main
    /// audiobooks its Book has, and when it was last listened to on any
    /// device, 0 for never: the CTE `listened`, for a WITH clause. Ordered
    /// by `touched DESC, position`, a Book's first is the one its Listen
    /// button plays.
    static let listenedSQL = """
        listened AS (
            SELECT f.id AS file_id, f.book_id, f.position, f.cover_version, f.duration_seconds, f.display_name,
                COUNT(*) OVER (PARTITION BY f.book_id) AS audiobooks,
                (l.file_id IS NOT NULL OR p.file_id IS NOT NULL) AS started,
                MAX(COALESCE(l.updated_at, 0), COALESCE(p.occurred_at, 0)) AS touched
            FROM book_files f
            LEFT JOIN local_positions l ON l.file_id = f.id
            LEFT JOIN reading_positions p ON p.file_id = f.id
            WHERE f.file_type = 'm4b' AND f.file_role != 'supplement'
        )
        """

    /// This device's place in the File in column to the whole percent, or
    /// else the server's, as the Downloads tab shows it.
    private static func fileProgressSQL(_ column: String) -> String {
        """
        COALESCE((SELECT percentage FROM local_progress WHERE file_id = \(column)),
            (SELECT percentage FROM reading_positions WHERE file_id = \(column))) AS progress
        """
    }

    private static func audiobook(_ row: Row, title: String, authors: String, progress: Double?) -> CarPlayAudiobook {
        let name: String = row["display_name"]
        let needsLabel = (row["audiobooks"] as Int? ?? 1) > 1 || (row["file_role"] as String?) == "supplement"
        return CarPlayAudiobook(fileID: row["file_id"], bookID: row["book_id"], title: title, authors: authors,
                                label: needsLabel && !name.isEmpty ? name : nil, coverVersion: row["cover_version"],
                                // To the whole percent, so a list doesn't redraw
                                // for each save of the place as it plays.
                                progress: progress.map { (min(1, max(0, $0)) * 100).rounded() / 100 })
    }

    /// The Statuses whose Books aren't being listened to: finished, given up
    /// on, or set aside.
    private static let notListeningStatuses = "'read', 'did_not_finish', 'paused'"

    /// The audiobooks the user is partway through, the one listened to last
    /// first, at most limit: each audiobook File started, of a Book being
    /// listened to, unless it opens at its end. Each is where the player
    /// would open it (`PlaceRules.open`).
    static func continueListening(_ db: Database, limit: Int, ownDeviceID: String) throws -> [CarPlayAudiobook] {
        guard limit > 0 else { return [] }
        let rows = try Row.fetchAll(db, sql: """
            WITH \(listenedSQL)
            SELECT l.*, b.title, b.authors
            FROM listened l JOIN books b ON b.id = l.book_id
            WHERE l.started AND COALESCE(\(statusExpression("b.id")), '') NOT IN (\(notListeningStatuses))
            ORDER BY l.touched DESC, l.file_id
            """)
        var listed: [CarPlayAudiobook] = []
        for row in rows {
            let fileID: Int = row["file_id"]
            let duration: Double = row["duration_seconds"] ?? 0
            let start = PlaceRules.open(local: try localPlace(db, fileID: fileID), server: try serverPlace(db, fileID: fileID),
                                        ownDeviceID: ownDeviceID).start
            if ListeningRules.isAtEnd(start, duration: duration) { continue }
            listed.append(audiobook(row, title: row["title"], authors: row["authors"], progress: duration > 0 ? start / duration : nil))
            if listed.count == limit { break }
        }
        return listed
    }

    /// The audiobook Files on the phone, supplements too, the newest
    /// Download first. A Download being replaced still plays the copy it has.
    static func downloadedAudiobooks(_ db: Database) throws -> [CarPlayAudiobook] {
        try Row.fetchAll(db, sql: """
            SELECT f.id AS file_id, f.book_id, f.cover_version, f.display_name, f.file_role, b.title, b.authors,
                (SELECT COUNT(*) FROM book_files g
                 WHERE g.book_id = f.book_id AND g.file_type = 'm4b' AND g.file_role != 'supplement') AS audiobooks,
                \(fileProgressSQL("f.id"))
            FROM downloads d
            JOIN book_files f ON f.id = d.file_id
            JOIN books b ON b.id = f.book_id
            WHERE (d.state = 'done' OR d.replacing) AND f.file_type = 'm4b'
            ORDER BY d.requested_at DESC, d.file_id
            """).map { audiobook($0, title: $0["title"], authors: $0["authors"], progress: $0["progress"]) }
    }

    /// The Libraries with an audiobook, by name, as CarPlay offers them.
    static func audiobookLibraries(_ db: Database) throws -> [LibrarySummary] {
        try Row.fetchAll(db, sql: """
            SELECT l.id, l.name FROM libraries l
            WHERE EXISTS (
                SELECT 1 FROM books b JOIN book_files f ON f.book_id = b.id
                WHERE b.library_id = l.id AND f.file_type = 'm4b' AND f.file_role != 'supplement'
            )
            ORDER BY l.name COLLATE NOCASE
            """).map { LibrarySummary(id: $0["id"], name: $0["name"]) }
    }

    /// A Library's audiobooks: its Books in the order the Library shows them
    /// (sort, the one picked on this device, or else the user's saved one),
    /// and each Book's audiobooks in its order of Files. A Book without one
    /// isn't listed.
    static func audiobooks(_ db: Database, libraryID: Int, sort: String? = nil) throws -> [CarPlayAudiobook] {
        let books = try CatalogStore.books(db, libraryID: libraryID, filter: LibraryFilter(fileTypes: ["m4b"]), sort: sort)
        guard !books.isEmpty else { return [] }
        let files = try Row.fetchAll(db, sql: """
            WITH \(listenedSQL)
            SELECT l.*, \(fileProgressSQL("l.file_id")) FROM listened l JOIN books b ON b.id = l.book_id
            WHERE b.library_id = ?
            ORDER BY l.book_id, l.position
            """, arguments: [libraryID])
        let byBook = Dictionary(grouping: files) { $0["book_id"] as Int }
        return books.flatMap { book in
            (byBook[book.id] ?? []).map { audiobook($0, title: book.title, authors: book.authors, progress: $0["progress"]) }
        }
    }

    /// What CarPlay's Library tab lists: the Libraries with audiobooks, or,
    /// with only one, its audiobooks. sort gives a Library's order picked on
    /// this device.
    static func carPlayLibraryTab(_ db: Database, sort: @Sendable (Int) -> String?) throws -> [CarPlayRow] {
        let libraries = try audiobookLibraries(db)
        if libraries.count == 1, let only = libraries.first {
            return try audiobooks(db, libraryID: only.id, sort: sort(only.id)).map(CarPlayRow.audiobook)
        }
        return libraries.map(CarPlayRow.library)
    }
}
