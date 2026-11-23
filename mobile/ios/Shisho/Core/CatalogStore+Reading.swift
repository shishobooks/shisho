import Foundation
import GRDB
import ShishoAPI

/// An entry in the reader's table of contents.
struct ReaderChapter: Equatable, Hashable, Sendable {
    let title: String
    /// Where it starts, as the book's navigation document wrote it, or nil
    /// for a heading with nowhere to go.
    let href: String?
    let depth: Int
}

/// A Reflowable File as the reader needs it, from the device's copy of the
/// catalog.
struct ReadingFile: Equatable, Identifiable, Sendable {
    var id: Int { fileID }
    let fileID: Int
    let book: BookSummary
    let fileType: String
    let displayName: String
    let sizeBytes: Int
    /// Shisho's chapters, nested under their parents, in order (ADR 0022).
    let chapters: [ReaderChapter]

    /// Shisho's chapters are the table of contents for an EPUB. MOBI and
    /// AZW3 use the table of contents inside the file until Shisho's stored
    /// chapter locations are mapped to places the reader can go to.
    var usesShishoChapters: Bool { CatalogStore.navigableChapterTypes.contains(fileType) }
}

/// How far through a Book's ebook the user is, for its Book page.
struct ReadProgress: Equatable, Sendable {
    let fileID: Int
    let percentage: Double
    /// A Read-through of the Book is underway.
    let underway: Bool
    var fileName = ""
    var ebookCount = 1
}

extension CatalogStore {
    /// The File Types the reader lays out with foliate (ADR 0026).
    static let reflowableTypes: Set<String> = ["epub", "kepub", "mobi", "azw3"]
    /// The Reflowable File Types whose Shisho chapters the reader can go to.
    static let navigableChapterTypes: Set<String> = ["epub", "kepub"]

    private static var reflowableList: String {
        reflowableTypes.sorted().map { "'\($0)'" }.joined(separator: ", ")
    }

    // MARK: - Reading

    static func readingFile(_ db: Database, fileID: Int) throws -> ReadingFile? {
        guard let row = try Row.fetchOne(db, sql: """
                SELECT book_id, file_type, display_name, size_bytes FROM book_files WHERE id = ? AND file_type IN (\(reflowableList))
                """, arguments: [fileID]),
              let book = try CatalogStore.book(db, id: row["book_id"])
        else { return nil }
        return ReadingFile(fileID: fileID, book: book.summary, fileType: row["file_type"], displayName: row["display_name"],
                           sizeBytes: row["size_bytes"], chapters: try readerChapters(db, fileID: fileID))
    }

    /// The File's chapters, each nested one deeper than its parent. The feed
    /// sends each after its parent.
    static func readerChapters(_ db: Database, fileID: Int) throws -> [ReaderChapter] {
        var depths: [Int: Int] = [:]
        return try Row.fetchAll(db, sql: """
            SELECT chapter_id, parent_id, title, href FROM book_file_chapters WHERE file_id = ? ORDER BY position
            """, arguments: [fileID]).map { row in
            let parent: Int? = row["parent_id"]
            let depth = parent.flatMap { depths[$0].map { $0 + 1 } } ?? 0
            depths[row["chapter_id"]] = depth
            let href: String? = row["href"]
            return ReaderChapter(title: row["title"], href: href?.isEmpty == true ? nil : href, depth: depth)
        }
    }

    /// Where an ebook opens across devices (`PlaceRules.openEbook`).
    static func ebookOpening(_ db: Database, fileID: Int, ownDeviceID: String) throws -> PlaceRules.EbookChoice {
        PlaceRules.openEbook(local: try localPlace(db, fileID: fileID), server: try serverPlace(db, fileID: fileID),
                             ownDeviceID: ownDeviceID)
    }

    /// The ebook File of bookID the Read button opens: the one read most
    /// recently on any device, or else the first, or nil when the Book has
    /// none.
    static func ebookFileID(_ db: Database, bookID: Int) throws -> Int? {
        try Int.fetchOne(db, sql: """
            SELECT f.id FROM book_files f
            LEFT JOIN local_positions l ON l.file_id = f.id
            LEFT JOIN reading_positions p ON p.file_id = f.id
            WHERE f.book_id = ? AND f.file_type IN (\(reflowableList)) AND f.file_role != 'supplement'
            ORDER BY MAX(COALESCE(l.updated_at, 0), COALESCE(p.occurred_at, 0)) DESC, f.position
            LIMIT 1
            """, arguments: [bookID])
    }

    /// The Book's ebook progress, or nil when it has no ebook.
    static func readProgress(_ db: Database, bookID: Int, ownDeviceID: String) throws -> ReadProgress? {
        guard let fileID = try ebookFileID(db, bookID: bookID) else { return nil }
        let local = try localPlace(db, fileID: fileID)
        let server = try serverPlace(db, fileID: fileID)
        let percentage = PlaceRules.openEbook(local: local, server: server, ownDeviceID: ownDeviceID).percentage
        let underway = try Bool.fetchOne(db, sql: "SELECT EXISTS (SELECT 1 FROM read_throughs WHERE book_id = ? AND outcome = 'underway')",
                                         arguments: [bookID]) ?? false
        let ebooks = try Int.fetchOne(db, sql: """
            SELECT COUNT(*) FROM book_files WHERE book_id = ? AND file_type IN (\(reflowableList)) AND file_role != 'supplement'
            """, arguments: [bookID]) ?? 1
        let name = try String.fetchOne(db, sql: "SELECT display_name FROM book_files WHERE id = ?", arguments: [fileID]) ?? ""
        return ReadProgress(fileID: fileID, percentage: min(1, max(0, percentage)), underway: underway, fileName: name,
                            ebookCount: ebooks)
    }

    // MARK: - Recording

    /// Saves the reader's place in fileID locally and queues it for the
    /// server, with the Reading Session so far, in one transaction: a kill
    /// right after a turn keeps both. Turns of one session replace each
    /// other until sent, but reaching the end is always sent. Each carries
    /// when the session began, so a Read-through it starts begins that day
    /// even when only a later turn is sent.
    func recordReading(fileID: Int, cfi: String, percentage: Double, atEnd: Bool, at: Date, sessionID: String,
                       sessionStartedAt: Date, session: FinishedSession?) throws {
        let payload = Shishoapp.JournalPositionPayload(fileId: fileID, cfi: cfi, percentage: min(1, max(0, percentage)), atEnd: atEnd,
                                                       listeningSince: JournalTime.string(min(sessionStartedAt, at)))
        let data = try JSONEncoder().encode(payload)
        // Reaching the end is never replaced by a later move, which would
        // lose the finish; it replaces only an earlier end in the session.
        let key = "\(atEnd ? "position-end" : "position"):\(fileID):\(sessionID)"
        try pool.write { db in
            try CatalogStore.writePosition(db, fileID: fileID, seconds: 0, cfi: cfi, percentage: percentage, at: at,
                                           payload: data, coalesceKey: key)
            if let session {
                try CatalogStore.appendSession(db, session)
            }
        }
    }

    /// Keeps the place the reader shows as this device's, without sending
    /// it, when the user chose it over another device's, and marks the
    /// server's place seen: the next open stays here rather than going to
    /// the place they turned down, or to an older place of this device's.
    func stayHere(fileID: Int, cfi: String, percentage: Double, at: Date) throws {
        try pool.write { db in
            let serverAt = try Double.fetchOne(db, sql: "SELECT occurred_at FROM reading_positions WHERE file_id = ?", arguments: [fileID])
            try db.execute(sql: """
                INSERT INTO local_positions (file_id, seconds, cfi, percentage, updated_at, base_occurred_at) VALUES (?, 0, ?, ?, ?, ?)
                ON CONFLICT (file_id) DO UPDATE SET cfi = excluded.cfi, percentage = excluded.percentage,
                    updated_at = excluded.updated_at
                """, arguments: [fileID, cfi, percentage, at.timeIntervalSince1970, serverAt])
            try CatalogStore.resolveOffer(db, fileID: fileID)
        }
    }

    /// Queues a Reading Session on its own, as when it ends with no move.
    func recordReading(session: FinishedSession) throws {
        try pool.write { db in
            try CatalogStore.appendSession(db, session)
        }
    }
}
