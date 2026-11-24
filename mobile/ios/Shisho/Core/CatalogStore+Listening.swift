import Foundation
import GRDB
import ShishoAPI

/// An audiobook File as the player needs it, from the device's copy of the
/// catalog.
struct ListeningFile: Equatable, Sendable {
    let fileID: Int
    let book: BookSummary
    /// Keys the File's own cover, which can differ from its Book's; empty
    /// when it has none.
    var coverVersion = ""
    /// The File's label among its Book's Files.
    var displayName = ""
    var narrators: [String] = []
    /// The narrators' Persons, in the same order, so their names link to
    /// their pages.
    var narratorIDs: [Int] = []
    /// The Book's authors, with their Persons, as its page lists them.
    var authors: [BookDetail.Author] = []
    /// The Book's first Series and its place in it, which Now Playing's
    /// header names.
    var series: ListeningSeries?
    /// The real length, which speed doesn't change.
    let duration: Double
    /// Shisho's chapters, top level only, in order (ADR 0022).
    let chapters: [AudioChapter]
}

/// A Book's place in a Series.
struct ListeningSeries: Equatable, Sendable {
    let name: String
    let number: Double?
    var numberEnd: Double?
}

/// How far through a Book's audiobook the user is, for its Book page.
struct ListenProgress: Equatable, Sendable {
    let fileID: Int
    let seconds: Double
    let duration: Double
    let chapters: [AudioChapter]
    /// A Read-through of the Book is underway.
    let underway: Bool
    /// The File's label, which says which one plays when the Book has more
    /// than one audiobook.
    var fileName = ""
    var narrators: [String] = []
    var audiobookCount = 1
    /// How big the File is, said before it's been listened to.
    var sizeBytes = 0

    var fraction: Double { duration > 0 ? min(1, max(0, seconds / duration)) : 0 }
}

/// A change the server refused, for the user to read.
struct JournalNote: Equatable, Identifiable, Sendable {
    let id: Int
    let at: Date
    let eventType: String
    let reason: String
}

/// A journal event waiting to be sent.
struct JournalRow: Equatable, Sendable {
    let seq: Int
    let eventID: String
    let type: String
    let at: Date
    let payload: Data
}

extension CatalogStore {
    // MARK: - Reading

    /// The audiobook File of bookID the Listen button plays: the one
    /// listened to most recently on any device, or else the first, or nil
    /// when the Book has none.
    static func audiobookFileID(_ db: Database, bookID: Int) throws -> Int? {
        try Int.fetchOne(db, sql: """
            SELECT f.id FROM book_files f
            LEFT JOIN local_positions l ON l.file_id = f.id
            LEFT JOIN reading_positions p ON p.file_id = f.id
            WHERE f.book_id = ? AND f.file_type = 'm4b' AND f.file_role != 'supplement'
            ORDER BY MAX(COALESCE(l.updated_at, 0), COALESCE(p.occurred_at, 0)) DESC, f.position
            LIMIT 1
            """, arguments: [bookID])
    }

    static func listeningFile(_ db: Database, fileID: Int) throws -> ListeningFile? {
        guard let row = try Row.fetchOne(db, sql: """
                SELECT book_id, duration_seconds, cover_version, display_name, narrators FROM book_files WHERE id = ?
                """, arguments: [fileID]),
              let book = try CatalogStore.book(db, id: row["book_id"])
        else { return nil }
        return ListeningFile(fileID: fileID, book: book.summary, coverVersion: row["cover_version"], displayName: row["display_name"],
                             // Names and their Persons from one row, so they pair.
                             narrators: book.files.first { $0.id == fileID }?.narrators ?? names(row["narrators"]),
                             narratorIDs: book.files.first { $0.id == fileID }?.narratorIDs ?? [],
                             authors: book.authors,
                             series: book.series.first.map { ListeningSeries(name: $0.name, number: $0.number, numberEnd: $0.numberEnd) },
                             duration: row["duration_seconds"] ?? 0, chapters: try chapters(db, fileID: fileID))
    }

    /// The File's top-level chapters with a start time.
    static func chapters(_ db: Database, fileID: Int) throws -> [AudioChapter] {
        try Row.fetchAll(db, sql: """
            SELECT title, start_ms FROM book_file_chapters
            WHERE file_id = ? AND parent_id IS NULL AND start_ms IS NOT NULL
            ORDER BY position
            """, arguments: [fileID]).map {
            AudioChapter(title: $0["title"], start: Double($0["start_ms"] as Int64) / 1000)
        }
    }

    /// The server's place in an audiobook File, in seconds. A place another
    /// reader left only as a percentage is placed by the File's length.
    static func serverPlace(_ db: Database, fileID: Int) throws -> ServerPlace? {
        guard let row = try Row.fetchOne(db, sql: """
            SELECT p.seconds, p.cfi, p.page, p.percentage, p.occurred_at, p.device_id, p.device_name, p.source, f.duration_seconds
            FROM reading_positions p LEFT JOIN book_files f ON f.id = p.file_id
            WHERE p.file_id = ?
            """, arguments: [fileID])
        else { return nil }
        let duration: Double = row["duration_seconds"] ?? 0
        let percentage: Double = row["percentage"]
        let seconds: Double = row["seconds"] ?? percentage * duration
        return ServerPlace(seconds: seconds, occurredAt: Date(timeIntervalSince1970: row["occurred_at"]),
                           deviceID: row["device_id"], source: row["source"], cfi: row["cfi"], percentage: percentage,
                           page: row["page"], deviceName: row["device_name"])
    }

    static func localPlace(_ db: Database, fileID: Int) throws -> LocalPlace? {
        guard let row = try Row.fetchOne(db, sql: """
            SELECT seconds, cfi, page, percentage, updated_at, base_occurred_at, offer_seconds, offer_cfi, offer_page, offer_percentage,
                offer_occurred_at, offer_source, offer_device_id, offer_device_name
            FROM local_positions WHERE file_id = ?
            """, arguments: [fileID])
        else { return nil }
        var offer: ServerPlace?
        if let seconds: Double = row["offer_seconds"], let at: Double = row["offer_occurred_at"] {
            offer = ServerPlace(seconds: seconds, occurredAt: Date(timeIntervalSince1970: at),
                                deviceID: row["offer_device_id"], source: row["offer_source"] ?? "",
                                cfi: row["offer_cfi"], percentage: row["offer_percentage"] ?? 0,
                                page: row["offer_page"], deviceName: row["offer_device_name"])
        }
        return LocalPlace(seconds: row["seconds"], updatedAt: Date(timeIntervalSince1970: row["updated_at"]),
                          pending: try hasPendingMoves(db, fileID: fileID),
                          base: (row["base_occurred_at"] as Double?).map(Date.init(timeIntervalSince1970:)), offer: offer,
                          cfi: row["cfi"], percentage: row["percentage"], page: row["page"])
    }

    /// Keeps another device's place as an offer when it arrives while this
    /// device has moves of its own to send, since sending them may replace
    /// it on the server. A newer one replaces an offer kept already, and the
    /// same move sent again, as when its Device is renamed or deleted,
    /// brings the Device's name up to date.
    static func keepOffer(_ position: Shishoapp.FeedReadingPosition, ownDeviceID: String, in db: Database) throws {
        guard !ownDeviceID.isEmpty, position.deviceId != ownDeviceID,
              let occurredAt = timestamp(position.occurredAt),
              let place = try serverPlace(db, fileID: position.fileId)
        else { return }
        let offered = try Bool.fetchOne(db, sql: "SELECT offer_seconds IS NOT NULL FROM local_positions WHERE file_id = ?",
                                        arguments: [position.fileId]) ?? false
        guard try offered || hasPendingMoves(db, fileID: position.fileId) else { return }
        try db.execute(sql: """
            UPDATE local_positions SET offer_seconds = ?, offer_cfi = ?, offer_page = ?, offer_percentage = ?, offer_occurred_at = ?,
                offer_source = ?, offer_device_id = ?, offer_device_name = ?
            WHERE file_id = ? AND ? > COALESCE(base_occurred_at, 0) AND ? > COALESCE(offer_occurred_at, 0)
            """, arguments: [place.seconds, place.cfi, place.page, place.percentage, occurredAt, position.source.rawValue,
                             position.deviceId, position.deviceName, position.fileId, occurredAt, occurredAt])
        // The same move by ServerPlace.isSameMove: another Device's move at
        // the same moment is a different one.
        try db.execute(sql: """
            UPDATE local_positions SET offer_device_id = ?, offer_device_name = ?
            WHERE file_id = ? AND offer_occurred_at = ? AND offer_source = ? AND offer_seconds = ?
                AND offer_cfi IS ? AND offer_page IS ? AND offer_percentage = ?
            """, arguments: [position.deviceId, position.deviceName, position.fileId, occurredAt, position.source.rawValue,
                             place.seconds, place.cfi, place.page, place.percentage])
    }

    /// The user went to the offered place, or chose to stay. Either way the
    /// server's place now is one they've seen, so it isn't offered or gone
    /// to again until another device moves.
    func resolveOffer(fileID: Int) throws {
        try pool.write { db in
            try CatalogStore.resolveOffer(db, fileID: fileID)
        }
    }

    static func resolveOffer(_ db: Database, fileID: Int) throws {
        try db.execute(sql: """
            UPDATE local_positions SET offer_seconds = NULL, offer_cfi = NULL, offer_page = NULL, offer_percentage = NULL,
                offer_occurred_at = NULL,
                offer_source = NULL, offer_device_id = NULL, offer_device_name = NULL,
                base_occurred_at = MAX(COALESCE(base_occurred_at, 0),
                    COALESCE((SELECT occurred_at FROM reading_positions WHERE file_id = ?), 0))
            WHERE file_id = ?
            """, arguments: [fileID, fileID])
    }

    static func hasPendingMoves(_ db: Database, fileID: Int) throws -> Bool {
        try Bool.fetchOne(db, sql: "SELECT EXISTS (SELECT 1 FROM journal WHERE type = ? AND file_id = ? AND held_for IS NULL)",
                          arguments: [Shishoapp.JournalEventType.readingPosition.rawValue, fileID]) ?? false
    }

    /// The Book's audiobook progress, or nil when it has no audiobook. With
    /// fileID, the progress of that audiobook File, for its own page, which
    /// needn't say which File it is.
    static func listenProgress(_ db: Database, bookID: Int, fileID asked: Int? = nil, ownDeviceID: String) throws -> ListenProgress? {
        guard let fileID = try asked ?? audiobookFileID(db, bookID: bookID),
              let file = try listeningFile(db, fileID: fileID)
        else { return nil }
        let choice = PlaceRules.open(local: try localPlace(db, fileID: fileID), server: try serverPlace(db, fileID: fileID),
                                     ownDeviceID: ownDeviceID)
        let underway = try Bool.fetchOne(db, sql: "SELECT EXISTS (SELECT 1 FROM read_throughs WHERE book_id = ? AND outcome = 'underway')",
                                         arguments: [bookID]) ?? false
        let audiobooks = try Int.fetchOne(db, sql: """
            SELECT COUNT(*) FROM book_files WHERE book_id = ? AND file_type = 'm4b' AND file_role != 'supplement'
            """, arguments: [bookID]) ?? 1
        let size = try Int.fetchOne(db, sql: "SELECT size_bytes FROM book_files WHERE id = ?", arguments: [fileID]) ?? 0
        return ListenProgress(fileID: fileID, seconds: choice.start, duration: file.duration, chapters: file.chapters, underway: underway,
                              fileName: file.displayName, narrators: file.narrators, audiobookCount: asked == nil ? audiobooks : 1,
                              sizeBytes: size)
    }

    /// Titles for the Books and Files a diagnostics log names by id, keyed as
    /// the log names them ("book 3", "file 12").
    static func titles(_ db: Database, books: Set<Int>, files: Set<Int>) throws -> [String: String] {
        var titles: [String: String] = [:]
        for id in books {
            if let title = try String.fetchOne(db, sql: "SELECT title FROM books WHERE id = ?", arguments: [id]) {
                titles["book \(id)"] = title
            }
        }
        for id in files {
            if let row = try Row.fetchOne(db, sql: """
                SELECT b.title, f.display_name FROM book_files f JOIN books b ON b.id = f.book_id WHERE f.id = ?
                """, arguments: [id]) {
                let title: String = row["title"]
                let name: String = row["display_name"]
                titles["file \(id)"] = name.isEmpty ? title : "\(title) (\(name))"
            }
        }
        return titles
    }

    static func journalNotes(_ db: Database) throws -> [JournalNote] {
        try Row.fetchAll(db, sql: "SELECT id, at, event_type, reason FROM journal_notes ORDER BY id").map {
            JournalNote(id: $0["id"], at: Date(timeIntervalSince1970: $0["at"]), eventType: $0["event_type"], reason: $0["reason"])
        }
    }

    /// How many changes wait to be sent.
    static func pendingCount(_ db: Database) throws -> Int {
        try Int.fetchOne(db, sql: "SELECT COUNT(*) FROM journal") ?? 0
    }

    // MARK: - Recording

    /// Saves the user's place in fileID locally and queues it for the server,
    /// in one transaction. Moves of one sitting replace each other until
    /// sent, but reaching the end is always sent.
    func recordPosition(fileID: Int, seconds: Double, percentage: Double, at: Date, atEnd: Bool,
                        sitting: ListeningSitting?) throws {
        var payload = Shishoapp.JournalPositionPayload(fileId: fileID, seconds: seconds, percentage: min(1, max(0, percentage)),
                                                       atEnd: atEnd)
        if let sitting, sitting.fileID == fileID {
            payload.listenedSeconds = sitting.played(at: at)
            payload.listeningSince = JournalTime.string(sitting.startedAt)
        }
        let data = try JSONEncoder().encode(payload)
        let key = atEnd ? nil : "position:\(fileID):\(sitting.map { String($0.startedAt.timeIntervalSince1970) } ?? "idle")"
        try pool.write { db in
            try CatalogStore.writePosition(db, fileID: fileID, seconds: seconds, cfi: nil, percentage: percentage, at: at,
                                           payload: data, coalesceKey: key)
        }
    }

    /// Saves a place locally and queues its event, in the caller's
    /// transaction. coalesceKey names moves that replace each other until
    /// sent; nil is always sent.
    static func writePosition(_ db: Database, fileID: Int, seconds: Double, cfi: String?, page: Int? = nil, percentage: Double,
                              at: Date, payload: Data, coalesceKey key: String?) throws {
        let pending = try hasPendingMoves(db, fileID: fileID)
        let serverAt = try Double.fetchOne(db, sql: "SELECT occurred_at FROM reading_positions WHERE file_id = ?", arguments: [fileID])
        try db.execute(sql: """
            INSERT INTO local_positions (file_id, seconds, cfi, page, percentage, updated_at, base_occurred_at)
            VALUES (?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT (file_id) DO UPDATE SET seconds = excluded.seconds, cfi = excluded.cfi, page = excluded.page,
                percentage = excluded.percentage, updated_at = excluded.updated_at,
                base_occurred_at = CASE WHEN ? THEN local_positions.base_occurred_at ELSE excluded.base_occurred_at END
            """, arguments: [fileID, seconds, cfi, page, percentage, at.timeIntervalSince1970, serverAt, pending])
        if let key {
            try db.execute(sql: "DELETE FROM journal WHERE coalesce_key = ? AND held_for IS NULL", arguments: [key])
        }
        try append(db, type: .readingPosition, at: at, payload: payload, fileID: fileID, coalesceKey: key)
    }

    /// Keeps the sitting being timed, or forgets it with nil, and queues a
    /// snapshot of a sitting's Reading Session, in one transaction: a kill in
    /// between can't lose the session or record it apart from its sitting.
    func saveSitting(_ sitting: ListeningSitting?, recording session: FinishedSession? = nil) throws {
        let kept = try sitting.map { try JSONEncoder().encode($0) }
        try pool.write { db in
            if let session {
                try CatalogStore.appendSession(db, session)
            }
            if let kept {
                try db.execute(sql: "INSERT INTO open_sitting (id, sitting) VALUES (1, ?) ON CONFLICT (id) DO UPDATE SET sitting = excluded.sitting",
                               arguments: [kept])
            } else {
                try db.execute(sql: "DELETE FROM open_sitting")
            }
        }
    }

    /// Queues a snapshot of a Reading Session, in the caller's transaction.
    /// A later snapshot of a session replaces one not yet sent; the server
    /// keeps the latest anyway.
    static func appendSession(_ db: Database, _ session: FinishedSession) throws {
        let data = try JSONEncoder().encode(Shishoapp.JournalSessionPayload(
            sessionId: session.sittingID, fileId: session.fileID, startedAt: JournalTime.string(session.startedAt),
            endedAt: JournalTime.string(session.endedAt), startPercentage: min(1, max(0, session.startPercentage)),
            endPercentage: min(1, max(0, session.endPercentage)), duration: session.duration
        ))
        let key = session.sittingID.isEmpty ? nil : "session:\(session.sittingID)"
        if let key {
            try db.execute(sql: "DELETE FROM journal WHERE coalesce_key = ? AND held_for IS NULL", arguments: [key])
        }
        try append(db, type: .readingSession, at: session.endedAt, payload: data, fileID: session.fileID, coalesceKey: key)
    }

    static func append(_ db: Database, type: Shishoapp.JournalEventType, at: Date, payload: Data, fileID: Int?,
                       coalesceKey: String?, bookmarkID: String? = nil) throws {
        try db.execute(sql: """
            INSERT INTO journal (event_id, type, at, payload, file_id, coalesce_key, bookmark_id) VALUES (?, ?, ?, ?, ?, ?, ?)
            """, arguments: [UUID().uuidString.lowercased(), type.rawValue, at.timeIntervalSince1970, payload, fileID, coalesceKey,
                             bookmarkID])
    }

    /// The sitting being timed, kept across launches.
    func openSitting() throws -> ListeningSitting? {
        try pool.read { db in
            guard let data = try Data.fetchOne(db, sql: "SELECT sitting FROM open_sitting WHERE id = 1") else { return nil }
            return try? JSONDecoder().decode(ListeningSitting.self, from: data)
        }
    }


    // MARK: - The journal

    /// The oldest events to send, leaving out those a server didn't
    /// understand while it listed capabilities, until the list changes.
    func pendingJournal(capabilities: String, limit: Int) throws -> [JournalRow] {
        try pool.read { db in
            try Row.fetchAll(db, sql: """
                SELECT seq, event_id, type, at, payload FROM journal
                WHERE held_for IS NULL OR held_for != ?
                ORDER BY seq LIMIT ?
                """, arguments: [capabilities, limit]).map {
                JournalRow(seq: $0["seq"], eventID: $0["event_id"], type: $0["type"],
                           at: Date(timeIntervalSince1970: $0["at"]), payload: $0["payload"])
            }
        }
    }

    /// Applies the server's answer to a batch: taken-in and duplicate events
    /// are done, not-understood ones wait for the capability list to change,
    /// and refused ones are dropped with a note for the user.
    func applyJournalResults(_ results: [Shishoapp.JournalResult], sent: [JournalRow], capabilities: String) throws {
        let types = Dictionary(sent.map { ($0.eventID, $0.type) }, uniquingKeysWith: { first, _ in first })
        try pool.write { db in
            for result in results where types[result.id] != nil {
                switch result.result {
                case .accepted, .duplicate:
                    try CatalogStore.finishBookmarkChange(eventID: result.id, duplicate: result.result == .duplicate, in: db)
                    // The feed brings the Book's flow as the server kept it.
                    try db.execute(sql: "DELETE FROM chosen_page_flows WHERE event_id = ?", arguments: [result.id])
                    try db.execute(sql: "DELETE FROM journal WHERE event_id = ?", arguments: [result.id])
                case .refused:
                    try CatalogStore.undoRefusedBookmarkChange(eventID: result.id, in: db)
                    try db.execute(sql: "DELETE FROM chosen_page_flows WHERE event_id = ?", arguments: [result.id])
                    try db.execute(sql: "DELETE FROM journal WHERE event_id = ?", arguments: [result.id])
                    try db.execute(sql: "INSERT INTO journal_notes (at, event_type, reason) VALUES (?, ?, ?)",
                                   arguments: [Date().timeIntervalSince1970, types[result.id] ?? "", result.reason])
                case .notUnderstood, .unknown:
                    try db.execute(sql: "UPDATE journal SET held_for = ?, held_reason = ? WHERE event_id = ?",
                                   arguments: [capabilities, result.reason, result.id])
                }
            }
        }
    }

    func dismissJournalNotes() throws {
        try pool.write { db in
            try db.execute(sql: "DELETE FROM journal_notes")
        }
    }
}

/// Times as the journal sends them: RFC 3339 with milliseconds.
enum JournalTime {
    static func string(_ date: Date) -> String {
        date.formatted(Date.ISO8601FormatStyle(includingFractionalSeconds: true))
    }
}
