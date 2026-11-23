import Foundation
import GRDB
import ShishoAPI

/// A moment in an audiobook File the user marked to come back to, with an
/// optional note (see "Bookmark" in CONTEXT.md).
struct Bookmark: Equatable, Identifiable, Hashable, Sendable {
    /// The id this installation made it under, or "server-<id>" for one made
    /// on the web or another installation.
    let id: String
    /// The server's id, once the feed brought it.
    let serverID: Int?
    /// The id an installation made it under, which its journal events name
    /// it by until the server's id is known.
    let localID: String?
    let fileID: Int
    let seconds: Double
    let note: String?
    let createdAt: Date
}

extension CatalogStore {
    // MARK: - Reading

    /// A Bookmark's columns as shown: its note is the one edited here while
    /// that edit waits to be sent, and the server's otherwise.
    private static let shownColumns = """
        b.id, b.server_id, b.local_id, b.file_id, b.seconds, b.created_at,
        CASE WHEN EXISTS (SELECT 1 FROM journal j WHERE j.bookmark_id = b.id AND j.type = 'bookmark_note')
            THEN b.local_note ELSE b.note END AS note
        """

    /// The File's Bookmarks in time order, leaving out those deleted here.
    static func bookmarks(_ db: Database, fileID: Int) throws -> [Bookmark] {
        try Row.fetchAll(db, sql: """
            SELECT \(shownColumns) FROM bookmarks b
            WHERE b.file_id = ? AND NOT b.deleted
            ORDER BY b.seconds, b.created_at
            """, arguments: [fileID]).map(bookmark)
    }

    private static func bookmark(_ row: Row) -> Bookmark {
        Bookmark(id: row["id"], serverID: row["server_id"], localID: row["local_id"], fileID: row["file_id"], seconds: row["seconds"],
                 note: row["note"], createdAt: Date(timeIntervalSince1970: row["created_at"]))
    }

    private static func bookmark(_ db: Database, id: String) throws -> Bookmark? {
        try Row.fetchOne(db, sql: "SELECT \(shownColumns) FROM bookmarks b WHERE b.id = ?", arguments: [id]).map(bookmark)
    }

    /// A note as the server keeps it: trimmed, and none when blank.
    static func bookmarkNote(_ note: String?) -> String? {
        let trimmed = note?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return trimmed.isEmpty ? nil : trimmed
    }

    // MARK: - Recording

    /// Makes a Bookmark at seconds into fileID, and queues it for the server,
    /// in one transaction.
    @discardableResult
    func addBookmark(fileID: Int, seconds: Double, note: String?, at: Date) throws -> Bookmark {
        let id = UUID().uuidString.lowercased()
        let note = CatalogStore.bookmarkNote(note)
        let payload = try JSONEncoder().encode(Shishoapp.JournalBookmarkCreatePayload(localId: id, fileId: fileID, seconds: seconds, note: note))
        return try pool.write { db in
            let generation = try CatalogStore.syncState(db).generation
            try db.execute(sql: """
                INSERT INTO bookmarks (id, local_id, file_id, seconds, note, created_at, generation) VALUES (?, ?, ?, ?, ?, ?, ?)
                """, arguments: [id, id, fileID, seconds, note, at.timeIntervalSince1970, generation])
            try CatalogStore.append(db, type: .bookmarkCreate, at: at, payload: payload, fileID: fileID, coalesceKey: nil, bookmarkID: id)
            return Bookmark(id: id, serverID: nil, localID: id, fileID: fileID, seconds: seconds, note: note, createdAt: at)
        }
    }

    /// Replaces the Bookmark's note, a blank one clearing it, and queues the
    /// edit. A later edit replaces one not yet sent.
    func editBookmarkNote(_ id: String, note: String, at: Date) throws {
        let note = CatalogStore.bookmarkNote(note)
        try pool.write { db in
            guard let bookmark = try CatalogStore.bookmark(db, id: id) else { return }
            var payload = Shishoapp.JournalBookmarkNotePayload(note: note ?? "")
            (payload.bookmarkId, payload.localId) = CatalogStore.reference(bookmark)
            let data = try JSONEncoder().encode(payload)
            try db.execute(sql: "UPDATE bookmarks SET local_note = ? WHERE id = ?", arguments: [note, id])
            let key = "bookmark-note:\(id)"
            try db.execute(sql: "DELETE FROM journal WHERE coalesce_key = ? AND held_for IS NULL", arguments: [key])
            try CatalogStore.append(db, type: .bookmarkNote, at: at, payload: data, fileID: bookmark.fileID, coalesceKey: key, bookmarkID: id)
        }
    }

    /// Deletes the Bookmark: it's hidden at once, and the delete is queued.
    /// Changes not yet sent are dropped, since a delete wins over them. The
    /// delete goes even when the create was still queued, since it may be in
    /// a batch being sent; the server takes a delete of a Bookmark it never
    /// had as done.
    func deleteBookmark(_ id: String, at: Date) throws {
        try pool.write { db in
            guard let bookmark = try CatalogStore.bookmark(db, id: id) else { return }
            try db.execute(sql: "DELETE FROM journal WHERE bookmark_id = ?", arguments: [id])
            var payload = Shishoapp.JournalBookmarkRef()
            (payload.bookmarkId, payload.localId) = CatalogStore.reference(bookmark)
            let data = try JSONEncoder().encode(payload)
            try db.execute(sql: "UPDATE bookmarks SET deleted = 1 WHERE id = ?", arguments: [id])
            try CatalogStore.append(db, type: .bookmarkDelete, at: at, payload: data, fileID: bookmark.fileID, coalesceKey: nil, bookmarkID: id)
        }
    }

    /// How events name a Bookmark: by the id an installation made it under,
    /// which works before the feed brings it and can never name another
    /// Bookmark, or else by the server's id.
    private static func reference(_ bookmark: Bookmark) -> (Int?, String?) {
        if let localID = bookmark.localID {
            return (nil, localID)
        }
        return (bookmark.serverID, nil)
    }

    // MARK: - The feed and the journal

    /// Applies a Bookmark from the feed: the one this installation made under
    /// its local id, or one made elsewhere. A delete waiting to go keeps it
    /// hidden, and a note edited here shows until its edit is sent.
    static func upsert(_ bookmark: Shishoapp.FeedBookmark, generation: Int, in db: Database) throws {
        let createdAt = timestamp(bookmark.createdAt) ?? 0
        // One made in the app is known by its local id. A server id alone
        // matches only a Bookmark made on the web: after a restore from a
        // backup, the server can give an id to another Bookmark.
        let existing = try bookmark.localId.map {
            try String.fetchOne(db, sql: "SELECT id FROM bookmarks WHERE local_id = ?", arguments: [$0])
        } ?? String.fetchOne(db, sql: "SELECT id FROM bookmarks WHERE server_id = ? AND local_id IS NULL", arguments: [bookmark.id])
        // A Bookmark here that had the id before is another one. One made on
        // the web is gone from the server, and its changes waiting here name
        // it by that id, so they would change this one: they go with it. One
        // made in the app keeps its changes, which name it by its own id.
        try db.execute(sql: """
            DELETE FROM journal WHERE bookmark_id IN (
                SELECT id FROM bookmarks WHERE server_id = ? AND id IS NOT ? AND local_id IS NULL)
            """, arguments: [bookmark.id, existing])
        try db.execute(sql: "DELETE FROM bookmarks WHERE server_id = ? AND id IS NOT ? AND local_id IS NULL",
                       arguments: [bookmark.id, existing])
        try db.execute(sql: "UPDATE bookmarks SET server_id = NULL WHERE server_id = ? AND id IS NOT ?", arguments: [bookmark.id, existing])
        guard let id = existing else {
            // The id it was made under, or one from the server's. A row from
            // before a restore can hold that one, so a new one then.
            var id = bookmark.localId ?? "server-\(bookmark.id)"
            if try Bool.fetchOne(db, sql: "SELECT EXISTS (SELECT 1 FROM bookmarks WHERE id = ?)", arguments: [id]) ?? false {
                id = "server-\(bookmark.id)-\(UUID().uuidString.lowercased())"
            }
            try db.execute(sql: """
                INSERT INTO bookmarks (id, server_id, local_id, file_id, seconds, note, created_at, generation) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
                """, arguments: [id, bookmark.id, bookmark.localId, bookmark.fileId, bookmark.seconds, bookmark.note, createdAt, generation])
            return
        }
        try db.execute(sql: """
            UPDATE bookmarks SET server_id = ?, file_id = ?, seconds = ?, note = ?, created_at = ?, generation = ? WHERE id = ?
            """, arguments: [bookmark.id, bookmark.fileId, bookmark.seconds, bookmark.note, createdAt, generation, id])
    }

    /// Drops Bookmarks the server has deleted that the device knows only by
    /// the id it made them under: with the feed caught up, one with nothing
    /// left to send that never came back was deleted before the feed brought
    /// it, and the feed's deletion named only its server id.
    static func dropBookmarksTheServerDeleted(in db: Database) throws {
        try db.execute(sql: """
            DELETE FROM bookmarks WHERE server_id IS NULL
                AND id NOT IN (SELECT bookmark_id FROM journal WHERE bookmark_id IS NOT NULL)
            """)
    }

    /// The server took in a Bookmark event: a deleted Bookmark is gone for
    /// good, whether or not the feed ever brought its server id. A note just
    /// taken in shows until the feed says otherwise; the server sends a
    /// Bookmark again when it kept a later note than the edit. One taken in
    /// before, a duplicate, isn't news: the feed has had the server's note
    /// since, which may be a later edit.
    static func finishBookmarkChange(eventID: String, duplicate: Bool, in db: Database) throws {
        guard let row = try Row.fetchOne(db, sql: "SELECT type, bookmark_id FROM journal WHERE event_id = ?", arguments: [eventID]),
              let id: String = row["bookmark_id"]
        else { return }
        switch Shishoapp.JournalEventType(rawValue: row["type"]) {
        case .bookmarkDelete:
            try db.execute(sql: "DELETE FROM bookmarks WHERE id = ?", arguments: [id])
        case .bookmarkNote where !duplicate:
            try db.execute(sql: "UPDATE bookmarks SET note = local_note WHERE id = ?", arguments: [id])
        default:
            break
        }
    }

    /// Puts back what a refused Bookmark event changed here: a create the
    /// server can't take goes, and a delete it can't take shows the Bookmark
    /// again. A refused note needs nothing: with its edit gone, the server's
    /// note shows.
    static func undoRefusedBookmarkChange(eventID: String, in db: Database) throws {
        guard let row = try Row.fetchOne(db, sql: "SELECT type, bookmark_id FROM journal WHERE event_id = ?", arguments: [eventID]),
              let id: String = row["bookmark_id"]
        else { return }
        switch Shishoapp.JournalEventType(rawValue: row["type"]) {
        case .bookmarkCreate:
            try db.execute(sql: "DELETE FROM journal WHERE bookmark_id = ? AND event_id != ?", arguments: [id, eventID])
            try db.execute(sql: "DELETE FROM bookmarks WHERE id = ?", arguments: [id])
        case .bookmarkDelete:
            try db.execute(sql: "UPDATE bookmarks SET deleted = 0 WHERE id = ?", arguments: [id])
        default:
            break
        }
    }
}
