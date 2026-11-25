import Foundation
import GRDB

/// An audiobook as CarPlay lists it: the File that plays, and its Book.
struct CarPlayAudiobook: Equatable, Sendable {
    let fileID: Int
    let bookID: Int
    let title: String
    let authors: String
    /// The cover the lock screen shows too: the File's own, or its Book's.
    let cover: CoverCache.Owner
    let coverVersion: String
    /// How far through it the user is, from 0 to 1, to the whole percent,
    /// as the same Book or Download shows it on the phone; nil for none.
    let progress: Double?
}

extension CatalogStore {
    /// The tables CarPlay's lists read, which their observations track.
    /// They leave out local_positions and the journal, which the player
    /// writes every few seconds: local_progress changes with the percent the
    /// lists show, and the order Continue Listening is in can wait for it.
    static let carPlayTables = [
        "books", "book_files", "book_authors", "book_series", "libraries", "reading_positions", "read_throughs", "user_books",
        "local_progress", "downloads",
    ].map(Table.init)

    /// Each main audiobook File's Book and when it was last listened to on
    /// any device, 0 for never: the CTE `listened`, for a WITH clause.
    /// Ordered by `touched DESC, position`, a Book's first is the one its
    /// Listen button plays.
    static let listenedSQL = """
        listened AS (
            SELECT f.id AS file_id, f.book_id, f.position, f.cover_version, f.duration_seconds,
                (l.file_id IS NOT NULL OR p.file_id IS NOT NULL) AS started,
                MAX(COALESCE(l.updated_at, 0), COALESCE(p.occurred_at, 0)) AS touched
            FROM book_files f
            LEFT JOIN local_positions l ON l.file_id = f.id
            LEFT JOIN reading_positions p ON p.file_id = f.id
            WHERE f.file_type = 'm4b' AND f.file_role != 'supplement'
        )
        """

    private static func audiobook(_ row: Row, title: String, authors: String, bookCover: String, progress: Double?) -> CarPlayAudiobook {
        let fileCover: String = row["cover_version"]
        return CarPlayAudiobook(fileID: row["file_id"], bookID: row["book_id"], title: title, authors: authors,
                                cover: fileCover.isEmpty ? .book(row["book_id"]) : .file(row["file_id"]),
                                coverVersion: fileCover.isEmpty ? bookCover : fileCover,
                                // To the whole percent, so a list doesn't redraw
                                // for each save of the place as it plays.
                                progress: progress.map { (min(1, max(0, $0)) * 100).rounded() / 100 })
    }

    /// The Statuses whose Books aren't being listened to: finished, given up
    /// on, or set aside.
    private static let notListeningStatuses = "'read', 'did_not_finish', 'paused'"

    /// The audiobooks the user is partway through, the one listened to last
    /// first, at most limit: for each Book being listened to, the audiobook
    /// File listened to last, unless it opens at its end. Each is where the
    /// player would open it (`PlaceRules.open`).
    static func continueListening(_ db: Database, limit: Int, ownDeviceID: String) throws -> [CarPlayAudiobook] {
        guard limit > 0 else { return [] }
        let rows = try Row.fetchAll(db, sql: """
            WITH \(listenedSQL),
            newest AS (
                SELECT *, ROW_NUMBER() OVER (PARTITION BY book_id ORDER BY touched DESC, position) AS n FROM listened
            )
            SELECT n.file_id, n.book_id, n.cover_version, n.duration_seconds, b.title, b.authors, b.cover_version AS book_cover
            FROM newest n JOIN books b ON b.id = n.book_id
            WHERE n.n = 1 AND n.started AND COALESCE(\(statusExpression("b.id")), '') NOT IN (\(notListeningStatuses))
            ORDER BY n.touched DESC, n.file_id
            """)
        var listed: [CarPlayAudiobook] = []
        for row in rows {
            let fileID: Int = row["file_id"]
            let duration: Double = row["duration_seconds"] ?? 0
            let start = PlaceRules.open(local: try localPlace(db, fileID: fileID), server: try serverPlace(db, fileID: fileID),
                                        ownDeviceID: ownDeviceID).start
            if ListeningRules.isAtEnd(start, duration: duration) { continue }
            listed.append(audiobook(row, title: row["title"], authors: row["authors"], bookCover: row["book_cover"],
                                    progress: duration > 0 ? start / duration : nil))
            if listed.count == limit { break }
        }
        return listed
    }

    /// The audiobook Files on the phone, the newest Download first, with the
    /// progress the Downloads tab shows. A Download being replaced still
    /// plays the copy it has.
    static func downloadedAudiobooks(_ db: Database) throws -> [CarPlayAudiobook] {
        try Row.fetchAll(db, sql: """
            SELECT f.id AS file_id, f.book_id, f.cover_version, b.title, b.authors, b.cover_version AS book_cover,
                COALESCE((SELECT percentage FROM local_progress WHERE file_id = f.id),
                    (SELECT percentage FROM reading_positions WHERE file_id = f.id)) AS progress
            FROM downloads d
            JOIN book_files f ON f.id = d.file_id
            JOIN books b ON b.id = f.book_id
            WHERE (d.state = 'done' OR d.replacing) AND f.file_type = 'm4b'
            ORDER BY d.requested_at DESC, d.file_id
            """).map {
            audiobook($0, title: $0["title"], authors: $0["authors"], bookCover: $0["book_cover"], progress: $0["progress"])
        }
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

    /// A Library's audiobooks, in the order the Library shows them: sort,
    /// the one picked on this device, or else the user's saved one, with
    /// the progress the Library shows. Each plays what Listen on its page
    /// plays (`audiobookFileID`).
    static func audiobooks(_ db: Database, libraryID: Int, sort: String? = nil) throws -> [CarPlayAudiobook] {
        let books = try CatalogStore.books(db, libraryID: libraryID, filter: LibraryFilter(fileTypes: ["m4b"]), sort: sort)
        guard !books.isEmpty else { return [] }
        let files = try Row.fetchAll(db, sql: """
            WITH \(listenedSQL)
            SELECT l.* FROM listened l JOIN books b ON b.id = l.book_id
            WHERE b.library_id = ?
            ORDER BY l.book_id, l.touched DESC, l.position
            """, arguments: [libraryID])
        var chosen: [Int: Row] = [:]
        for row in files where chosen[row["book_id"]] == nil {
            chosen[row["book_id"]] = row
        }
        return books.compactMap { book in
            chosen[book.id].map {
                audiobook($0, title: book.title, authors: book.authors, bookCover: book.coverVersion, progress: book.progress)
            }
        }
    }
}
