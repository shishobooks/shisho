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
    /// The 0-based page it starts on in a Page-based File.
    var startPage: Int?
}

/// An ebook File as the reader needs it, from the device's copy of the
/// catalog: a Reflowable one, or a Page-based one (CBZ or PDF).
struct ReadingFile: Equatable, Identifiable, Sendable {
    var id: Int { fileID }
    let fileID: Int
    let book: BookSummary
    let fileType: String
    let displayName: String
    let sizeBytes: Int
    /// Shisho's chapters, nested under their parents, in order (ADR 0022).
    let chapters: [ReaderChapter]
    /// The File's language as Shisho normalized it, which a book's own
    /// metadata may give in a form the reader can't use.
    var language: String?
    /// A CBZ's pages, as the names of its archive entries in the order
    /// Shisho counts them (ADR 0025): the reader never sorts them itself.
    var pages: [String] = []
    /// The File's Reading Direction, "rtl" or "ltr", or nil for left to
    /// right.
    var readingDirection: String?

    /// Read in the image pager (ADR 0025) rather than with foliate.
    var isPageBased: Bool { CatalogStore.pageBasedTypes.contains(fileType) }

    /// The File runs right to left, as manga does.
    var rightToLeft: Bool { readingDirection == "rtl" }

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
    /// The chapter and pages left in the book the reader last showed here,
    /// while that's still the place shown.
    var chapter: String?
    var pagesLeft: Int?
    /// How long and how big the File is, said before it's been read.
    var wordCount: Int?
    var sizeBytes = 0
    /// A Page-based File's pages, said before it's been read, and what
    /// pages left counts from.
    var pageCount: Int?

    /// What the Read card says before the book's been read: how long it is,
    /// in pages for a comic or PDF and in words for an ebook, and how big,
    /// as the Listen card says it of an audiobook.
    var unstartedFacts: String? {
        let length = pageCount.map { [FileFacts.pages($0)] }
            ?? ((wordCount ?? 0) > 0 ? [FileFacts.words(wordCount ?? 0)] : [])
        let size = sizeBytes > 0 ? [FileFacts.size(sizeBytes)] : []
        let facts = length + size
        return facts.isEmpty ? nil : facts.joined(separator: " · ")
    }
}

extension CatalogStore {
    /// What the reader keeps for a File's Book page, and the Page flow the
    /// user chose for a Book here.
    static func createReaderState(_ db: Database) throws {
        // The chapter and pages left the reader last showed in a File, for
        // its Book's page, which can't work them out without the book.
        try db.create(table: "reader_places") { t in
            t.primaryKey("file_id", .integer)
            t.column("chapter", .text)
            t.column("pages_left", .integer)
            t.column("percentage", .double).notNull()
        }
        // The Page flow the user chose for a Book comes with their record
        // of it, and one chosen here shows until the feed brings it back.
        try db.create(table: "chosen_page_flows") { t in
            t.primaryKey("book_id", .integer)
            t.column("page_flow", .text).notNull()
            // The journal event sending it, or nil for a choice kept on
            // the device because the server can't take it.
            t.column("event_id", .text)
            // The server took it in: it goes once the feed brings the
            // Book's record back, so the choice never flickers back to
            // the old one in between.
            t.column("accepted", .boolean).notNull().defaults(to: false)
        }
    }

    /// The File Types the reader lays out with foliate (ADR 0026).
    static let reflowableTypes: Set<String> = ["epub", "kepub", "mobi", "azw3"]
    /// The File Types the reader shows in the image pager (ADR 0025).
    static let pageBasedTypes: Set<String> = ["cbz", "pdf"]
    /// Every File Type the reader opens.
    static var readableTypes: Set<String> { reflowableTypes.union(pageBasedTypes) }
    /// The Reflowable File Types whose Shisho chapters the reader can go to.
    static let navigableChapterTypes: Set<String> = ["epub", "kepub"]

    private static var readableList: String {
        readableTypes.sorted().map { "'\($0)'" }.joined(separator: ", ")
    }

    // MARK: - Reading

    static func readingFile(_ db: Database, fileID: Int) throws -> ReadingFile? {
        guard let row = try Row.fetchOne(db, sql: """
                SELECT book_id, file_type, display_name, size_bytes, language, pages, reading_direction
                FROM book_files WHERE id = ? AND file_type IN (\(readableList))
                """, arguments: [fileID]),
              let book = try CatalogStore.book(db, id: row["book_id"])
        else { return nil }
        return ReadingFile(fileID: fileID, book: book.summary, fileType: row["file_type"], displayName: row["display_name"],
                           sizeBytes: row["size_bytes"], chapters: try readerChapters(db, fileID: fileID), language: row["language"],
                           pages: names(row["pages"]), readingDirection: row["reading_direction"])
    }

    /// The File's chapters, each nested one deeper than its parent. The feed
    /// sends each after its parent.
    static func readerChapters(_ db: Database, fileID: Int) throws -> [ReaderChapter] {
        var depths: [Int: Int] = [:]
        return try Row.fetchAll(db, sql: """
            SELECT chapter_id, parent_id, title, href, start_page FROM book_file_chapters WHERE file_id = ? ORDER BY position
            """, arguments: [fileID]).map { row in
            let parent: Int? = row["parent_id"]
            let depth = parent.flatMap { depths[$0].map { $0 + 1 } } ?? 0
            depths[row["chapter_id"]] = depth
            let href: String? = row["href"]
            return ReaderChapter(title: row["title"], href: href?.isEmpty == true ? nil : href, depth: depth, startPage: row["start_page"])
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
            WHERE f.book_id = ? AND f.file_type IN (\(readableList)) AND f.file_role != 'supplement'
            ORDER BY MAX(COALESCE(l.updated_at, 0), COALESCE(p.occurred_at, 0)) DESC, f.position
            LIMIT 1
            """, arguments: [bookID])
    }

    /// The Book's ebook progress, or nil when it has no ebook.
    static func readProgress(_ db: Database, bookID: Int, ownDeviceID: String) throws -> ReadProgress? {
        guard let fileID = try ebookFileID(db, bookID: bookID) else { return nil }
        let local = try localPlace(db, fileID: fileID)
        let server = try serverPlace(db, fileID: fileID)
        let choice = PlaceRules.openEbook(local: local, server: server, ownDeviceID: ownDeviceID)
        let percentage = choice.percentage
        let underway = try Bool.fetchOne(db, sql: "SELECT EXISTS (SELECT 1 FROM read_throughs WHERE book_id = ? AND outcome = 'underway')",
                                         arguments: [bookID]) ?? false
        let ebooks = try Int.fetchOne(db, sql: """
            SELECT COUNT(*) FROM book_files WHERE book_id = ? AND file_type IN (\(readableList)) AND file_role != 'supplement'
            """, arguments: [bookID]) ?? 1
        let file = try Row.fetchOne(db, sql: "SELECT display_name, word_count, size_bytes, file_type, page_count, pages FROM book_files WHERE id = ?",
                                    arguments: [fileID])
        var progress = ReadProgress(fileID: fileID, percentage: min(1, max(0, percentage)), underway: underway,
                                    fileName: file?["display_name"] ?? "", ebookCount: ebooks,
                                    wordCount: file?["word_count"], sizeBytes: file?["size_bytes"] ?? 0)
        if let file, pageBasedTypes.contains(file["file_type"] as String) {
            // A CBZ's own page list is the count the reader pages through.
            let listed = names(file["pages"]).count
            let count: Int? = listed > 0 ? listed : file["page_count"]
            guard let count, count > 0 else { return progress }
            progress.pageCount = count
            // Pages left, exactly, from the place it opens at, wherever it
            // was read.
            let page: Int = switch choice.start {
            case .page(let page): min(max(0, page), count - 1)
            default: PageLayout.page(at: percentage, pageCount: count)
            }
            progress.pagesLeft = count - 1 - page
            progress.chapter = try readerChapters(db, fileID: fileID).last { ($0.startPage ?? Int.max) <= page }?.title
            return progress
        }
        if let place = try Row.fetchOne(db, sql: "SELECT chapter, pages_left, percentage FROM reader_places WHERE file_id = ?",
                                        arguments: [fileID]),
           abs((place["percentage"] as Double) - progress.percentage) < 0.005 {
            progress.chapter = place["chapter"]
            progress.pagesLeft = place["pages_left"]
        }
        return progress
    }

    // MARK: - Recording

    /// Saves the reader's place in fileID locally and queues it for the
    /// server, with the Reading Session so far, in one transaction: a kill
    /// right after a turn keeps both. Turns of one session replace each
    /// other until sent, but reaching the end is always sent. Each carries
    /// when the session began, so a Read-through it starts begins that day
    /// even when only a later turn is sent.
    /// The place is a CFI in a Reflowable File, or a 0-based page in a
    /// Page-based one.
    ///
    /// queued saves it on the writer's queue without waiting, for saves
    /// made while the reader moves, so a commit, and every observer it
    /// wakes, never holds up a turn or a scroll. Saves land in the order
    /// made, and a later write waits for them, so a sync or a read after
    /// one sees them.
    func recordReading(fileID: Int, cfi: String?, page: Int? = nil, percentage: Double, atEnd: Bool, at: Date, sessionID: String,
                       sessionStartedAt: Date, session: FinishedSession?, queued: Bool = false) throws {
        let payload = Shishoapp.JournalPositionPayload(fileId: fileID, cfi: cfi, page: page, percentage: min(1, max(0, percentage)),
                                                       atEnd: atEnd, listeningSince: JournalTime.string(min(sessionStartedAt, at)))
        let data = try JSONEncoder().encode(payload)
        // Reaching the end is never replaced, not even by a later end: the
        // server finishes the Read-through at the first one, and undoes it
        // only for a move back soon after it.
        let key = atEnd ? nil : "position:\(fileID):\(sessionID)"
        let save: @Sendable (Database) throws -> Void = { db in
            try CatalogStore.writePosition(db, fileID: fileID, seconds: 0, cfi: cfi, page: page, percentage: percentage, at: at,
                                           payload: data, coalesceKey: key)
            if let session {
                try CatalogStore.appendSession(db, session)
            }
        }
        if queued {
            writeQueued("file \(fileID): couldn't save the place", save)
        } else {
            try pool.write(save)
        }
    }

    /// Writes without waiting, on the writer's queue, so it lands before
    /// any write made after it. A failure goes to Diagnostics, as nothing
    /// waits to hear it.
    func writeQueued(_ failure: String, category: Diagnostics.Category = .reader,
                     _ updates: @escaping @Sendable (Database) throws -> Void) {
        pool.asyncWrite(updates) { _, result in
            guard case .failure(let error) = result else { return }
            Task { await Diagnostics.shared.log(category, "\(failure): \(Diagnostics.describe(error))") }
        }
    }

    /// Keeps the place the reader shows as this device's, without sending
    /// it, when the user chose it over another device's, and marks the
    /// server's place seen: the next open stays here rather than going to
    /// the place they turned down, or to an older place of this device's.
    func stayHere(fileID: Int, cfi: String?, page: Int? = nil, percentage: Double, at: Date) throws {
        try pool.write { db in
            let serverAt = try Double.fetchOne(db, sql: "SELECT occurred_at FROM reading_positions WHERE file_id = ?", arguments: [fileID])
            try db.execute(sql: """
                INSERT INTO local_positions (file_id, seconds, cfi, page, percentage, updated_at, base_occurred_at)
                VALUES (?, 0, ?, ?, ?, ?, ?)
                ON CONFLICT (file_id) DO UPDATE SET cfi = excluded.cfi, page = excluded.page, percentage = excluded.percentage,
                    updated_at = excluded.updated_at
                """, arguments: [fileID, cfi, page, percentage, at.timeIntervalSince1970, serverAt])
            try CatalogStore.resolveOffer(db, fileID: fileID)
        }
    }

    /// Keeps the chapter and pages left in the book the reader shows at
    /// percentage through fileID, for its Book's page.
    func noteReaderPlace(fileID: Int, chapter: String?, pagesLeft: Int?, percentage: Double) throws {
        try pool.write { db in
            try db.execute(sql: """
                INSERT INTO reader_places (file_id, chapter, pages_left, percentage) VALUES (?, ?, ?, ?)
                ON CONFLICT (file_id) DO UPDATE SET chapter = excluded.chapter, pages_left = excluded.pages_left,
                    percentage = excluded.percentage
                """, arguments: [fileID, chapter, pagesLeft, percentage])
        }
    }

    // MARK: - Page flow

    /// The Page flow the user chose for bookID: one chosen here that the
    /// feed hasn't brought back yet, or kept here alone, or else the one the
    /// feed brought, from here or the web; nil when they chose none.
    static func pageFlow(_ db: Database, bookID: Int) throws -> PageFlow? {
        try String.fetchOne(db, sql: """
            SELECT COALESCE((SELECT page_flow FROM chosen_page_flows WHERE book_id = ?),
                            (SELECT page_flow FROM user_books WHERE book_id = ?))
            """, arguments: [bookID, bookID]).flatMap(PageFlow.init(rawValue:))
    }

    /// Saves the Page flow the user chose for bookID: it's queued for the
    /// server, which keeps it for the Book on the web too, and shows here
    /// until the feed brings the Book's record back after the server took it
    /// in. A later choice replaces one not yet sent.
    func choosePageFlow(_ flow: PageFlow, bookID: Int, at: Date) throws {
        let payload = Shishoapp.JournalPageFlowPayload(bookId: bookID, pageFlow: Models.PageFlow(rawValue: flow.rawValue))
        let data = try JSONEncoder().encode(payload)
        let key = "page_flow:\(bookID)"
        try pool.write { db in
            try db.execute(sql: "DELETE FROM journal WHERE coalesce_key = ? AND held_for IS NULL", arguments: [key])
            try CatalogStore.append(db, type: .pageFlow, at: at, payload: data, fileID: nil, coalesceKey: key)
            let eventID = try String.fetchOne(db, sql: "SELECT event_id FROM journal WHERE seq = last_insert_rowid()")
            try db.execute(sql: """
                INSERT INTO chosen_page_flows (book_id, page_flow, event_id, accepted) VALUES (?, ?, ?, FALSE)
                ON CONFLICT (book_id) DO UPDATE SET page_flow = excluded.page_flow, event_id = excluded.event_id, accepted = FALSE
                """, arguments: [bookID, flow.rawValue, eventID])
        }
    }

    /// Queues a Reading Session on its own, as when it ends with no move.
    func recordReading(session: FinishedSession, queued: Bool = false) throws {
        let save: @Sendable (Database) throws -> Void = { db in
            try CatalogStore.appendSession(db, session)
        }
        if queued {
            writeQueued("file \(session.fileID): couldn't save the session", save)
        } else {
            try pool.write(save)
        }
    }
}
