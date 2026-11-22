import Foundation
import GRDB
import ShishoAPI

/// A Series, Person, Genre, or Tag as a browsing list shows it.
struct ResourceSummary: Identifiable, Hashable, Sendable {
    let id: Int
    let name: String
    /// How many of the device's Books name it.
    let bookCount: Int
    /// The line under the name, as the web app's lists show it: a Person's
    /// sort name, and other names it's found by.
    var detail = ""
    /// A Person's Books they wrote, and Files they narrated.
    var authoredCount = 0
    var narratedCount = 0
    /// Its first Books, for the covers its row shows.
    var previews: [BookSummary] = []
}

/// Which kind of resource a browsing screen lists.
enum ResourceKind: String, CaseIterable, Hashable, Sendable {
    case series, person, genre, tag

    /// The device's table of this kind, and its join to Books.
    fileprivate var table: String {
        switch self {
        case .series: "series"
        case .person: "persons"
        case .genre: "genres"
        case .tag: "tags"
        }
    }
}

/// A Series, Person, Genre, or Tag's own page.
struct ResourceDetail: Hashable, Sendable {
    let kind: ResourceKind
    let id: Int
    let name: String
    let description: String?
    let aliases: [String]
}

/// A List as the Lists tab shows it.
struct ListSummary: Identifiable, Hashable, Sendable {
    let id: Int
    let name: String
    let description: String?
    /// "owner", or what the List Share allows.
    let permission: String
    let ownerName: String
    let bookCount: Int
    /// The first Books' covers, in the List's order.
    let covers: [BookSummary]
    /// Who it's shared with, for its owner and managers.
    var shares: [ListShare] = []
}

/// Someone a List is shared with.
struct ListShare: Hashable, Codable, Sendable {
    let userID: Int
    let username: String
    let permission: String
}

/// The user's own standing with a Book: their Reading Status and Rating.
struct BookStanding: Hashable, Sendable {
    /// The server's name for the Reading Status, or nil for none.
    let status: String?
    /// Quarter stars, 1 to 20, or nil when unrated.
    let rating: Int?
}

/// What a search found on the device.
struct SearchResults: Hashable, Sendable {
    var books: [BookSummary] = []
    var series: [ResourceSummary] = []
    var persons: [ResourceSummary] = []
    var genres: [ResourceSummary] = []
    var tags: [ResourceSummary] = []

    var isEmpty: Bool {
        books.isEmpty && series.isEmpty && persons.isEmpty && genres.isEmpty && tags.isEmpty
    }
}

/// What applying feed entries touched, so the search index follows.
struct Touched {
    var books = Set<Int>()
    var series = Set<Int>()
    var persons = Set<Int>()
    var genres = Set<Int>()
    var tags = Set<Int>()
}

extension CatalogStore {
    // MARK: - Schema

    /// The whole catalog: Series, Persons, Genres, Tags, Publishers,
    /// Lists, and the user's record for each Book, what Books and Files
    /// gained with them, what the user can access, and the search index.
    static func migrateFullCatalog(_ db: Database) throws {
        try db.alter(table: "books") { t in
            t.add(column: "description", .text)
        }
        try db.alter(table: "book_files") { t in
            t.add(column: "publisher_id", .integer)
            t.add(column: "publisher", .text)
            // JSON arrays: [{"type","value"}], and a CBZ's page names.
            t.add(column: "identifiers", .text).notNull().defaults(to: "[]")
            t.add(column: "pages", .text).notNull().defaults(to: "[]")
            // What the file details sheet shows, as the web's does.
            t.add(column: "codec", .text)
            t.add(column: "bitrate", .integer)
            t.add(column: "language", .text)
            t.add(column: "abridged", .boolean)
            t.add(column: "url", .text)
            t.add(column: "reading_direction", .text)
            t.add(column: "filepath", .text).notNull().defaults(to: "")
            t.add(column: "file_modified_at", .double)
            t.add(column: "created_at", .double)
        }
        // False until a download from the start records what the user could
        // access when it began; access changes after are answered from then.
        try db.alter(table: "sync_state") { t in
            t.add(column: "access_known", .boolean).notNull().defaults(to: false)
        }
        try db.create(index: "book_authors_by_person", on: "book_authors", columns: ["person_id"])
        try db.create(index: "book_series_by_series", on: "book_series", columns: ["series_id"])
        for (table, column) in [("book_genres", "genre_id"), ("book_tags", "tag_id")] {
            try db.create(table: table) { t in
                t.column("book_id", .integer).notNull().references("books", onDelete: .cascade)
                t.column("position", .integer).notNull()
                t.column(column, .integer).notNull().indexed()
                t.column("name", .text).notNull()
                t.primaryKey(["book_id", "position"])
            }
        }
        // Each File's narrators by Person, for a Person's page and search.
        try db.create(table: "book_narrators") { t in
            t.column("file_id", .integer).notNull().references("book_files", onDelete: .cascade)
            t.column("position", .integer).notNull()
            t.column("book_id", .integer).notNull().indexed()
            t.column("person_id", .integer).notNull().indexed()
            t.column("name", .text).notNull()
            t.primaryKey(["file_id", "position"])
        }
        for table in ["series", "persons", "genres", "tags", "publishers"] {
            try db.create(table: table) { t in
                t.primaryKey("id", .integer)
                t.column("library_id", .integer).notNull().indexed()
                t.column("name", .text).notNull()
                t.column("sort_name", .text).notNull()
                t.column("description", .text)
                t.column("parent_id", .integer)
                t.column("aliases", .text).notNull().defaults(to: "[]")
                t.column("generation", .integer).notNull()
            }
        }
        try db.create(table: "lists") { t in
            t.primaryKey("id", .integer)
            t.column("name", .text).notNull()
            t.column("description", .text)
            t.column("is_ordered", .boolean).notNull()
            t.column("default_sort", .text).notNull()
            t.column("owner_id", .integer).notNull()
            t.column("owner_name", .text).notNull()
            t.column("permission", .text).notNull()
            // JSON: [{"user_id","username","permission"}], for its owner and
            // managers.
            t.column("shares", .text).notNull().defaults(to: "[]")
            t.column("generation", .integer).notNull()
        }
        try db.create(table: "list_books") { t in
            t.column("list_id", .integer).notNull().references("lists", onDelete: .cascade)
            t.column("position", .integer).notNull()
            t.column("book_id", .integer).notNull().indexed()
            t.column("sort_order", .integer)
            t.column("added_at", .double).notNull()
            t.primaryKey(["list_id", "position"])
        }
        try db.create(table: "user_books") { t in
            t.primaryKey("book_id", .integer)
            t.column("rating", .integer)
            t.column("to_read_position", .integer)
            t.column("reference", .boolean).notNull()
            t.column("generation", .integer).notNull()
        }
        // What the user can access, and how far a Library or List gained
        // since has been fetched whole (ADR 0023).
        try db.create(table: "library_access") { t in
            t.primaryKey("library_id", .integer)
            t.column("cursor", .integer).notNull().defaults(to: 0)
            t.column("complete", .boolean).notNull()
        }
        try db.create(table: "list_access") { t in
            t.primaryKey("list_id", .integer)
            t.column("complete", .boolean).notNull()
        }
        // The search index, made as the server makes its own (unicode61, with
        // prefix indexes), so a search finds what the web finds. Rows are
        // keyed by rowid equal to the record's id.
        let fts = [
            ("books_fts", "book_id UNINDEXED, library_id UNINDEXED, title, subtitle, authors, narrators, series_names"),
            ("series_fts", "series_id UNINDEXED, library_id UNINDEXED, name, description, book_titles, book_authors"),
            ("persons_fts", "person_id UNINDEXED, library_id UNINDEXED, name, sort_name"),
            ("genres_fts", "genre_id UNINDEXED, library_id UNINDEXED, name"),
            ("tags_fts", "tag_id UNINDEXED, library_id UNINDEXED, name"),
        ]
        for (table, columns) in fts {
            try db.execute(sql: "CREATE VIRTUAL TABLE \(table) USING fts5(\(columns), tokenize='unicode61', prefix='2,3')")
        }
    }

    // MARK: - Applying

    /// Applies one entry of the kinds this file adds; others are skipped.
    static func applyCatalogEntry(_ entry: Shishoapp.FeedEntry, generation: Int, touched: inout Touched, in db: Database) throws {
        switch entry.kind {
        case .series:
            try applyResource("series", entry: entry, generation: generation, in: db,
                              values: entry.series.map { [$0.id, $0.libraryId, $0.name, $0.sortName, $0.description, nil, namesJSON($0.aliases)] })
            touched.series.insert(entry.id)
            // A Series' Books carry its aliases in their search row.
            try touched.books.formUnion(Int.fetchAll(db, sql: "SELECT book_id FROM book_series WHERE series_id = ?", arguments: [entry.id]))
        case .person:
            try applyResource("persons", entry: entry, generation: generation, in: db,
                              values: entry.person.map { [$0.id, $0.libraryId, $0.name, $0.sortName, nil, nil, namesJSON($0.aliases)] })
            touched.persons.insert(entry.id)
            try touched.books.formUnion(Int.fetchAll(db, sql: """
                SELECT book_id FROM book_authors WHERE person_id = ? UNION SELECT book_id FROM book_narrators WHERE person_id = ?
                """, arguments: [entry.id, entry.id]))
        case .genre:
            try applyResource("genres", entry: entry, generation: generation, in: db,
                              values: entry.genre.map { [$0.id, $0.libraryId, $0.name, $0.name, nil, nil, namesJSON($0.aliases)] })
            touched.genres.insert(entry.id)
        case .tag:
            try applyResource("tags", entry: entry, generation: generation, in: db,
                              values: entry.tag.map { [$0.id, $0.libraryId, $0.name, $0.name, nil, nil, namesJSON($0.aliases)] })
            touched.tags.insert(entry.id)
        case .publisher:
            try applyResource("publishers", entry: entry, generation: generation, in: db,
                              values: entry.publisher.map { [$0.id, $0.libraryId, $0.name, $0.name, nil, $0.parentId, namesJSON($0.aliases)] })
        case .bookLeft:
            // The Book moved out of this Library; one that has since arrived
            // in another Library stays.
            touched.books.insert(entry.id)
            try touched.series.formUnion(Int.fetchAll(db, sql: """
                SELECT series_id FROM book_series bs JOIN books b ON b.id = bs.book_id WHERE b.id = ? AND b.library_id = ?
                """, arguments: [entry.id, entry.libraryId]))
            try db.execute(sql: "DELETE FROM books WHERE id = ? AND library_id = ?", arguments: [entry.id, entry.libraryId])
        case .userBook:
            if entry.deleted {
                try db.execute(sql: "DELETE FROM user_books WHERE book_id = ?", arguments: [entry.id])
            } else if let record = entry.userBook {
                try db.execute(sql: """
                    INSERT INTO user_books (book_id, rating, to_read_position, reference, generation) VALUES (?, ?, ?, ?, ?)
                    ON CONFLICT (book_id) DO UPDATE SET rating = excluded.rating, to_read_position = excluded.to_read_position,
                        reference = excluded.reference, generation = excluded.generation
                    """, arguments: [record.bookId, record.rating, record.toReadPosition, record.reference, generation])
            }
        case .list:
            if entry.deleted {
                try dropList(entry.id, in: db)
            } else if let list = entry.list {
                try upsert(list, generation: generation, in: db)
            }
        default:
            return
        }
    }

    /// Upserts or deletes one resource row; values are id, library_id, name,
    /// sort_name, description, parent_id, aliases.
    private static func applyResource(_ table: String, entry: Shishoapp.FeedEntry, generation: Int, in db: Database,
                                      values: [(any DatabaseValueConvertible)?]?) throws {
        if entry.deleted {
            try db.execute(sql: "DELETE FROM \(table) WHERE id = ?", arguments: [entry.id])
            return
        }
        guard let values else { return }
        try db.execute(sql: """
            INSERT INTO \(table) (id, library_id, name, sort_name, description, parent_id, aliases, generation)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT (id) DO UPDATE SET library_id = excluded.library_id, name = excluded.name, sort_name = excluded.sort_name,
                description = excluded.description, parent_id = excluded.parent_id, aliases = excluded.aliases,
                generation = excluded.generation
            """, arguments: StatementArguments(values + [generation]))
    }

    private static func upsert(_ list: Shishoapp.FeedList, generation: Int, in db: Database) throws {
        let shares = list.shares.map { ListShare(userID: $0.user.id, username: $0.user.username, permission: $0.permission.rawValue) }
        let sharesJSON = (try? String(data: JSONEncoder().encode(shares), encoding: .utf8)) ?? "[]"
        try db.execute(sql: """
            INSERT INTO lists (id, name, description, is_ordered, default_sort, owner_id, owner_name, permission, shares, generation)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT (id) DO UPDATE SET name = excluded.name, description = excluded.description,
                is_ordered = excluded.is_ordered, default_sort = excluded.default_sort, owner_id = excluded.owner_id,
                owner_name = excluded.owner_name, permission = excluded.permission, shares = excluded.shares,
                generation = excluded.generation
            """, arguments: [list.id, list.name, list.description, list.isOrdered, list.defaultSort.rawValue, list.owner.id,
                             list.owner.username, list.permission.rawValue, sharesJSON, generation])
        try db.execute(sql: "DELETE FROM list_books WHERE list_id = ?", arguments: [list.id])
        for (position, book) in list.books.enumerated() {
            try db.execute(sql: "INSERT INTO list_books (list_id, position, book_id, sort_order, added_at) VALUES (?, ?, ?, ?, ?)",
                           arguments: [list.id, position, book.bookId, book.sortOrder, timestamp(book.addedAt) ?? 0])
        }
    }

    /// The Book's parts this file adds: its description is set with the
    /// Book, and here are its genres, tags, narrators, and Files' details.
    static func upsertCatalogParts(_ book: Shishoapp.FeedBook, in db: Database) throws {
        for table in ["book_genres", "book_tags"] {
            try db.execute(sql: "DELETE FROM \(table) WHERE book_id = ?", arguments: [book.id])
        }
        for (position, genre) in book.genres.enumerated() {
            try db.execute(sql: "INSERT INTO book_genres (book_id, position, genre_id, name) VALUES (?, ?, ?, ?)",
                           arguments: [book.id, position, genre.genreId, genre.name])
        }
        for (position, tag) in book.tags.enumerated() {
            try db.execute(sql: "INSERT INTO book_tags (book_id, position, tag_id, name) VALUES (?, ?, ?, ?)",
                           arguments: [book.id, position, tag.tagId, tag.name])
        }
        for file in book.files {
            let identifiers = file.identifiers.map { ["type": $0.type, "value": $0.value] }
            let identifiersJSON = (try? String(data: JSONEncoder().encode(identifiers), encoding: .utf8)) ?? "[]"
            try db.execute(sql: """
                UPDATE book_files SET publisher_id = ?, publisher = ?, identifiers = ?, pages = ?, codec = ?, bitrate = ?,
                    language = ?, abridged = ?, url = ?, reading_direction = ?, filepath = ?, file_modified_at = ?, created_at = ?
                WHERE id = ?
                """, arguments: [file.publisher?.publisherId, file.publisher?.name, identifiersJSON, namesJSON(file.pages),
                                 file.audiobookCodec, file.audiobookBitrateBps, file.language, file.abridged, file.url,
                                 file.readingDirection?.rawValue, file.filepath, file.fileModifiedAt.flatMap(timestamp),
                                 timestamp(file.createdAt), file.id])
            try db.execute(sql: "DELETE FROM book_narrators WHERE file_id = ?", arguments: [file.id])
            for (position, narrator) in file.narrators.enumerated() {
                try db.execute(sql: "INSERT INTO book_narrators (file_id, position, book_id, person_id, name) VALUES (?, ?, ?, ?, ?)",
                               arguments: [file.id, position, book.id, narrator.personId, narrator.name])
            }
        }
    }

    /// Drops the rows a finished download from the start didn't send.
    static func dropOlderCatalog(before generation: Int, in db: Database) throws {
        for table in ["series", "persons", "genres", "tags", "publishers", "lists", "user_books"] {
            try db.execute(sql: "DELETE FROM \(table) WHERE generation < ?", arguments: [generation])
        }
    }

    // MARK: - Search index

    /// Rewrites the search rows of what entries touched, as the server's
    /// own index holds them (`pkg/search/service.go`): a Book's authors,
    /// narrators, and Series with their aliases; a Series' name with its
    /// aliases, its Books' titles and authors; a Person's name with aliases.
    static func reindex(_ touched: Touched, in db: Database) throws {
        var touched = touched
        if !touched.books.isEmpty {
            // A Book's title and authors are in its Series' rows.
            let books = touched.books.map(String.init).joined(separator: ",")
            try touched.series.formUnion(Int.fetchAll(db, sql: "SELECT series_id FROM book_series WHERE book_id IN (\(books))"))
        }
        try rewrite("books_fts", "rowid, book_id, library_id, title, subtitle, authors, narrators, series_names", ids: touched.books, in: db, select: """
            SELECT b.id, b.id, b.library_id, b.title, COALESCE(b.subtitle, ''),
                COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (
                    SELECT DISTINCT a.name FROM book_authors a WHERE a.book_id = b.id
                    UNION SELECT DISTINCT j.value FROM book_authors a JOIN persons p ON p.id = a.person_id, json_each(p.aliases) j WHERE a.book_id = b.id
                )), ''),
                COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (
                    SELECT DISTINCT n.name FROM book_narrators n WHERE n.book_id = b.id
                    UNION SELECT DISTINCT j.value FROM book_narrators n JOIN persons p ON p.id = n.person_id, json_each(p.aliases) j WHERE n.book_id = b.id
                )), ''),
                COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (
                    SELECT bs.name FROM book_series bs WHERE bs.book_id = b.id
                    UNION SELECT j.value FROM book_series bs JOIN series s ON s.id = bs.series_id, json_each(s.aliases) j WHERE bs.book_id = b.id
                )), '')
            FROM books b WHERE b.id IN
            """)
        try rewrite("series_fts", "rowid, series_id, library_id, name, description, book_titles, book_authors", ids: touched.series, in: db, select: """
            SELECT s.id, s.id, s.library_id,
                s.name || COALESCE(' ' || (SELECT GROUP_CONCAT(j.value, ' ') FROM json_each(s.aliases) j), ''),
                COALESCE(s.description, ''),
                COALESCE((SELECT GROUP_CONCAT(b.title, ' ') FROM book_series bs JOIN books b ON b.id = bs.book_id WHERE bs.series_id = s.id), ''),
                COALESCE((SELECT GROUP_CONCAT(name, ' ') FROM (SELECT DISTINCT a.name FROM book_series bs JOIN book_authors a ON a.book_id = bs.book_id WHERE bs.series_id = s.id)), '')
            FROM series s WHERE s.id IN
            """)
        try rewrite("persons_fts", "rowid, person_id, library_id, name, sort_name", ids: touched.persons, in: db, select: """
            SELECT p.id, p.id, p.library_id,
                p.name || COALESCE(' ' || (SELECT GROUP_CONCAT(j.value, ' ') FROM json_each(p.aliases) j), ''), p.sort_name
            FROM persons p WHERE p.id IN
            """)
        for (table, idColumn, ids) in [("genres", "genre_id", touched.genres), ("tags", "tag_id", touched.tags)] {
            try rewrite("\(table)_fts", "rowid, \(idColumn), library_id, name", ids: ids, in: db, select: """
                SELECT r.id, r.id, r.library_id,
                    r.name || COALESCE(' ' || (SELECT GROUP_CONCAT(j.value, ' ') FROM json_each(r.aliases) j), '')
                FROM \(table) r WHERE r.id IN
                """)
        }
    }

    /// Drops the search rows of records a download from the start dropped.
    /// Each page indexed what it brought, so nothing else needs writing.
    static func pruneSearch(in db: Database) throws {
        for (index, table) in [("books_fts", "books"), ("series_fts", "series"), ("persons_fts", "persons"),
                               ("genres_fts", "genres"), ("tags_fts", "tags")] {
            try db.execute(sql: "DELETE FROM \(index) WHERE rowid NOT IN (SELECT id FROM \(table))")
        }
    }

    /// Deletes the rows of ids from an index, then inserts them again from
    /// select, which gives columns in order and ends with "WHERE <id> IN".
    private static func rewrite(_ table: String, _ columns: String, ids: Set<Int>, in db: Database, select: String) throws {
        guard !ids.isEmpty else { return }
        let list = ids.map(String.init).joined(separator: ",")
        try db.execute(sql: "DELETE FROM \(table) WHERE rowid IN (\(list))")
        try db.execute(sql: "INSERT INTO \(table) (\(columns)) \(select) (\(list))")
    }

    // MARK: - Access

    /// Brings what's on the device in line with what the user can access
    /// now (ADR 0023): a Library or List they lost goes, and one they gained
    /// is marked to fetch whole. A Library gained or lost changes which of
    /// its Books each List holds for the user, so every List is fetched
    /// again too. Until a download from the start has recorded the access
    /// it began with, there's nothing to compare against. force applies it
    /// even then, as when the server refuses the user every Book.
    ///
    /// fromFeed says access came with a feed page, whose behind list names
    /// the Libraries with records before its cursor: a gained Library not
    /// listed, such as one just made, arrives in the feed and isn't fetched
    /// again. Access from a whole fetch says nothing about that.
    func reconcileAccess(_ access: Shishoapp.FeedAccess, fromFeed: Bool = false, force: Bool = false) throws {
        try pool.write { db in
            let known = try Bool.fetchOne(db, sql: "SELECT access_known FROM sync_state WHERE id = 1") ?? false
            guard known || force else { return }
            if force, access.libraryIds.isEmpty, access.listIds.isEmpty {
                // Nothing is the user's to see: everything from the catalog
                // goes, even rows whose Library hasn't arrived yet.
                try CatalogStore.dropCatalog(in: db)
                return
            }
            // A gain is measured against the access recorded, never the rows
            // on hand: a Library renamed just after it was granted arrives in
            // the feed with its Books still behind the cursor. Rows on hand
            // only say what else a loss has to drop.
            let libraries = Set(access.libraryIds)
            let recorded = Set(try Int.fetchAll(db, sql: "SELECT library_id FROM library_access"))
            let had = recorded.union(try Int.fetchAll(db, sql: "SELECT id FROM libraries"))
            let lostLibraries = had.subtracting(libraries)
            for lost in lostLibraries {
                try CatalogStore.dropLibrary(lost, in: db)
            }
            let gained = libraries.subtracting(recorded)
            let behind = Set(access.behindLibraryIds)
            for id in gained {
                let complete = fromFeed && !behind.contains(id)
                try db.execute(sql: "INSERT OR REPLACE INTO library_access (library_id, cursor, complete) VALUES (?, 0, ?)",
                               arguments: [id, complete])
            }
            let lists = Set(access.listIds)
            let recordedLists = Set(try Int.fetchAll(db, sql: "SELECT list_id FROM list_access"))
            let hadLists = recordedLists.union(try Int.fetchAll(db, sql: "SELECT id FROM lists"))
            for lost in hadLists.subtracting(lists) {
                try CatalogStore.dropList(lost, in: db)
            }
            if !gained.isEmpty || !lostLibraries.isEmpty {
                try db.execute(sql: "UPDATE list_access SET complete = 0")
            }
            for id in lists.subtracting(recordedLists) {
                try db.execute(sql: "INSERT OR REPLACE INTO list_access (list_id, complete) VALUES (?, 0)", arguments: [id])
            }
        }
    }

    /// Records the access a download from the start began with, on its first
    /// page: it brings every Library and List listed. One gained or lost
    /// while it runs is answered when it ends (reconcileAccess), since a
    /// Library gained midway has records behind the cursor.
    static func setAccess(_ access: Shishoapp.FeedAccess, in db: Database) throws {
        try db.execute(sql: "DELETE FROM library_access")
        try db.execute(sql: "DELETE FROM list_access")
        for id in access.libraryIds {
            try db.execute(sql: "INSERT INTO library_access (library_id, cursor, complete) VALUES (?, 0, 1)", arguments: [id])
        }
        for id in access.listIds {
            try db.execute(sql: "INSERT INTO list_access (list_id, complete) VALUES (?, 1)", arguments: [id])
        }
        // What the copy holds that the user can't access goes now.
        let keep = Set(access.libraryIds)
        for id in try Int.fetchAll(db, sql: "SELECT id FROM libraries") where !keep.contains(id) {
            try dropLibrary(id, in: db)
        }
        let keepLists = Set(access.listIds)
        for id in try Int.fetchAll(db, sql: "SELECT id FROM lists") where !keepLists.contains(id) {
            try dropList(id, in: db)
        }
        try db.execute(sql: "UPDATE sync_state SET access_known = 1 WHERE id = 1")
    }

    /// The Libraries and Lists gained since, still to fetch whole, with the
    /// cursor each Library's fetch has reached.
    func pendingAccess() throws -> (libraries: [(id: Int, cursor: Int)], lists: [Int]) {
        try pool.read { db in
            let libraries = try Row.fetchAll(db, sql: "SELECT library_id, cursor FROM library_access WHERE NOT complete ORDER BY library_id")
                .map { (id: $0["library_id"] as Int, cursor: $0["cursor"] as Int) }
            let lists = try Int.fetchAll(db, sql: "SELECT list_id FROM list_access WHERE NOT complete ORDER BY list_id")
            return (libraries, lists)
        }
    }

    /// Applies one page of a Library's or List's whole fetch, without
    /// moving the feed's cursor, and records how far it got.
    func applyScoped(_ page: Shishoapp.FeedResponse, libraryID: Int? = nil, listID: Int? = nil, ownDeviceID: String = "") throws {
        try pool.write { db in
            let generation = try CatalogStore.syncState(db).generation
            var touched = Touched()
            for entry in page.entries where !entry.deleted {
                try CatalogStore.applyEntry(entry, generation: generation, ownDeviceID: ownDeviceID, touched: &touched, in: db)
            }
            try CatalogStore.reindex(touched, in: db)
            if let libraryID {
                try db.execute(sql: "UPDATE library_access SET cursor = ?, complete = ? WHERE library_id = ?",
                               arguments: [page.cursor, !page.hasMore, libraryID])
            }
            if let listID {
                try db.execute(sql: "UPDATE list_access SET complete = ? WHERE list_id = ?", arguments: [!page.hasMore, listID])
            }
        }
    }

    /// Forgets a Library the user can't access anymore and everything in it.
    static func dropLibrary(_ id: Int, in db: Database) throws {
        let books = try Int.fetchAll(db, sql: "SELECT id FROM books WHERE library_id = ?", arguments: [id])
        try db.execute(sql: "DELETE FROM books WHERE library_id = ?", arguments: [id])
        for table in ["series", "persons", "genres", "tags", "publishers"] {
            try db.execute(sql: "DELETE FROM \(table) WHERE library_id = ?", arguments: [id])
        }
        for index in ["series_fts", "persons_fts", "genres_fts", "tags_fts"] {
            try db.execute(sql: "DELETE FROM \(index) WHERE library_id = ?", arguments: [id])
        }
        if !books.isEmpty {
            try db.execute(sql: "DELETE FROM books_fts WHERE rowid IN (\(books.map(String.init).joined(separator: ",")))")
        }
        try db.execute(sql: "DELETE FROM libraries WHERE id = ?", arguments: [id])
        try db.execute(sql: "DELETE FROM library_access WHERE library_id = ?", arguments: [id])
    }

    /// Forgets the whole catalog, keeping the journal and this device's own
    /// places, for when the user can't read Books at all.
    static func dropCatalog(in db: Database) throws {
        for table in ["books", "libraries", "series", "persons", "genres", "tags", "publishers", "lists", "user_books",
                      "library_access", "list_access", "books_fts", "series_fts", "persons_fts", "genres_fts", "tags_fts"] {
            try db.execute(sql: "DELETE FROM \(table)")
        }
    }

    /// Forgets a List deleted, or no longer shared with the user.
    static func dropList(_ id: Int, in db: Database) throws {
        try db.execute(sql: "DELETE FROM lists WHERE id = ?", arguments: [id])
        try db.execute(sql: "DELETE FROM list_access WHERE list_id = ?", arguments: [id])
    }

    // MARK: - Reading

    /// The SQL for the user's Reading Status of the Book in bookColumn, as
    /// the server derives it (`reading.StatusExpr`): a Read-through
    /// underway, else To Read, else Reference, else the latest ended one.
    static func statusExpression(_ bookColumn: String) -> String {
        """
        (CASE
            WHEN EXISTS (SELECT 1 FROM read_throughs rts WHERE rts.book_id = \(bookColumn) AND rts.outcome = 'underway' AND NOT rts.paused) THEN 'currently_reading'
            WHEN EXISTS (SELECT 1 FROM read_throughs rts WHERE rts.book_id = \(bookColumn) AND rts.outcome = 'underway' AND rts.paused) THEN 'paused'
            WHEN EXISTS (SELECT 1 FROM user_books ubs WHERE ubs.book_id = \(bookColumn) AND ubs.to_read_position IS NOT NULL) THEN 'to_read'
            WHEN EXISTS (SELECT 1 FROM user_books ubs WHERE ubs.book_id = \(bookColumn) AND ubs.reference) THEN 'reference'
            ELSE (
                SELECT CASE rts.outcome WHEN 'finished' THEN 'read' WHEN 'abandoned' THEN 'did_not_finish' END
                FROM read_throughs rts
                WHERE rts.book_id = \(bookColumn) AND rts.outcome != 'underway'
                ORDER BY rts.end_day IS NULL, rts.end_day DESC, rts.id DESC
                LIMIT 1
            )
        END)
        """
    }

    static func standing(_ db: Database, bookID: Int) throws -> BookStanding {
        let row = try Row.fetchOne(db, sql: """
            WITH book(id) AS (SELECT ?)
            SELECT \(statusExpression("book.id")) AS status, (SELECT rating FROM user_books WHERE book_id = book.id) AS rating FROM book
            """, arguments: [bookID])
        return BookStanding(status: row?["status"], rating: row?["rating"])
    }

    /// The Series, Persons, Genres, or Tags of libraryID, or of every
    /// Library, by sort name, each with how many Books name it.
    static func resources(_ db: Database, kind: ResourceKind, libraryID: Int?, ids: [Int]? = nil) throws -> [ResourceSummary] {
        // Each kind's Books, in the order its covers show, as rid, book_id.
        let books: String
        switch kind {
        case .series:
            books = "SELECT series_id AS rid, book_id, COALESCE(number, 1e9) AS ord FROM book_series"
        case .person:
            books = "SELECT person_id AS rid, book_id, 0 AS ord FROM book_authors UNION SELECT person_id, book_id, 1 FROM book_narrators"
        case .genre:
            books = "SELECT genre_id AS rid, book_id, 0 AS ord FROM book_genres"
        case .tag:
            books = "SELECT tag_id AS rid, book_id, 0 AS ord FROM book_tags"
        }
        var sql = """
            SELECT r.id, r.name, r.sort_name, r.aliases,
                (SELECT COUNT(DISTINCT j.book_id) FROM (\(books)) j WHERE j.rid = r.id) AS book_count,
                \(kind == .person ? "(SELECT COUNT(DISTINCT book_id) FROM book_authors WHERE person_id = r.id)" : "0") AS authored,
                \(kind == .person ? "(SELECT COUNT(DISTINCT file_id) FROM book_narrators WHERE person_id = r.id)" : "0") AS narrated
            FROM \(kind.table) r
            """
        var arguments: StatementArguments = []
        var conditions: [String] = []
        if let libraryID {
            conditions.append("r.library_id = ?")
            arguments = [libraryID]
        }
        if let ids {
            conditions.append("r.id IN (\(ids.isEmpty ? "NULL" : ids.map(String.init).joined(separator: ",")))")
        }
        if !conditions.isEmpty {
            sql += " WHERE " + conditions.joined(separator: " AND ")
        }
        sql += " ORDER BY r.sort_name COLLATE NOCASE, r.id"
        var items = try Row.fetchAll(db, sql: sql, arguments: arguments).map { row -> ResourceSummary in
            let name: String = row["name"]
            let sortName: String = row["sort_name"]
            var parts = kind == .person && sortName != name ? [sortName] : []
            parts += names(row["aliases"])
            return ResourceSummary(id: row["id"], name: name, bookCount: row["book_count"], detail: parts.joined(separator: ", "),
                                   authoredCount: row["authored"], narratedCount: row["narrated"])
        }
        // A resource no Book names yet, before its Books arrive, isn't
        // worth a row.
        .filter { $0.bookCount > 0 }

        // Up to three covers each, in one query.
        guard !items.isEmpty else { return items }
        let listed = items.map { String($0.id) }.joined(separator: ",")
        let previewRows = try Row.fetchAll(db, sql: """
            SELECT rid, book_id FROM (
                SELECT j.rid, j.book_id, ROW_NUMBER() OVER (PARTITION BY j.rid ORDER BY MIN(j.ord), j.book_id) AS n
                FROM (\(books)) j JOIN books b ON b.id = j.book_id
                WHERE j.rid IN (\(listed))
                GROUP BY j.rid, j.book_id
            ) WHERE n <= 3 ORDER BY rid, n
            """)
        let bookIDs = Set(previewRows.map { $0["book_id"] as Int })
        let summaries = try summaries(db, rows: Row.fetchAll(db, sql: summarySQL + " WHERE b.id IN (\(bookIDs.map(String.init).joined(separator: ",")))"))
        let byID = Dictionary(uniqueKeysWithValues: summaries.map { ($0.id, $0) })
        var previews: [Int: [BookSummary]] = [:]
        for row in previewRows {
            if let book = byID[row["book_id"]] {
                previews[row["rid"], default: []].append(book)
            }
        }
        for index in items.indices {
            items[index].previews = previews[items[index].id] ?? []
        }
        return items
    }

    static func resource(_ db: Database, kind: ResourceKind, id: Int) throws -> ResourceDetail? {
        guard let row = try Row.fetchOne(db, sql: "SELECT name, description, aliases FROM \(kind.table) WHERE id = ?", arguments: [id]) else {
            return nil
        }
        return ResourceDetail(kind: kind, id: id, name: row["name"], description: row["description"], aliases: names(row["aliases"]))
    }

    /// The Books naming a resource: a Series' in its order, a Person's
    /// authored ones (narrated is false) or narrated ones, and a Genre's or
    /// Tag's in the Library's sort.
    static func books(_ db: Database, kind: ResourceKind, id: Int, narrated: Bool = false) throws -> [BookSummary] {
        let filter: String
        var order = LibrarySort.orderBy("")
        switch kind {
        case .series:
            filter = "b.id IN (SELECT book_id FROM book_series WHERE series_id = ?)"
            let membership = { (value: String) in
                "(SELECT \(value) FROM book_series WHERE book_id = b.id AND series_id = \(id) ORDER BY position LIMIT 1)"
            }
            order = "\(membership("number")) IS NULL, \(membership("number")), \(membership("COALESCE(number_end, number)")), b.sort_title, b.id"
        case .person:
            filter = narrated
                ? "b.id IN (SELECT book_id FROM book_narrators WHERE person_id = ?)"
                : "b.id IN (SELECT book_id FROM book_authors WHERE person_id = ?)"
        case .genre:
            filter = "b.id IN (SELECT book_id FROM book_genres WHERE genre_id = ?)"
        case .tag:
            filter = "b.id IN (SELECT book_id FROM book_tags WHERE tag_id = ?)"
        }
        let rows = try Row.fetchAll(db, sql: summarySQL + " WHERE \(filter) ORDER BY \(order)", arguments: [id])
        return try summaries(db, rows: rows)
    }

    /// The Series Number each Book has in seriesID, for the Series page's
    /// badges.
    static func seriesNumbers(_ db: Database, seriesID: Int) throws -> [Int: (Double, Double?)] {
        var out: [Int: (Double, Double?)] = [:]
        for row in try Row.fetchAll(db, sql: "SELECT book_id, number, number_end FROM book_series WHERE series_id = ? AND number IS NOT NULL", arguments: [seriesID]) {
            out[row["book_id"]] = (row["number"], row["number_end"])
        }
        return out
    }

    static func lists(_ db: Database) throws -> [ListSummary] {
        try Row.fetchAll(db, sql: """
            SELECT l.id, l.name, l.description, l.permission, l.owner_name, l.shares,
                (SELECT COUNT(*) FROM list_books lb JOIN books b ON b.id = lb.book_id WHERE lb.list_id = l.id) AS book_count
            FROM lists l ORDER BY l.permission != 'owner', l.name COLLATE NOCASE, l.id
            """).map { row in
            let id: Int = row["id"]
            let covers = try listBooks(db, listID: id, limit: 3)
            let shares = ((row["shares"] as String?)?.data(using: .utf8))
                .flatMap { try? JSONDecoder().decode([ListShare].self, from: $0) } ?? []
            return ListSummary(id: id, name: row["name"], description: row["description"], permission: row["permission"],
                               ownerName: row["owner_name"], bookCount: row["book_count"], covers: covers, shares: shares)
        }
    }

    static func list(_ db: Database, id: Int) throws -> ListSummary? {
        try lists(db).first { $0.id == id }
    }

    /// A List's Books the device has, in the List's default sort, as the
    /// web app shows them.
    static func listBooks(_ db: Database, listID: Int, limit: Int? = nil) throws -> [BookSummary] {
        let sort = try String.fetchOne(db, sql: "SELECT default_sort FROM lists WHERE id = ?", arguments: [listID]) ?? ""
        let order: String
        switch sort {
        case "manual": order = "lb.sort_order IS NULL, lb.sort_order, lb.added_at DESC"
        case "added_at_asc": order = "lb.added_at ASC"
        case "title_asc": order = "b.sort_title COLLATE NOCASE ASC"
        case "title_desc": order = "b.sort_title COLLATE NOCASE DESC"
        case "author_asc", "author_desc":
            let author = "(SELECT sort_name FROM book_authors WHERE book_id = b.id ORDER BY position LIMIT 1)"
            order = "\(author) IS NULL, \(author) COLLATE NOCASE \(sort.hasSuffix("asc") ? "ASC" : "DESC")"
        case "rating_asc", "rating_desc":
            let rating = "(SELECT rating FROM user_books WHERE book_id = b.id)"
            order = "\(rating) IS NULL, \(rating) \(sort.hasSuffix("asc") ? "ASC" : "DESC")"
        case "date_finished_asc", "date_finished_desc":
            let finished = "(SELECT MAX(end_day) FROM read_throughs WHERE book_id = b.id AND outcome = 'finished')"
            order = "\(finished) IS NULL, \(finished) \(sort.hasSuffix("asc") ? "ASC" : "DESC")"
        case "reading_status_asc", "reading_status_desc":
            order = "\(LibrarySort.statusRank("b.id")) \(sort.hasSuffix("asc") ? "ASC" : "DESC")"
        default: order = "lb.added_at DESC"
        }
        var sql = summarySQL + " JOIN list_books lb ON lb.book_id = b.id AND lb.list_id = ? ORDER BY \(order), lb.position"
        if let limit {
            sql += " LIMIT \(limit)"
        }
        return try summaries(db, rows: Row.fetchAll(db, sql: sql, arguments: [listID]))
    }

    // MARK: - Search

    /// The FTS5 query for what the user typed, made as the server makes it
    /// (`search.BuildPrefixQuery`): the whole input as one phrase, so
    /// operators are literal, matching words that start with its last word.
    static func searchQuery(_ input: String) -> String? {
        var text = input.trimmingCharacters(in: .whitespacesAndNewlines)
        if text.utf8.count > 100 {
            text = String(decoding: text.utf8.prefix(100), as: UTF8.self)
        }
        guard !text.isEmpty else { return nil }
        return "\"" + text.replacingOccurrences(of: "\"", with: "\"\"") + "\"*"
    }

    /// Searches the device's copy as the web's global search does: Books
    /// whose identifier is exactly what was typed first, then the index in
    /// the server's ranking, and Series, Persons, Genres, and Tags.
    static func search(_ db: Database, text: String, libraryID: Int?, limit: Int = 50) throws -> SearchResults {
        guard let query = searchQuery(text) else { return SearchResults() }
        var results = SearchResults()
        let library = libraryID.map { " AND f.library_id = \($0)" } ?? ""

        var bookIDs: [Int] = []
        // An identifier is one word, or an ISBN typed with spaces, so other
        // searches skip the scan of every File's identifiers.
        var trimmed = text.trimmingCharacters(in: .whitespaces)
        for prefix in ["ISBN:", "ISBN"] where trimmed.uppercased().hasPrefix(prefix) {
            trimmed = String(trimmed.dropFirst(prefix.count)).trimmingCharacters(in: .whitespaces)
        }
        let oneWord = !trimmed.contains(" ") || trimmed.allSatisfy { $0.isNumber || " -xX".contains($0) }
        let candidates = oneWord ? identifierForms(text) : []
        if !candidates.isEmpty {
            let marks = candidates.map { _ in "?" }.joined(separator: ",")
            bookIDs = try Int.fetchAll(db, sql: """
                SELECT DISTINCT f.book_id FROM book_files f, json_each(f.identifiers) j
                JOIN books b ON b.id = f.book_id
                WHERE json_extract(j.value, '$.value') IN (\(marks))\(libraryID.map { " AND b.library_id = \($0)" } ?? "")
                LIMIT \(limit)
                """, arguments: StatementArguments(candidates))
        }
        for id in try Int.fetchAll(db, sql: "SELECT f.book_id FROM books_fts f WHERE books_fts MATCH ?\(library) ORDER BY f.rank LIMIT \(limit)",
                                   arguments: [query]) where !bookIDs.contains(id) {
            bookIDs.append(id)
        }
        if !bookIDs.isEmpty {
            let rows = try Row.fetchAll(db, sql: summarySQL + " WHERE b.id IN (\(bookIDs.map(String.init).joined(separator: ",")))")
            let byID = Dictionary(uniqueKeysWithValues: try summaries(db, rows: rows).map { ($0.id, $0) })
            results.books = bookIDs.compactMap { byID[$0] }
        }

        let resource = { (kind: ResourceKind, fts: String, idColumn: String) throws -> [ResourceSummary] in
            let ids = try Int.fetchAll(db, sql: "SELECT f.\(idColumn) FROM \(fts) f WHERE \(fts) MATCH ?\(library) ORDER BY f.rank LIMIT 20",
                                       arguments: [query])
            guard !ids.isEmpty else { return [] }
            let all = Dictionary(uniqueKeysWithValues: try resources(db, kind: kind, libraryID: libraryID, ids: ids).map { ($0.id, $0) })
            return ids.compactMap { all[$0] }
        }
        results.series = try resource(.series, "series_fts", "series_id")
        results.persons = try resource(.person, "persons_fts", "person_id")
        results.genres = try resource(.genre, "genres_fts", "genre_id")
        results.tags = try resource(.tag, "tags_fts", "tag_id")
        return results
    }

    /// The forms an identifier is stored in that text could be, as the
    /// server's `identifiers.CandidateForms` makes them: as typed, an ISBN
    /// without hyphens or spaces, an ASIN in capitals, and a UUID without
    /// its urn prefix.
    static func identifierForms(_ text: String) -> [String] {
        let value = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !value.isEmpty else { return [] }
        var forms = [value]
        func add(_ form: String) {
            if !form.isEmpty, !forms.contains(form) { forms.append(form) }
        }
        var isbn = value.uppercased()
        for prefix in ["ISBN:", "ISBN"] where isbn.hasPrefix(prefix) {
            isbn = String(isbn.dropFirst(prefix.count))
        }
        isbn = isbn.filter { $0 != "-" && $0 != " " }
        if (isbn.count == 10 || isbn.count == 13), isbn.allSatisfy({ $0.isNumber || $0 == "X" }) {
            add(isbn)
        }
        let upper = value.uppercased()
        if upper.count == 10, upper.allSatisfy({ $0.isLetter || $0.isNumber }) {
            add(upper)
        }
        var lower = value.lowercased()
        if lower.hasPrefix("urn:uuid:") {
            lower = String(lower.dropFirst("urn:uuid:".count))
        }
        if UUID(uuidString: lower) != nil {
            add(lower)
        }
        return forms
    }

    // MARK: - Summaries

    /// The columns a BookSummary reads, for `books b`.
    static let summarySQL = """
        SELECT b.id, b.library_id, b.title, b.authors, b.cover_version,
            COALESCE(l.cover_aspect_ratio, 'book') AS aspect, \(readingColumns)
        FROM books b
        LEFT JOIN libraries l ON l.id = b.library_id
        """

    static func summaries(_ db: Database, rows: [Row]) throws -> [BookSummary] {
        guard !rows.isEmpty else { return [] }
        let ids = rows.map { String($0["id"] as Int) }.joined(separator: ",")
        var fileTypes: [Int: [String]] = [:]
        for row in try Row.fetchAll(db, sql: """
            SELECT book_id, file_type FROM book_files WHERE book_id IN (\(ids)) AND file_role != 'supplement' ORDER BY book_id, position
            """) {
            let bookID: Int = row["book_id"]
            let fileType: String = row["file_type"]
            if !(fileTypes[bookID] ?? []).contains(fileType) {
                fileTypes[bookID, default: []].append(fileType)
            }
        }
        return rows.map {
            BookSummary(id: $0["id"], libraryID: $0["library_id"], title: $0["title"], authors: $0["authors"],
                        coverVersion: $0["cover_version"], coverAspectRatio: $0["aspect"], fileTypes: fileTypes[$0["id"]] ?? [],
                        status: $0["status"], rating: $0["rating"], progress: $0["progress"])
        }
    }

    // MARK: - Covers

    /// Every cover the Library grid shows, for downloading them all ahead
    /// of going offline.
    func coverVersions() throws -> [(bookID: Int, version: String)] {
        try pool.read { db in
            try Row.fetchAll(db, sql: "SELECT id, cover_version FROM books WHERE cover_version != '' ORDER BY id")
                .map { (bookID: $0["id"], version: $0["cover_version"]) }
        }
    }
}
