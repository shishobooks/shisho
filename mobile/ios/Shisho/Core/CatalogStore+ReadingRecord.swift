import Foundation
import GRDB
import ShishoAPI

/// A change to the user's reading record made on the device, sent through
/// the journal (ADR 0024).
enum ReadingChange: Equatable, Sendable {
    case start(book: Int)
    case finish(book: Int)
    case abandon(book: Int)
    case pause(book: Int)
    case resume(book: Int)
    /// Mark as not started: the Read-through underway goes.
    case notStarted(book: Int)
    /// A Read-through from Reading History, by its id here.
    case deleteReadThrough(id: Int)
    case markReference(book: Int)
    case removeReference(book: Int)
    /// Quarter stars, 1 to 20, or nil to clear.
    case rating(book: Int, units: Int?)
    case addToRead(book: Int)
    case removeFromToRead(book: Int)
    case moveToRead(book: Int, after: Int?)
    /// Returns the List's id here, through recordList.
    case createList(name: String, description: String?, ordered: Bool)
    case renameList(id: Int, name: String)
    case deleteList(id: Int)
    case addToList(id: Int, book: Int)
    case removeFromList(id: Int, book: Int)
    case moveInList(id: Int, book: Int, after: Int?)
    case clearPosition(file: Int)

    /// The capability a server lists for it (ADR 0024).
    var capability: Shishoapp.Capability {
        switch self {
        case .start, .finish, .abandon, .pause, .resume, .notStarted, .deleteReadThrough, .markReference, .removeReference:
            .readingStatus
        case .rating: .ratings
        case .addToRead, .removeFromToRead, .moveToRead: .toRead
        case .createList, .renameList, .deleteList, .addToList, .removeFromList, .moveInList: .lists
        case .clearPosition: .clearPosition
        }
    }
}

extension CatalogStore {
    // MARK: - Schema

    /// The user's reading record as the feed sends it, kept apart from what
    /// screens read: user_books, read_throughs, lists, and list_books hold it
    /// with the changes the server hasn't confirmed laid over it, rebuilt
    /// whenever either changes. Accepted changes wait in journal_applied
    /// until the feed brings them back, so nothing flickers in between, and
    /// Reading Stats keeps a copy of each range it showed, with what it was
    /// fetched after.
    static func migrateReadingRecord(_ db: Database) throws {
        try db.create(table: "synced_user_books") { t in
            t.primaryKey("book_id", .integer)
            t.column("rating", .integer)
            t.column("to_read_position", .integer)
            t.column("reference", .boolean).notNull()
            t.column("page_flow", .text)
            t.column("generation", .integer).notNull()
        }
        try db.execute(sql: """
            INSERT INTO synced_user_books (book_id, rating, to_read_position, reference, page_flow, generation)
            SELECT book_id, rating, to_read_position, reference, page_flow, generation FROM user_books
            """)
        try db.create(table: "synced_read_throughs") { t in
            t.primaryKey("id", .integer)
            t.column("book_id", .integer).indexed()
            t.column("begin_day", .text)
            t.column("end_day", .text)
            t.column("outcome", .text).notNull()
            t.column("paused", .boolean).notNull()
            t.column("last_read_at", .double)
            t.column("book_title", .text)
            t.column("book_authors", .text)
            t.column("generation", .integer).notNull()
        }
        try db.execute(sql: """
            INSERT INTO synced_read_throughs (id, book_id, begin_day, end_day, outcome, paused, last_read_at, generation)
            SELECT id, book_id, begin_day, end_day, outcome, paused, last_read_at, generation FROM read_throughs
            """)
        try db.create(table: "synced_lists") { t in
            t.primaryKey("id", .integer)
            t.column("local_id", .text)
            t.column("name", .text).notNull()
            t.column("description", .text)
            t.column("is_ordered", .boolean).notNull()
            t.column("default_sort", .text).notNull()
            t.column("owner_id", .integer).notNull()
            t.column("owner_name", .text).notNull()
            t.column("permission", .text).notNull()
            t.column("shares", .text).notNull().defaults(to: "[]")
            t.column("generation", .integer).notNull()
        }
        try db.execute(sql: """
            INSERT INTO synced_lists (id, name, description, is_ordered, default_sort, owner_id, owner_name, permission, shares, generation)
            SELECT id, name, description, is_ordered, default_sort, owner_id, owner_name, permission, shares, generation FROM lists
            """)
        try db.create(table: "synced_list_books") { t in
            t.column("list_id", .integer).notNull().references("synced_lists", onDelete: .cascade)
            t.column("position", .integer).notNull()
            t.column("book_id", .integer).notNull()
            t.column("sort_order", .integer)
            t.column("added_at", .double).notNull()
            t.primaryKey(["list_id", "position"])
        }
        try db.execute(sql: """
            INSERT INTO synced_list_books (list_id, position, book_id, sort_order, added_at)
            SELECT list_id, position, book_id, sort_order, added_at FROM list_books
            """)
        try db.alter(table: "read_throughs") { t in
            t.add(column: "book_title", .text)
            t.add(column: "book_authors", .text)
            t.add(column: "pending", .boolean).notNull().defaults(to: false)
            t.add(column: "ended_here", .boolean).notNull().defaults(to: false)
        }
        try db.alter(table: "lists") { t in
            t.add(column: "local_id", .text)
            t.add(column: "pending", .boolean).notNull().defaults(to: false)
        }
        // The Book a journal event is about, so a Book's changes are found
        // without reading every payload.
        try db.alter(table: "journal") { t in
            t.add(column: "book_id", .integer).indexed()
        }
        try db.create(table: "journal_applied") { t in
            t.primaryKey("seq", .integer)
            t.column("event_id", .text).notNull()
            t.column("type", .text).notNull()
            t.column("at", .double).notNull()
            t.column("payload", .blob).notNull()
            t.column("file_id", .integer)
            t.column("book_id", .integer)
            t.column("accepted_at", .double).notNull()
        }
        // Reading Stats as the server last sent each range: 0 for all time,
        // or the year.
        try db.create(table: "stats_cache") { t in
            t.primaryKey("range_key", .integer)
            t.column("body", .blob).notNull()
            t.column("fetched_at", .double).notNull()
        }
        // Reading Sessions the server took in that no copy of Reading Stats
        // counts yet, and how much of each sitting the copies do count, so
        // a sitting's later snapshot adds only what's new.
        try db.create(table: "unstated_sessions") { t in
            t.primaryKey("event_id", .text)
            t.column("payload", .blob).notNull()
            t.column("accepted_at", .double).notNull()
        }
        try db.create(table: "counted_sessions") { t in
            t.primaryKey("session_id", .text)
            t.column("seconds", .double).notNull()
            t.column("counted_at", .double).notNull()
        }
    }

    // MARK: - Recording

    /// The event types the reading record lays over the synced copy.
    static let layerTypes: Set<String> = Set([
        Shishoapp.JournalEventType.readThroughStart, .readThroughFinish, .readThroughAbandon, .readThroughPause,
        .readThroughResume, .readThroughDelete, .referenceMark, .referenceRemove, .ratingSet, .ratingClear, .toReadAdd,
        .toReadRemove, .toReadMove, .listCreate, .listRename, .listDelete, .listAdd, .listRemove, .listMove,
        .readingPosition, .readingSession,
    ].map(\.rawValue))

    /// The phone's own day at date, as the journal's events carry it.
    static func day(_ date: Date, in zone: TimeZone = .current) -> String {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = zone
        let parts = calendar.dateComponents([.year, .month, .day], from: date)
        return String(format: "%04d-%02d-%02d", parts.year ?? 0, parts.month ?? 1, parts.day ?? 1)
    }

    /// Records change in the journal and shows it at once. A List it makes
    /// returns its id here.
    @discardableResult
    func record(_ change: ReadingChange, at date: Date = Date()) throws -> Int? {
        try pool.write { db in
            try CatalogStore.record(change, at: date, in: db)
        }
    }

    @discardableResult
    static func record(_ change: ReadingChange, at date: Date, in db: Database) throws -> Int? {
        let day = day(date)
        let encoder = JSONEncoder()
        func book(_ id: Int) throws -> Data {
            try encoder.encode(Shishoapp.JournalBookPayload(bookId: id, day: day))
        }
        var made: Int?
        var bookID: Int?
        var fileID: Int?
        let type: Shishoapp.JournalEventType
        let payload: Data
        switch change {
        case .start(let id):
            (type, payload, bookID) = (.readThroughStart, try book(id), id)
        case .finish(let id):
            (type, payload, bookID) = (.readThroughFinish, try book(id), id)
        case .abandon(let id):
            (type, payload, bookID) = (.readThroughAbandon, try book(id), id)
        case .pause(let id):
            (type, payload, bookID) = (.readThroughPause, try book(id), id)
        case .resume(let id):
            (type, payload, bookID) = (.readThroughResume, try book(id), id)
        case .notStarted(let id):
            bookID = id
            type = .readThroughDelete
            // One started here and not sent yet goes with its start.
            let underway = try Int.fetchOne(db, sql: "SELECT id FROM read_throughs WHERE book_id = ? AND outcome = 'underway'",
                                            arguments: [id])
            if let underway, underway > 0 {
                payload = try encoder.encode(Shishoapp.JournalReadThroughDeletePayload(readThroughId: underway, day: day))
            } else {
                payload = try encoder.encode(Shishoapp.JournalReadThroughDeletePayload(bookId: id, day: day))
            }
        case .deleteReadThrough(let id):
            bookID = try Int.fetchOne(db, sql: "SELECT book_id FROM read_throughs WHERE id = ?", arguments: [id])
            type = .readThroughDelete
            if id > 0 {
                payload = try encoder.encode(Shishoapp.JournalReadThroughDeletePayload(readThroughId: id, day: day))
            } else {
                // Made here and not confirmed. One a status choice made that
                // hasn't been sent goes with it, as if never chosen, and so
                // do the later choices about it not sent either.
                let made = [Shishoapp.JournalEventType.readThroughStart, .readThroughFinish].map(\.rawValue)
                let maker = try String.fetchOne(db, sql: "SELECT type FROM journal WHERE seq = ? AND held_for IS NULL", arguments: [-id])
                if let maker, made.contains(maker), let bookID {
                    // Up to the next pass, which a later start begins.
                    let next = try Int.fetchOne(db, sql: "SELECT MIN(seq) FROM journal WHERE seq > ? AND book_id = ? AND type = ?",
                                                arguments: [-id, bookID, Shishoapp.JournalEventType.readThroughStart.rawValue]) ?? Int.max
                    let later = [Shishoapp.JournalEventType.readThroughStart, .readThroughFinish, .readThroughAbandon,
                                 .readThroughPause, .readThroughResume].map { "'\($0.rawValue)'" }.joined(separator: ",")
                    try db.execute(sql: """
                        DELETE FROM journal WHERE seq >= ? AND seq < ? AND book_id = ? AND type IN (\(later)) AND held_for IS NULL
                        """, arguments: [-id, next, bookID])
                    try rebuildLayer(db)
                    return nil
                }
                // Otherwise only the one underway can be named before the
                // feed brings its id.
                let underway = try Bool.fetchOne(db, sql: "SELECT outcome = 'underway' FROM read_throughs WHERE id = ?",
                                                 arguments: [id]) ?? false
                guard underway, let bookID else { return nil }
                payload = try encoder.encode(Shishoapp.JournalReadThroughDeletePayload(bookId: bookID, day: day))
            }
        case .markReference(let id):
            (type, payload, bookID) = (.referenceMark, try book(id), id)
        case .removeReference(let id):
            (type, payload, bookID) = (.referenceRemove, try book(id), id)
        case .rating(let id, let units):
            bookID = id
            if let units {
                (type, payload) = (.ratingSet, try encoder.encode(Shishoapp.JournalRatingPayload(bookId: id, rating: units)))
            } else {
                (type, payload) = (.ratingClear, try book(id))
            }
        case .addToRead(let id):
            (type, payload, bookID) = (.toReadAdd, try book(id), id)
        case .removeFromToRead(let id):
            (type, payload, bookID) = (.toReadRemove, try book(id), id)
        case .moveToRead(let id, let after):
            (type, payload, bookID) = (.toReadMove, try encoder.encode(Shishoapp.JournalToReadMovePayload(bookId: id, afterBookId: after)), id)
        case .createList(let name, let description, let ordered):
            let localID = UUID().uuidString.lowercased()
            type = .listCreate
            payload = try encoder.encode(Shishoapp.JournalListCreatePayload(localId: localID, name: name, description: description,
                                                                           isOrdered: ordered))
        case .renameList(let id, let name):
            let ref = try listRef(id, in: db)
            type = .listRename
            payload = try encoder.encode(Shishoapp.JournalListRenamePayload(listId: ref.listId, localId: ref.localId, name: name))
        case .deleteList(let id):
            type = .listDelete
            payload = try encoder.encode(try listRef(id, in: db))
        case .addToList(let id, let book), .removeFromList(let id, let book):
            let ref = try listRef(id, in: db)
            bookID = book
            if case .addToList = change { type = .listAdd } else { type = .listRemove }
            payload = try encoder.encode(Shishoapp.JournalListBookPayload(listId: ref.listId, localId: ref.localId, bookId: book))
        case .moveInList(let id, let book, let after):
            let ref = try listRef(id, in: db)
            bookID = book
            type = .listMove
            payload = try encoder.encode(Shishoapp.JournalListMovePayload(listId: ref.listId, localId: ref.localId, bookId: book,
                                                                         afterBookId: after))
        case .clearPosition(let file):
            fileID = file
            type = .positionClear
            payload = try encoder.encode(Shishoapp.JournalFilePayload(fileId: file))
            // This device's own place goes at once, as the web's does. Moves
            // not sent yet stay, since they can start or finish the Book;
            // the server clears the place they leave, being older.
            try db.execute(sql: "DELETE FROM local_positions WHERE file_id = ?", arguments: [file])
            try db.execute(sql: "DELETE FROM reader_places WHERE file_id = ?", arguments: [file])
            try db.execute(sql: "DELETE FROM reading_positions WHERE file_id = ?", arguments: [file])
        }
        let seq = try append(db, type: type, at: date, payload: payload, fileID: fileID, coalesceKey: nil, bookID: bookID)
        if case .createList = change {
            made = ReadingLayer.localID(seq)
        }
        return made
    }

    /// How the journal names List id: by the server's id, or by the local
    /// id a List made here is known by until the feed brings it.
    private static func listRef(_ id: Int, in db: Database) throws -> Shishoapp.JournalListRef {
        if id > 0 { return Shishoapp.JournalListRef(listId: id) }
        let localID = try String.fetchOne(db, sql: "SELECT local_id FROM lists WHERE id = ?", arguments: [id])
        return Shishoapp.JournalListRef(localId: localID ?? "")
    }

    // MARK: - Laying changes over the record

    /// Rebuilds what screens read from the synced copy and the changes not
    /// yet confirmed: everything, or only books' Read-throughs and records,
    /// as a place or session saved while reading needs. Only rows that
    /// differ are written, so screens observing them redraw only when the
    /// record really changed.
    static func rebuildLayer(_ db: Database, books: Set<Int>? = nil) throws {
        var record = try syncedRecord(db, books: books)
        let events = try layerEvents(db, books: books)
        let files = try layerFiles(db, events: events)
        ReadingLayer.apply(events, to: &record, files: files)
        try write(record, books: books, in: db)
    }

    private static func idList(_ ids: Set<Int>) -> String {
        ids.isEmpty ? "NULL" : ids.map(String.init).joined(separator: ",")
    }

    private static func syncedRecord(_ db: Database, books: Set<Int>?) throws -> ReadingRecord {
        var record = ReadingRecord()
        let only = books.map { " WHERE book_id IN (\(idList($0)))" } ?? ""
        for row in try Row.fetchAll(db, sql: "SELECT book_id, rating, to_read_position, reference, page_flow FROM synced_user_books\(only)") {
            record.userBooks[row["book_id"]] = .init(rating: row["rating"], reference: row["reference"], pageFlow: row["page_flow"])
        }
        record.toRead = try Int.fetchAll(db, sql: """
            SELECT book_id FROM synced_user_books\(only.isEmpty ? " WHERE" : only + " AND") to_read_position IS NOT NULL
            ORDER BY to_read_position, book_id
            """)
        for row in try Row.fetchAll(db, sql: """
            SELECT id, book_id, begin_day, end_day, outcome, paused, last_read_at, book_title, book_authors,
                0 AS pending, 0 AS ended_here FROM synced_read_throughs\(only)
            """) {
            let rt = readThrough(row)
            record.readThroughs[rt.id] = rt
        }
        guard books == nil else { return record }
        var listBooks: [Int: [ReadingRecord.ListBook]] = [:]
        for row in try Row.fetchAll(db, sql: "SELECT list_id, book_id, sort_order, added_at FROM synced_list_books ORDER BY list_id, position") {
            listBooks[row["list_id"], default: []].append(.init(bookID: row["book_id"], sortOrder: row["sort_order"], addedAt: row["added_at"]))
        }
        for row in try Row.fetchAll(db, sql: """
            SELECT id, local_id, name, description, is_ordered, default_sort, owner_id, owner_name, permission, shares,
                0 AS pending FROM synced_lists
            """) {
            let list = list(row, books: listBooks)
            record.lists[list.id] = list
        }
        return record
    }

    private static func readThrough(_ row: Row) -> ReadingRecord.ReadThrough {
        let id: Int = row["id"]
        var rt = ReadingRecord.ReadThrough(id: id, outcome: row["outcome"])
        rt.bookID = row["book_id"]
        rt.beginDay = row["begin_day"]
        rt.endDay = row["end_day"]
        rt.paused = row["paused"]
        rt.lastReadAt = row["last_read_at"]
        rt.bookTitle = row["book_title"]
        rt.bookAuthors = row["book_authors"]
        rt.pending = row["pending"]
        rt.endedHere = row["ended_here"]
        return rt
    }

    private static func list(_ row: Row, books: [Int: [ReadingRecord.ListBook]]) -> ReadingRecord.List {
        let id: Int = row["id"]
        let name: String = row["name"]
        let ordered: Bool = row["is_ordered"]
        let sort: String = row["default_sort"]
        let ownerID: Int = row["owner_id"]
        let ownerName: String = row["owner_name"]
        let permission: String = row["permission"]
        let shares: String = row["shares"]
        var list = ReadingRecord.List(id: id, name: name, isOrdered: ordered, defaultSort: sort, ownerID: ownerID, ownerName: ownerName,
                                      permission: permission, shares: shares, books: books[id] ?? [])
        list.localID = row["local_id"]
        list.description = row["description"]
        list.pending = row["pending"]
        return list
    }

    /// The changes the server hasn't confirmed, oldest first: those it took
    /// in that the feed hasn't brought back yet, then those not yet sent.
    static func layerEvents(_ db: Database, books: Set<Int>? = nil) throws -> [LayerEvent] {
        let types = layerTypes.map { "'\($0)'" }.joined(separator: ",")
        let book = "COALESCE(book_id, (SELECT f.book_id FROM book_files f WHERE f.id = file_id))"
        let only = books.map { " AND \(book) IN (\(idList($0)))" } ?? ""
        let rows = try Row.fetchAll(db, sql: """
            SELECT seq, type, at, payload, 1 AS accepted FROM journal_applied WHERE type IN (\(types))\(only)
            UNION ALL
            SELECT seq, type, at, payload, 0 AS accepted FROM journal WHERE type IN (\(types))\(only)
            ORDER BY seq
            """)
        return rows.compactMap { row in
            var event = layerEvent(seq: row["seq"], type: row["type"], at: row["at"], payload: row["payload"])
            event?.accepted = row["accepted"]
            return event
        }
    }

    static func layerEvent(seq: Int, type: String, at: Double, payload: Data) -> LayerEvent? {
        let decoder = makeShishoDecoder()
        func decode<T: Decodable>(_ kind: T.Type) -> T? { try? decoder.decode(kind, from: payload) }
        func ref(_ listID: Int?, _ localID: String?) -> ListRef? {
            if let listID { return .id(listID) }
            if let localID, !localID.isEmpty { return .local(localID) }
            return nil
        }
        let date = Date(timeIntervalSince1970: at)
        let change: LayerEvent.Change?
        switch Shishoapp.JournalEventType(rawValue: type) {
        case .readThroughStart:
            change = decode(Shishoapp.JournalBookPayload.self).map { .start(book: $0.bookId, day: $0.day.isEmpty ? day(date) : $0.day) }
        case .readThroughFinish:
            change = decode(Shishoapp.JournalBookPayload.self).map { .finish(book: $0.bookId, day: $0.day.isEmpty ? day(date) : $0.day) }
        case .readThroughAbandon:
            change = decode(Shishoapp.JournalBookPayload.self).map { .abandon(book: $0.bookId, day: $0.day.isEmpty ? day(date) : $0.day) }
        case .readThroughPause:
            change = decode(Shishoapp.JournalBookPayload.self).map { .pause(book: $0.bookId) }
        case .readThroughResume:
            change = decode(Shishoapp.JournalBookPayload.self).map { .resume(book: $0.bookId) }
        case .readThroughDelete:
            change = decode(Shishoapp.JournalReadThroughDeletePayload.self).map { .deleteReadThrough(id: $0.readThroughId, book: $0.bookId) }
        case .referenceMark:
            change = decode(Shishoapp.JournalBookPayload.self).map { .referenceMark(book: $0.bookId) }
        case .referenceRemove:
            change = decode(Shishoapp.JournalBookPayload.self).map { .referenceRemove(book: $0.bookId) }
        case .ratingSet:
            change = decode(Shishoapp.JournalRatingPayload.self).map { .rating(book: $0.bookId, units: $0.rating) }
        case .ratingClear:
            change = decode(Shishoapp.JournalBookPayload.self).map { .rating(book: $0.bookId, units: nil) }
        case .toReadAdd:
            change = decode(Shishoapp.JournalBookPayload.self).map { .toReadAdd(book: $0.bookId) }
        case .toReadRemove:
            change = decode(Shishoapp.JournalBookPayload.self).map { .toReadRemove(book: $0.bookId) }
        case .toReadMove:
            change = decode(Shishoapp.JournalToReadMovePayload.self).map { .toReadMove(book: $0.bookId, after: $0.afterBookId) }
        case .listCreate:
            change = decode(Shishoapp.JournalListCreatePayload.self).map {
                .listCreate(localID: $0.localId, name: $0.name, description: $0.description, ordered: $0.isOrdered)
            }
        case .listRename:
            change = decode(Shishoapp.JournalListRenamePayload.self).flatMap { p in
                ref(p.listId, p.localId).map { .listRename(list: $0, name: p.name) }
            }
        case .listDelete:
            change = decode(Shishoapp.JournalListRef.self).flatMap { p in ref(p.listId, p.localId).map { .listDelete(list: $0) } }
        case .listAdd:
            change = decode(Shishoapp.JournalListBookPayload.self).flatMap { p in
                ref(p.listId, p.localId).map { .listAdd(list: $0, book: p.bookId) }
            }
        case .listRemove:
            change = decode(Shishoapp.JournalListBookPayload.self).flatMap { p in
                ref(p.listId, p.localId).map { .listRemove(list: $0, book: p.bookId) }
            }
        case .listMove:
            change = decode(Shishoapp.JournalListMovePayload.self).flatMap { p in
                ref(p.listId, p.localId).map { .listMove(list: $0, book: p.bookId, after: p.afterBookId) }
            }
        case .readingPosition:
            change = decode(Shishoapp.JournalPositionPayload.self).map { p in
                let since = p.listeningSince.flatMap(CatalogStore.timestamp).map { day(Date(timeIntervalSince1970: $0)) }
                return .position(file: p.fileId, seconds: p.seconds, page: p.page, percentage: p.percentage, atEnd: p.atEnd,
                                 listened: p.listenedSeconds ?? 0, day: day(date), sinceDay: since)
            }
        case .readingSession:
            change = decode(Shishoapp.JournalSessionPayload.self).map { .session(file: $0.fileId) }
        default:
            change = nil
        }
        return change.map { LayerEvent(seq: seq, at: at, change: $0) }
    }

    private static func layerFiles(_ db: Database, events: [LayerEvent]) throws -> [Int: LayerFile] {
        var ids = Set<Int>()
        for event in events {
            switch event.change {
            case .position(let file, _, _, _, _, _, _, _), .session(let file):
                ids.insert(file)
            default:
                continue
            }
        }
        guard !ids.isEmpty else { return [:] }
        var files: [Int: LayerFile] = [:]
        for row in try Row.fetchAll(db, sql: """
            SELECT id, book_id, file_role, page_count, duration_seconds FROM book_files WHERE id IN (\(idList(ids)))
            """) {
            files[row["id"]] = LayerFile(bookID: row["book_id"], supplement: (row["file_role"] as String) == "supplement",
                                         pageCount: row["page_count"], duration: row["duration_seconds"])
        }
        return files
    }

    /// Writes record over what screens read, changing only rows that
    /// differ. With books, only those Books' records are written, and a Book
    /// on To Read keeps its place there.
    private static func write(_ record: ReadingRecord, books: Set<Int>?, in db: Database) throws {
        let only = books.map { " WHERE book_id IN (\(idList($0)))" } ?? ""

        // The user's record of each Book.
        var places: [Int: Int] = [:]
        if books == nil {
            for (index, id) in record.toRead.enumerated() { places[id] = index + 1 }
        } else {
            let current = Dictionary(try Row.fetchAll(db, sql: "SELECT book_id, to_read_position FROM user_books\(only)").compactMap { row in
                (row["to_read_position"] as Int?).map { (row["book_id"] as Int, $0) }
            }, uniquingKeysWith: { first, _ in first })
            var next = (try Int.fetchOne(db, sql: "SELECT MAX(to_read_position) FROM user_books") ?? 0) + 1
            for id in record.toRead {
                if let place = current[id] {
                    places[id] = place
                } else {
                    places[id] = next
                    next += 1
                }
            }
        }
        let bookIDs = Set(record.userBooks.keys).union(record.toRead)
        var have: [Int: Row] = [:]
        for row in try Row.fetchAll(db, sql: "SELECT book_id, rating, to_read_position, reference, page_flow FROM user_books\(only)") {
            have[row["book_id"]] = row
        }
        for id in Set(have.keys).subtracting(bookIDs) {
            try db.execute(sql: "DELETE FROM user_books WHERE book_id = ?", arguments: [id])
        }
        for id in bookIDs {
            let want = record.userBooks[id] ?? .init()
            let place = places[id]
            if let row = have[id], row["rating"] as Int? == want.rating, row["to_read_position"] as Int? == place,
               row["reference"] as Bool == want.reference, row["page_flow"] as String? == want.pageFlow {
                continue
            }
            try db.execute(sql: """
                INSERT INTO user_books (book_id, rating, to_read_position, reference, page_flow, generation) VALUES (?, ?, ?, ?, ?, 0)
                ON CONFLICT (book_id) DO UPDATE SET rating = excluded.rating, to_read_position = excluded.to_read_position,
                    reference = excluded.reference, page_flow = excluded.page_flow
                """, arguments: [id, want.rating, place, want.reference, want.pageFlow])
        }

        // Read-throughs.
        var haveRTs: [Int: ReadingRecord.ReadThrough] = [:]
        for row in try Row.fetchAll(db, sql: """
            SELECT id, book_id, begin_day, end_day, outcome, paused, last_read_at, book_title, book_authors, pending, ended_here
            FROM read_throughs\(only)
            """) {
            let rt = readThrough(row)
            haveRTs[rt.id] = rt
        }
        for id in Set(haveRTs.keys).subtracting(record.readThroughs.keys) {
            try db.execute(sql: "DELETE FROM read_throughs WHERE id = ?", arguments: [id])
        }
        for (id, rt) in record.readThroughs where haveRTs[id] != rt {
            try db.execute(sql: """
                INSERT INTO read_throughs (id, book_id, begin_day, end_day, outcome, paused, last_read_at, book_title, book_authors,
                    pending, ended_here, generation)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
                ON CONFLICT (id) DO UPDATE SET book_id = excluded.book_id, begin_day = excluded.begin_day, end_day = excluded.end_day,
                    outcome = excluded.outcome, paused = excluded.paused, last_read_at = excluded.last_read_at,
                    book_title = excluded.book_title, book_authors = excluded.book_authors, pending = excluded.pending,
                    ended_here = excluded.ended_here
                """, arguments: [id, rt.bookID, rt.beginDay, rt.endDay, rt.outcome, rt.paused, rt.lastReadAt, rt.bookTitle,
                                 rt.bookAuthors, rt.pending, rt.endedHere])
        }

        // Lists, only when everything is rebuilt.
        guard books == nil else { return }
        var haveLists: [Int: ReadingRecord.List] = [:]
        var haveBooks: [Int: [ReadingRecord.ListBook]] = [:]
        for row in try Row.fetchAll(db, sql: "SELECT list_id, book_id, sort_order, added_at FROM list_books ORDER BY list_id, position") {
            haveBooks[row["list_id"], default: []].append(.init(bookID: row["book_id"], sortOrder: row["sort_order"], addedAt: row["added_at"]))
        }
        for row in try Row.fetchAll(db, sql: """
            SELECT id, local_id, name, description, is_ordered, default_sort, owner_id, owner_name, permission, shares, pending FROM lists
            """) {
            let list = list(row, books: haveBooks)
            haveLists[list.id] = list
        }
        for id in Set(haveLists.keys).subtracting(record.lists.keys) {
            try db.execute(sql: "DELETE FROM lists WHERE id = ?", arguments: [id])
        }
        for (id, list) in record.lists where haveLists[id] != list {
            try db.execute(sql: """
                INSERT INTO lists (id, local_id, name, description, is_ordered, default_sort, owner_id, owner_name, permission, shares,
                    pending, generation)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
                ON CONFLICT (id) DO UPDATE SET local_id = excluded.local_id, name = excluded.name, description = excluded.description,
                    is_ordered = excluded.is_ordered, default_sort = excluded.default_sort, owner_id = excluded.owner_id,
                    owner_name = excluded.owner_name, permission = excluded.permission, shares = excluded.shares,
                    pending = excluded.pending
                """, arguments: [id, list.localID, list.name, list.description, list.isOrdered, list.defaultSort, list.ownerID,
                                 list.ownerName, list.permission, list.shares, list.pending])
            if haveLists[id]?.books != list.books {
                try db.execute(sql: "DELETE FROM list_books WHERE list_id = ?", arguments: [id])
                for (position, book) in list.books.enumerated() {
                    try db.execute(sql: "INSERT INTO list_books (list_id, position, book_id, sort_order, added_at) VALUES (?, ?, ?, ?, ?)",
                                   arguments: [id, position, book.bookID, book.sortOrder, book.addedAt])
                }
            }
        }
    }

    // MARK: - Confirming

    /// Keeps a reading change the server took in until the feed brings the
    /// record back, and a Reading Session until Reading Stats count it.
    static func keepAccepted(eventID: String, in db: Database) throws {
        let types = layerTypes.map { "'\($0)'" }.joined(separator: ",")
        let now = Date().timeIntervalSince1970
        try db.execute(sql: """
            INSERT OR REPLACE INTO journal_applied (seq, event_id, type, at, payload, file_id, book_id, accepted_at)
            SELECT seq, event_id, type, at, payload, file_id, book_id, ? FROM journal WHERE event_id = ? AND type IN (\(types))
            """, arguments: [now, eventID])
        try db.execute(sql: """
            INSERT OR REPLACE INTO unstated_sessions (event_id, payload, accepted_at)
            SELECT event_id, payload, ? FROM journal WHERE event_id = ? AND type = ?
            """, arguments: [now, eventID, Shishoapp.JournalEventType.readingSession.rawValue])
    }

    /// The changes to a Book's Read-throughs, and to its own record,
    /// which the feed's entries of each kind bring back.
    static let readThroughTypes = [Shishoapp.JournalEventType.readThroughStart, .readThroughFinish, .readThroughAbandon,
                                   .readThroughPause, .readThroughResume, .readThroughDelete, .readingPosition, .readingSession]
    static let userBookTypes = [Shishoapp.JournalEventType.referenceMark, .referenceRemove, .ratingSet, .ratingClear, .toReadAdd,
                                .toReadRemove, .toReadMove]

    /// The feed brought a Book's Read-through or record: the changes of
    /// types to it the server took in are in the synced copy now, so they
    /// stop being laid over it. Syncs run one at a time, so what the feed
    /// brings after a change was taken in already holds it. Each kind
    /// confirms only its own changes: a Book's record can come a page before
    /// the Read-through a place started.
    static func confirmApplied(book: Int, types: [Shishoapp.JournalEventType], in db: Database) throws {
        let names = types.map { "'\($0.rawValue)'" }.joined(separator: ",")
        try db.execute(sql: """
            DELETE FROM journal_applied WHERE COALESCE(book_id, (SELECT f.book_id FROM book_files f WHERE f.id = file_id)) = ?
                AND type IN (\(names))
            """, arguments: [book])
    }

    /// The feed brought a List: the changes to it the server took in, named
    /// by its id or the local id this installation made it under.
    static func confirmApplied(list: Int, localID: String?, in db: Database) throws {
        try db.execute(sql: """
            DELETE FROM journal_applied WHERE type LIKE 'list_%'
                AND (json_extract(CAST(payload AS TEXT), '$.list_id') = ? OR json_extract(CAST(payload AS TEXT), '$.local_id') = ?)
            """, arguments: [list, localID ?? ""])
    }

    /// The feed caught up after since: what the server took in before then
    /// is in the synced copy now.
    func feedCaughtUp(since: Date) throws {
        try pool.write { db in
            try db.execute(sql: "DELETE FROM journal_applied WHERE accepted_at < ?", arguments: [since.timeIntervalSince1970])
            try CatalogStore.rebuildLayer(db)
        }
    }
}
