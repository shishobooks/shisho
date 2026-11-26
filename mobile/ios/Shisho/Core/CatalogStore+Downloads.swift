import Foundation
import GRDB

/// Where a Download stands on the device (ADR 0022). How far one that's
/// running has got is kept in memory by `DownloadManager`, not here.
enum DownloadState: String, Sendable {
    /// Wanted and not on the device yet: running, or waiting for a
    /// connection or for Wi-Fi.
    case queued
    /// On the device, pinned until removed.
    case done
    /// Stopped by something trying again won't fix, such as the File being
    /// gone from the server. Download tries again.
    case failed
}

/// One Download's row.
struct DownloadRow: Equatable, Sendable {
    let fileID: Int
    let fileType: String
    let state: DownloadState
    /// The File's size in the catalog while queued, and the bytes on the
    /// device once done.
    let sizeBytes: Int
    let error: String?
    /// Queued again because the server replaced its File, with the old copy
    /// still on the device, used until the new one arrives.
    var replacing = false
    /// Which copy is in the File's place on the device, new each time one
    /// is put there (`DownloadCopies`).
    var copy: UUID?
}

/// One Download, as the Downloads tab lists it.
struct StoredDownload: Equatable, Identifiable, Sendable {
    var id: Int { fileID }
    let fileID: Int
    let book: BookSummary
    let fileType: String
    /// The File's label among its Book's Files.
    let label: String
    let state: DownloadState
    let sizeBytes: Int
    let error: String?
    /// Keys the File's own cover, which shows its edition; empty when it
    /// has none and shows its Book's.
    var coverVersion = ""
    /// How far through the File the user is, from 0 to 1: this device's
    /// place, or the server's; nil when it hasn't been started.
    var progress: Double?
}

extension CatalogStore {
    /// The Downloads table: one row per File the user downloaded or asked
    /// to. It has no foreign key to `book_files`, whose rows a Book's update
    /// deletes and inserts again; a row whose File left the catalog is
    /// found by `orphanedDownloads` and removed with its file.
    static func createDownloads(_ db: Database) throws {
        try db.create(table: "downloads") { t in
            t.primaryKey("file_id", .integer)
            t.column("file_type", .text).notNull()
            t.column("state", .text).notNull()
            t.column("size_bytes", .integer).notNull()
            // The File's size in the catalog when it was queued. A File
            // replaced on the server shows as a new size there, and
            // downloads again. Kept apart from size_bytes, which becomes the
            // bytes on the device, so a catalog size that never matched the
            // bytes doesn't download it over and over.
            t.column("catalog_size", .integer).notNull().defaults(to: 0)
            t.column("error", .text)
            // Read to its end with Remove when finished on, while the reader
            // still had it open: it goes once the reader lets go of it, or
            // at the next launch.
            t.column("remove_pending", .boolean).notNull().defaults(to: false)
            // Queued again for a replaced File, its old copy still here.
            t.column("replacing", .boolean).notNull().defaults(to: false)
            // Which copy is in the File's place, a new one each time a copy
            // is put there, so an open removes only the copy it downloaded.
            // Random rather than counted, so a Download removed and made
            // again never reuses one.
            t.column("copy_id", .text)
            t.column("requested_at", .double).notNull()
        }
    }

    /// The File Types a Download can be made of: the ones this version
    /// knows (ADR 0020).
    static var downloadableTypes: String {
        FileTypes.known.keys.sorted().map { "'\($0)'" }.joined(separator: ",")
    }

    // MARK: - Reading

    /// Whether fileID is a File in the catalog of a type a Download can be
    /// made of.
    static func isDownloadable(_ db: Database, fileID: Int) throws -> Bool {
        try Bool.fetchOne(db, sql: "SELECT 1 FROM book_files WHERE id = ? AND file_type IN (\(downloadableTypes))",
                          arguments: [fileID]) ?? false
    }

    static func downloads(_ db: Database) throws -> [DownloadRow] {
        try Row.fetchAll(db, sql: """
            SELECT file_id, file_type, state, size_bytes, error, replacing, copy_id FROM downloads ORDER BY requested_at, file_id
            """).map(downloadRow)
    }

    static func download(_ db: Database, fileID: Int) throws -> DownloadRow? {
        try Row.fetchOne(db, sql: """
            SELECT file_id, file_type, state, size_bytes, error, replacing, copy_id FROM downloads WHERE file_id = ?
            """, arguments: [fileID]).map(downloadRow)
    }

    private static func downloadRow(_ row: Row) -> DownloadRow {
        DownloadRow(fileID: row["file_id"], fileType: row["file_type"],
                    state: DownloadState(rawValue: row["state"]) ?? .failed, sizeBytes: row["size_bytes"], error: row["error"],
                    replacing: row["replacing"], copy: (row["copy_id"] as String?).flatMap(UUID.init(uuidString:)))
    }

    /// Finished Downloads whose File was replaced on the server, which the
    /// catalog shows as a size other than the one they were queued at.
    static func replacedDownloads(_ db: Database) throws -> [Int] {
        try Int.fetchAll(db, sql: """
            SELECT d.file_id FROM downloads d JOIN book_files f ON f.id = d.file_id
            WHERE d.state = 'done' AND f.size_bytes != d.catalog_size
            ORDER BY d.file_id
            """)
    }

    /// The Books with a File on the device, which the Library shows as they
    /// are while the server can't be reached, and the rest dimmed.
    static func downloadedBookIDs(_ db: Database) throws -> Set<Int> {
        Set(try Int.fetchAll(db, sql: """
            SELECT DISTINCT f.book_id FROM downloads d JOIN book_files f ON f.id = d.file_id WHERE (d.state = 'done' OR d.replacing)
            """))
    }

    /// How many Downloads are on their way, for the Downloads tab's badge.
    static func downloadingCount(_ db: Database) throws -> Int {
        try Int.fetchOne(db, sql: "SELECT COUNT(*) FROM downloads WHERE state = 'queued'") ?? 0
    }

    /// Downloads whose File isn't in the catalog copy anymore: deleted on
    /// the server, or in a Library the user lost.
    static func orphanedDownloads(_ db: Database) throws -> [Int] {
        try Int.fetchAll(db, sql: "SELECT file_id FROM downloads WHERE file_id NOT IN (SELECT id FROM book_files)")
    }

    /// Every Download with its Book, the newest first, for the Downloads
    /// tab.
    static func storedDownloads(_ db: Database) throws -> [StoredDownload] {
        let rows = try Row.fetchAll(db, sql: """
            SELECT d.file_id, d.file_type, d.state, d.size_bytes, d.error, f.display_name, f.book_id, f.cover_version,
                COALESCE((SELECT percentage FROM local_progress WHERE file_id = d.file_id),
                    (SELECT percentage FROM reading_positions WHERE file_id = d.file_id)) AS progress
            FROM downloads d JOIN book_files f ON f.id = d.file_id
            ORDER BY d.requested_at DESC, d.file_id
            """)
        guard !rows.isEmpty else { return [] }
        let bookIDs = Set(rows.map { $0["book_id"] as Int }).map(String.init).joined(separator: ",")
        let books = try summaries(db, rows: Row.fetchAll(db, sql: "\(summarySQL) WHERE b.id IN (\(bookIDs))"))
        let byID = Dictionary(books.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        return rows.compactMap { row in
            guard let book = byID[row["book_id"]] else { return nil }
            let type: String = row["file_type"]
            let name: String = row["display_name"]
            return StoredDownload(fileID: row["file_id"], book: book, fileType: type,
                                  label: name.isEmpty ? FileTypes.label(type) : name,
                                  state: DownloadState(rawValue: row["state"]) ?? .failed, sizeBytes: row["size_bytes"],
                                  error: row["error"], coverVersion: row["cover_version"], progress: row["progress"])
        }
    }

    /// Every File of a Series' Books, or a List's, that a Download can be
    /// made of, for downloading them all at once: supplements too, since
    /// they're usually small and go with their Book, each Book's main Files
    /// first.
    static func downloadableFiles(_ db: Database, seriesID: Int? = nil, listID: Int? = nil) throws -> [Int] {
        let books: String
        if let seriesID {
            books = "SELECT book_id FROM book_series WHERE series_id = \(seriesID)"
        } else if let listID {
            books = "SELECT book_id FROM list_books WHERE list_id = \(listID)"
        } else {
            return []
        }
        return try Int.fetchAll(db, sql: """
            SELECT id FROM book_files WHERE book_id IN (\(books)) AND file_type IN (\(downloadableTypes))
            ORDER BY book_id, file_role = 'supplement', position
            """)
    }

    /// How many of fileIDs have no Download, or one that failed: what
    /// Download All would start.
    static func notDownloaded(_ db: Database, fileIDs: [Int]) throws -> Int {
        guard !fileIDs.isEmpty else { return 0 }
        let wanted = try Int.fetchOne(db, sql: """
            SELECT COUNT(*) FROM downloads WHERE file_id IN (\(fileIDs.map(String.init).joined(separator: ","))) AND state != 'failed'
            """) ?? 0
        return fileIDs.count - wanted
    }

    // MARK: - Writing

    /// Asks for Downloads of fileIDs, skipping Files of a type this version
    /// doesn't know, and ones already downloaded or on their way. A failed
    /// one is tried again from the start. Returns the Files newly queued.
    @discardableResult
    func queueDownloads(_ fileIDs: [Int], at: Date = Date()) throws -> [Int] {
        guard !fileIDs.isEmpty else { return [] }
        return try pool.write { try CatalogStore.queueDownloads($0, fileIDs, at: at) }
    }

    static func queueDownloads(_ db: Database, _ fileIDs: [Int], at: Date = Date()) throws -> [Int] {
        var queued: [Int] = []
        for (offset, fileID) in fileIDs.enumerated() {
            guard let row = try Row.fetchOne(db, sql: """
                SELECT file_type, size_bytes FROM book_files WHERE id = ? AND file_type IN (\(CatalogStore.downloadableTypes))
                """, arguments: [fileID])
            else { continue }
            let existing = try String.fetchOne(db, sql: "SELECT state FROM downloads WHERE file_id = ?", arguments: [fileID])
            guard existing == nil || existing == DownloadState.failed.rawValue else { continue }
            // In the order asked for, so a Series downloads in order.
            try db.execute(sql: """
                INSERT INTO downloads (file_id, file_type, state, size_bytes, catalog_size, error, requested_at)
                VALUES (?, ?, 'queued', ?, ?, NULL, ?)
                ON CONFLICT (file_id) DO UPDATE SET file_type = excluded.file_type, state = 'queued',
                    size_bytes = excluded.size_bytes, catalog_size = excluded.catalog_size, error = NULL, remove_pending = 0,
                    requested_at = excluded.requested_at
                """, arguments: [fileID, row["file_type"], row["size_bytes"], row["size_bytes"],
                                 at.timeIntervalSince1970 + Double(offset) / 1000])
            queued.append(fileID)
        }
        return queued
    }

    /// Records a finished transfer as a new copy, and returns which copy it
    /// is. Nil when the Download was removed meanwhile, so its file isn't
    /// wanted. `DownloadCopies.publish` calls it as it puts the file there.
    static func markDownloaded(_ db: Database, fileID: Int, sizeBytes: Int) throws -> UUID? {
        let copy = UUID()
        try db.execute(sql: """
            UPDATE downloads SET state = 'done', size_bytes = ?, error = NULL, replacing = 0, copy_id = ?
            WHERE file_id = ? AND state = 'queued'
            """, arguments: [sizeBytes, copy.uuidString, fileID])
        return db.changesCount > 0 ? copy : nil
    }

    func markDownloadFailed(fileID: Int, message: String) throws {
        try pool.write { db in
            try db.execute(sql: "UPDATE downloads SET state = 'failed', error = ? WHERE file_id = ? AND state = 'queued'",
                           arguments: [message, fileID])
        }
    }

    /// Finished Downloads whose files aren't on the device go back in the
    /// queue, as after restoring a backup, which leaves Downloads out.
    func requeueDownloads(_ fileIDs: [Int]) throws {
        guard !fileIDs.isEmpty else { return }
        try pool.write { try CatalogStore.requeueDownloads($0, fileIDs) }
    }

    static func requeueDownloads(_ db: Database, _ fileIDs: [Int]) throws {
        try db.execute(sql: """
            UPDATE downloads SET state = 'queued', error = NULL WHERE file_id IN (\(fileIDs.map(String.init).joined(separator: ",")))
            """)
    }

    /// Forgets the copy in a File's place: a finished Download goes with it,
    /// and one not finished, as a replacement queued or failed, stays as it
    /// is with no copy to use meanwhile.
    static func discardCopy(_ db: Database, fileID: Int, finished: Bool) throws {
        if finished {
            try db.execute(sql: "DELETE FROM downloads WHERE file_id = ?", arguments: [fileID])
        } else {
            try db.execute(sql: "UPDATE downloads SET copy_id = NULL, replacing = 0 WHERE file_id = ?", arguments: [fileID])
        }
    }

    /// Queues every replaced Download again at its File's new size, and
    /// returns them. Each keeps its file until the new one arrives.
    func requeueReplaced() throws -> [Int] {
        try pool.write { db in
            let replaced = try CatalogStore.replacedDownloads(db)
            for fileID in replaced {
                try db.execute(sql: """
                    UPDATE downloads SET state = 'queued', error = NULL, remove_pending = 0, replacing = 1,
                        size_bytes = (SELECT size_bytes FROM book_files WHERE id = downloads.file_id),
                        catalog_size = (SELECT size_bytes FROM book_files WHERE id = downloads.file_id)
                    WHERE file_id = ?
                    """, arguments: [fileID])
            }
            return replaced
        }
    }

    /// Marks a Download to go once the reader lets go of it.
    func markRemovePending(fileID: Int) throws {
        try pool.write { db in
            try db.execute(sql: "UPDATE downloads SET remove_pending = 1 WHERE file_id = ? AND (state = 'done' OR replacing)",
                           arguments: [fileID])
        }
    }

    /// Going back from the end, which undoes an automatic finish, keeps it.
    func clearRemovePending(fileID: Int) throws {
        try pool.write { db in
            try db.execute(sql: "UPDATE downloads SET remove_pending = 0 WHERE file_id = ?", arguments: [fileID])
        }
    }

    /// A supplement never finishes a Read-through, so Remove when finished
    /// leaves it.
    static func isSupplement(_ db: Database, fileID: Int) throws -> Bool {
        try String.fetchOne(db, sql: "SELECT file_role FROM book_files WHERE id = ?", arguments: [fileID]) == "supplement"
    }

    static func removePending(_ db: Database) throws -> [Int] {
        try Int.fetchAll(db, sql: "SELECT file_id FROM downloads WHERE remove_pending")
    }

    func deleteDownloads(_ fileIDs: [Int]) throws {
        guard !fileIDs.isEmpty else { return }
        try pool.write { db in
            try db.execute(sql: "DELETE FROM downloads WHERE file_id IN (\(fileIDs.map(String.init).joined(separator: ",")))")
        }
    }
}
