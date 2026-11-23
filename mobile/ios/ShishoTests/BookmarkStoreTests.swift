import GRDB
import ShishoAPI
import XCTest
@testable import Shisho

/// Bookmarks in one account's database: made, edited, and deleted offline
/// through the journal (ADR 0024), and brought in by the feed, including
/// those made on the web.
final class BookmarkStoreTests: XCTestCase {
    private var store: CatalogStore!
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    override func setUpWithError() throws {
        store = try CatalogStore(path: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path)
        try store.startOverIfNeeded()
        try store.apply(page("""
        {"entries":[{"kind":"library","id":1,"library_id":1,"library":{"id":1,"name":"Audio","cover_aspect_ratio":"audiobook"}},
         {"kind":"book","id":1,"library_id":1,"book":{"id":1,"library_id":1,"title":"Harbor Lights","sort_title":"Harbor Lights",
          "created_at":"2026-01-01T00:00:00Z","cover_version":"1-1","files":[{"id":5,"file_type":"m4b","file_role":"main","size_bytes":10,
          "audiobook_duration_seconds":600,"chapters":[]}]}}],
         "cursor":2}
        """))
    }

    private func page(_ json: String) throws -> Shishoapp.FeedResponse {
        try makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data(json.utf8))
    }

    private func bookmarks() throws -> [Bookmark] {
        try store.pool.read { try CatalogStore.bookmarks($0, fileID: 5) }
    }

    /// The journal's events, as (type, payload).
    private func journal() throws -> [(String, [String: Any])] {
        try store.pool.read { try Row.fetchAll($0, sql: "SELECT event_id, type, payload FROM journal ORDER BY seq") }.map { row in
            let payload = try XCTUnwrap(JSONSerialization.jsonObject(with: row["payload"] as Data) as? [String: Any])
            return (row["type"], payload)
        }
    }

    private func journalRows() throws -> [JournalRow] {
        try store.pendingJournal(capabilities: "", limit: 100)
    }

    private func accept(_ rows: [JournalRow]) throws {
        try store.applyJournalResults(rows.map { Shishoapp.JournalResult(id: $0.eventID, result: .accepted) }, sent: rows, capabilities: "")
    }

    private func feedBookmark(_ id: Int, seconds: Double = 83.5, note: String? = nil, localID: String? = nil, deleted: Bool = false,
                              cursor: Int) throws -> Shishoapp.FeedResponse {
        if deleted {
            return try page(#"{"entries":[{"kind":"bookmark","id":\#(id),"deleted":true}],"cursor":\#(cursor)}"#)
        }
        let noteJSON = note.map { "\"\($0)\"" } ?? "null"
        let localJSON = localID.map { "\"\($0)\"" } ?? "null"
        return try page("""
        {"entries":[{"kind":"bookmark","id":\(id),"bookmark":{"id":\(id),"file_id":5,"seconds":\(seconds),"note":\(noteJSON),
          "created_at":"2026-05-01T10:00:00Z","local_id":\(localJSON)}}],"cursor":\(cursor)}
        """)
    }

    /// A Bookmark made on the device shows at once, goes out as a create, and
    /// is the same one when the feed brings it back with the server's id.
    func testABookmarkMadeHereIsTheSameOneWhenTheFeedBringsItBack() throws {
        let made = try store.addBookmark(fileID: 5, seconds: 83.5, note: "  the twist  ", at: now)
        XCTAssertEqual(try bookmarks().map(\.note), ["the twist"])
        XCTAssertNil(made.serverID)
        let events = try journal()
        XCTAssertEqual(events.map(\.0), ["bookmark_create"])
        XCTAssertEqual(events[0].1["local_id"] as? String, made.id)
        XCTAssertEqual(events[0].1["seconds"] as? Double, 83.5)
        XCTAssertEqual(events[0].1["note"] as? String, "the twist")

        try accept(journalRows())
        try store.apply(feedBookmark(42, note: "the twist", localID: made.id, cursor: 3))
        let listed = try bookmarks()
        XCTAssertEqual(listed.count, 1)
        XCTAssertEqual(listed[0].id, made.id)
        XCTAssertEqual(listed[0].serverID, 42)

        // It's still named by the id it was made under, which can't name
        // another Bookmark, even after the server is restored from a backup.
        try store.editBookmarkNote(made.id, note: "the real twist", at: now)
        let edit = try journal()
        XCTAssertEqual(edit.map(\.0), ["bookmark_note"])
        XCTAssertEqual(edit[0].1["local_id"] as? String, made.id)
        XCTAssertNil(edit[0].1["bookmark_id"])
        XCTAssertEqual(try bookmarks().map(\.note), ["the real twist"])
    }

    /// One made on the web arrives in the feed, and is edited and deleted by
    /// its server id. A delete hides it at once.
    func testABookmarkFromTheWebIsEditedAndDeletedByItsServerID() throws {
        try store.apply(feedBookmark(43, seconds: 12, note: "web", cursor: 3))
        XCTAssertEqual(try bookmarks().map(\.seconds), [12])

        let id = try XCTUnwrap(bookmarks().first?.id)
        try store.editBookmarkNote(id, note: "", at: now)
        XCTAssertEqual(try bookmarks().map(\.note), [nil])
        try store.deleteBookmark(id, at: now)
        XCTAssertEqual(try bookmarks(), [])
        // The note edit is moot once deleted.
        let events = try journal()
        XCTAssertEqual(events.map(\.0), ["bookmark_delete"])
        XCTAssertEqual(events[0].1["bookmark_id"] as? Int, 43)

        // The feed bringing it again before the delete is sent doesn't bring
        // it back; the feed's delete removes it for good.
        try store.apply(feedBookmark(43, seconds: 12, note: "web", cursor: 4))
        XCTAssertEqual(try bookmarks(), [])
        try accept(journalRows())
        try store.apply(feedBookmark(43, deleted: true, cursor: 5))
        XCTAssertEqual(try store.pool.read { try Int.fetchOne($0, sql: "SELECT COUNT(*) FROM bookmarks") }, 0)
    }

    /// Deleting one whose create hasn't been sent drops the create, but still
    /// sends a delete by the id it was made under: the create may be on its
    /// way already, in a batch being sent.
    func testDeletingABookmarkNotYetSentStillSendsADelete() throws {
        let made = try store.addBookmark(fileID: 5, seconds: 10, note: nil, at: now)
        let inFlight = try journalRows()
        try store.editBookmarkNote(made.id, note: "note", at: now)
        try store.deleteBookmark(made.id, at: now)
        XCTAssertEqual(try bookmarks(), [])
        let events = try journal()
        XCTAssertEqual(events.map(\.0), ["bookmark_delete"])
        XCTAssertEqual(events[0].1["local_id"] as? String, made.id)

        // The create in flight lands, and the feed brings it before the
        // delete is sent: it stays hidden.
        try accept(inFlight)
        try store.apply(feedBookmark(42, seconds: 10, localID: made.id, cursor: 3))
        XCTAssertEqual(try bookmarks(), [])
        // Once the delete is taken in, the Bookmark is gone for good.
        try accept(journalRows())
        XCTAssertEqual(try store.pool.read { try Int.fetchOne($0, sql: "SELECT COUNT(*) FROM bookmarks") }, 0)
    }

    /// A Bookmark deleted on the web before the feed brought it here arrives
    /// only as a deletion of a server id the device never knew. Once the
    /// feed is caught up, one the server should have sent and didn't is gone.
    func testABookmarkDeletedElsewhereBeforeTheFeedBroughtItGoes() throws {
        _ = try store.addBookmark(fileID: 5, seconds: 10, note: nil, at: now)
        try store.apply(page(#"{"entries":[],"cursor":3,"has_more":false}"#))
        XCTAssertEqual(try bookmarks().count, 1, "still waiting to be sent")
        try accept(journalRows())
        try store.apply(feedBookmark(42, deleted: true, cursor: 4))
        XCTAssertEqual(try bookmarks(), [])
    }

    /// A note edited here and not yet sent stays over the feed's older one.
    func testAnUnsentNoteStaysOverTheFeeds() throws {
        try store.apply(feedBookmark(43, note: "web", cursor: 3))
        let id = try XCTUnwrap(bookmarks().first?.id)
        try store.editBookmarkNote(id, note: "first", at: now)
        try store.editBookmarkNote(id, note: "mine", at: now)
        XCTAssertEqual(try journal().map(\.0), ["bookmark_note"], "a later edit replaces an unsent one")
        try store.apply(feedBookmark(43, note: "web", cursor: 4))
        XCTAssertEqual(try bookmarks().map(\.note), ["mine"])

        try accept(journalRows())
        try store.apply(feedBookmark(43, note: "newer on the web", cursor: 5))
        XCTAssertEqual(try bookmarks().map(\.note), ["newer on the web"])
    }

    /// A note edited here, then later on the web: the feed brings the web's
    /// before the edit is sent, and the server keeps the web's. Once the edit
    /// is taken in, the device shows the server's note, with nothing more
    /// from the feed.
    func testAnOlderEditTakenInShowsTheServersNote() throws {
        try store.apply(feedBookmark(43, note: "web", cursor: 3))
        let id = try XCTUnwrap(bookmarks().first?.id)
        try store.editBookmarkNote(id, note: "offline", at: now)
        try store.apply(feedBookmark(43, note: "later on the web", cursor: 4))
        XCTAssertEqual(try bookmarks().map(\.note), ["offline"])
        // Taken in, the edit shows until the feed says otherwise: the server
        // sends the Bookmark again when it kept a later note.
        try accept(journalRows())
        XCTAssertEqual(try bookmarks().map(\.note), ["offline"])
        try store.apply(feedBookmark(43, note: "later on the web", cursor: 5))
        XCTAssertEqual(try bookmarks().map(\.note), ["later on the web"])
    }

    /// After a server restore, a Bookmark made on the web can come with the
    /// id one made here had. It's a different Bookmark, not this one.
    func testAServerIDReusedAfterARestoreIsAnotherBookmark() throws {
        let made = try store.addBookmark(fileID: 5, seconds: 10, note: "mine", at: now)
        try accept(journalRows())
        try store.apply(feedBookmark(42, seconds: 10, note: "mine", localID: made.id, cursor: 3))
        try store.deleteBookmark(made.id, at: now)
        try store.startOverIfNeeded(serverSaidSo: true)
        try store.apply(feedBookmark(42, seconds: 99, note: "theirs", cursor: 5))
        XCTAssertEqual(try bookmarks().map(\.note), ["theirs"])
        let events = try journal()
        XCTAssertEqual(events.map(\.0), ["bookmark_delete"])
        XCTAssertEqual(events[0].1["local_id"] as? String, made.id)
    }

    /// A create the server refused, such as for a File since deleted, goes.
    func testARefusedCreateDropsTheBookmark() throws {
        _ = try store.addBookmark(fileID: 5, seconds: 10, note: nil, at: now)
        let rows = try journalRows()
        try store.applyJournalResults(rows.map { Shishoapp.JournalResult(id: $0.eventID, result: .refused, reason: "gone") },
                                      sent: rows, capabilities: "")
        XCTAssertEqual(try bookmarks(), [])
    }

    /// Downloading the catalog again drops Bookmarks the server no longer
    /// has, but keeps those with changes still to send.
    func testStartingOverKeepsOnlyWhatTheServerHasOrIsPending() throws {
        try store.apply(feedBookmark(43, cursor: 3))
        try store.apply(feedBookmark(44, seconds: 20, cursor: 4))
        let pending = try store.addBookmark(fileID: 5, seconds: 30, note: nil, at: now)
        // An edit waiting to go doesn't keep one the server no longer has.
        try store.editBookmarkNote("server-43", note: "edited", at: now)
        try store.startOverIfNeeded(serverSaidSo: true)
        try store.apply(feedBookmark(44, seconds: 20, cursor: 6))
        XCTAssertEqual(try bookmarks().map(\.seconds), [20, 30])
        XCTAssertEqual(try bookmarks().last?.id, pending.id)
    }
}
