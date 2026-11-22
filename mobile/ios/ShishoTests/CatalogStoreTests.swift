import ShishoAPI
import XCTest
@testable import Shisho

final class CatalogStoreTests: XCTestCase {
    private var store: CatalogStore!

    override func setUpWithError() throws {
        store = try CatalogStore(path: Self.freshPath())
    }

    private static func freshPath() -> String {
        FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path
    }

    private func page(_ json: String) throws -> Shishoapp.FeedResponse {
        try makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data(json.utf8))
    }

    private func book(_ id: Int, _ title: String, library: Int = 1, added: String = "2026-01-01T00:00:00Z") -> String {
        """
        {"kind":"book","id":\(id),"library_id":\(library),"book":{"id":\(id),"library_id":\(library),"title":"\(title)","sort_title":"\(title)","created_at":"\(added)",
         "authors":[{"person_id":1,"name":"Ann Author","sort_name":"Author, Ann"}],"files":[{"id":\(id * 10),"file_type":"epub","file_role":"main","size_bytes":10,"display_name":"\(title).epub"}],"cover_version":"\(id)-1"}}
        """
    }

    private let library = #"{"kind":"library","id":1,"library_id":1,"library":{"id":1,"name":"Fiction","cover_aspect_ratio":"book"}}"#

    func testAppliesCreatesUpdatesAndDeletes() throws {
        try store.startOverIfNeeded()
        try store.apply(page(#"{"entries":[\#(library),\#(book(1, "Dune")),\#(book(2, "Emma"))],"cursor":3}"#))
        XCTAssertEqual(try store.books(libraryID: nil).map(\.title), ["Dune", "Emma"])
        XCTAssertEqual(try store.syncState().cursor, 3)

        try store.apply(page(#"{"entries":[\#(book(1, "Anathem")),{"kind":"book","id":2,"deleted":true,"library_id":1}],"cursor":5}"#))
        XCTAssertEqual(try store.books(libraryID: nil).map(\.title), ["Anathem"])
        let detail = try XCTUnwrap(store.book(id: 1))
        XCTAssertEqual(detail.authors.map(\.name), ["Ann Author"])
        XCTAssertEqual(detail.files.map(\.fileType), ["epub"])
        XCTAssertEqual(detail.files.map(\.displayName), ["Anathem.epub"])
        XCTAssertNil(try store.book(id: 2))
    }

    /// A File moved from Book 2 into Book 1 arrives in Book 1's document
    /// before Book 2's, which still lists it until its own entry comes.
    func testAFileMovedBetweenBooks() throws {
        try store.startOverIfNeeded()
        try store.apply(page(#"{"entries":[\#(library),\#(book(1, "Dune")),\#(book(2, "Emma"))],"cursor":3}"#))
        let moved = """
        {"kind":"book","id":1,"library_id":1,"book":{"id":1,"library_id":1,"title":"Dune","sort_title":"Dune",
         "files":[{"id":10,"file_type":"epub","file_role":"main","size_bytes":10},{"id":20,"file_type":"epub","file_role":"main","size_bytes":10}],"cover_version":"1-1"}}
        """
        try store.apply(page(#"{"entries":[\#(moved),{"kind":"book","id":2,"deleted":true,"library_id":1}],"cursor":5}"#))
        XCTAssertEqual(try store.book(id: 1)?.files.map(\.id), [10, 20])
        XCTAssertEqual(try store.syncState().cursor, 5)
    }

    /// How far the server's deletions had expired is kept with the cursor,
    /// in the same transaction, for the next request to send back.
    func testKeepsTheExpiryWithTheCursor() throws {
        try store.startOverIfNeeded()
        try store.apply(page(#"{"entries":[\#(library)],"cursor":2,"expired_through":7}"#))
        XCTAssertEqual(try store.syncState().cursor, 2)
        XCTAssertEqual(try store.syncState().expiredThrough, 7)
    }

    /// A page applies only on top of the cursor it was asked for. A sync that
    /// was stopped and whose answer came late can't rewind a newer one.
    func testAPageFromAnOlderCursorIsDropped() throws {
        try store.startOverIfNeeded()
        XCTAssertTrue(try store.apply(page(#"{"entries":[\#(library)],"cursor":2}"#), after: 0))
        XCTAssertTrue(try store.apply(page(#"{"entries":[\#(book(1, "Dune"))],"cursor":4}"#), after: 2))
        XCTAssertFalse(try store.apply(page(#"{"entries":[\#(book(2, "Emma"))],"cursor":3}"#), after: 2))
        XCTAssertEqual(try store.syncState().cursor, 4)
        XCTAssertEqual(try store.books(libraryID: nil).map(\.id), [1])
    }

    func testIgnoresKindsItDoesntKnow() throws {
        try store.startOverIfNeeded()
        try store.apply(page(#"{"entries":[{"kind":"bookmark","id":9,"bookmark":{"seconds":3}},\#(book(1, "Dune"))],"cursor":2}"#))
        XCTAssertEqual(try store.books(libraryID: nil).map(\.id), [1])
        XCTAssertEqual(try store.syncState().cursor, 2)
    }

    func testDeletedLibraryTakesItsBooks() throws {
        try store.startOverIfNeeded()
        try store.apply(page(#"{"entries":[\#(library),\#(book(1, "Dune"))],"cursor":2}"#))
        try store.apply(page(#"{"entries":[{"kind":"library","id":1,"deleted":true,"library_id":1}],"cursor":3}"#))
        XCTAssertEqual(try store.books(libraryID: nil), [])
        XCTAssertEqual(try store.libraries(), [])
    }

    func testStartingOverKeepsTheCopyUntilTheDownloadFinishes() throws {
        try store.startOverIfNeeded()
        try store.apply(page(#"{"entries":[\#(library),\#(book(1, "Dune")),\#(book(2, "Emma"))],"cursor":3}"#))
        XCTAssertFalse(try store.syncState().startingOver)

        try store.startOverIfNeeded(serverSaidSo: true)
        XCTAssertEqual(try store.syncState().cursor, 0)
        try store.apply(page(#"{"entries":[\#(library),\#(book(1, "Dune"))],"cursor":8,"has_more":true}"#))
        // Book 2 wasn't sent yet, and stays on screen meanwhile.
        XCTAssertEqual(try store.books(libraryID: nil).map(\.id), [1, 2])
        try store.apply(page(#"{"entries":[],"cursor":9}"#))
        // The download is done: book 2 wasn't in it, so it was deleted
        // while the cursor was too old to say so.
        XCTAssertEqual(try store.books(libraryID: nil).map(\.id), [1])
        XCTAssertFalse(try store.syncState().startingOver)
    }

    /// Newest first, as the web app's Library sorts by default, with ties
    /// in id order. Go writes fractional seconds only when there are any.
    func testBooksAreNewestFirst() throws {
        try store.startOverIfNeeded()
        try store.apply(page(#"""
        {"entries":[\#(library),
          \#(book(1, "Oldest", added: "2024-05-01T10:00:00Z")),
          \#(book(2, "Newest", added: "2026-02-03T04:05:06.5Z")),
          \#(book(3, "Same second, later", added: "2026-02-03T04:05:06.75Z")),
          \#(book(4, "Tie", added: "2025-01-01T00:00:00Z")),
          \#(book(5, "Tie too", added: "2025-01-01T00:00:00Z"))],"cursor":6}
        """#))
        XCTAssertEqual(try store.books(libraryID: nil).map(\.id), [3, 2, 4, 5, 1])
    }

    private func sortedLibrary(_ spec: String) -> String {
        #"{"kind":"library","id":2,"library_id":2,"library":{"id":2,"name":"Sorted","cover_aspect_ratio":"book","sort_spec":"\#(spec)"}}"#
    }

    /// A Book in Library 2 with what each sort reads. files are
    /// (id, release date, page count) in any order.
    private func sortable(_ id: Int, title: String, author: String? = nil, series: (String, Double?, Double?)? = nil,
                          files: [(Int, String?, Int?)] = [], added: String = "2026-01-01T00:00:00Z") -> String {
        let authors = author.map { #"[{"person_id":1,"name":"\#($0)","sort_name":"\#($0)"}]"# } ?? "[]"
        let seriesJSON = series.map { name, number, end in
            #"[{"series_id":1,"name":"\#(name)","sort_name":"\#(name)","number":\#(number.map { "\($0)" } ?? "null"),"number_end":\#(end.map { "\($0)" } ?? "null")}]"#
        } ?? "[]"
        let filesJSON = files.map { fileID, released, pages in
            #"{"id":\#(fileID),"file_type":"epub","file_role":"main","size_bytes":1,"release_date":\#(released.map { "\"\($0)\"" } ?? "null"),"page_count":\#(pages.map(String.init) ?? "null")}"#
        }.joined(separator: ",")
        return #"{"kind":"book","id":\#(id),"library_id":2,"book":{"id":\#(id),"library_id":2,"title":"\#(title)","sort_title":"\#(title)","created_at":"\#(added)","authors":\#(authors),"series":\#(seriesJSON),"files":[\#(filesJSON)],"cover_version":""}}"#
    }

    /// The order of books in a fresh copy whose Library has spec.
    private func order(_ spec: String, _ books: [String]) throws -> [Int] {
        store = try CatalogStore(path: Self.freshPath())
        try store.startOverIfNeeded()
        try store.apply(page(#"{"entries":[\#(sortedLibrary(spec)),\#(books.joined(separator: ","))],"cursor":99}"#))
        return try store.books(libraryID: 2).map(\.id)
    }

    /// A Library sorts by the user's saved sort, as the server does
    /// (pkg/sortspec/sql.go): missing values last in either direction, and
    /// the Book's id after every level.
    func testALibrarySortsByTheSavedSort() throws {
        // Title: byte order, as the server compares, so "Zed" before "apple".
        XCTAssertEqual(try order("title:asc", [sortable(1, title: "apple"), sortable(2, title: "Zed"), sortable(3, title: "Mid")]), [3, 2, 1])
        // Author: the first author's sort name; no author is last both ways.
        let byAuthor = [sortable(1, title: "a", author: "Brown"), sortable(2, title: "b"), sortable(3, title: "c", author: "Adams")]
        XCTAssertEqual(try order("author:desc", byAuthor), [1, 3, 2])
        XCTAssertEqual(try order("author:asc", byAuthor), [3, 1, 2])
        // Series: name, then singles before ranges, then number, then the
        // range's end.
        XCTAssertEqual(try order("series:asc", [
            sortable(1, title: "a", series: ("Dune", 2, nil)),
            sortable(2, title: "b", series: ("Dune", 1, 3)),
            sortable(3, title: "c", series: ("Dune", 1, nil)),
            sortable(4, title: "d", series: ("Alpha", 9, nil)),
            sortable(5, title: "e"),
        ]), [4, 3, 1, 2, 5])
        // Release date: the newest File's, or the newest File that has one.
        XCTAssertEqual(try order("date_released:desc", [
            sortable(1, title: "a", files: [(10, "1990-01-01T00:00:00Z", nil), (11, nil, nil)]),
            sortable(2, title: "b", files: [(20, "2000-01-01T00:00:00Z", nil), (21, "1980-01-01T00:00:00Z", nil)]),
            sortable(3, title: "c", files: [(30, nil, nil)]),
        ]), [1, 2, 3])
        // Several levels, then the id.
        XCTAssertEqual(try order("page_count:asc,title:desc", [
            sortable(1, title: "a", files: [(10, nil, 100)]),
            sortable(2, title: "b", files: [(20, nil, 100)]),
            sortable(3, title: "c", files: [(30, nil, 50)]),
            sortable(4, title: "c", files: [(40, nil, 50)]),
        ]), [3, 4, 2, 1])
    }

    /// All Libraries together sort by their saved sort when they share one,
    /// as a single Library always does, and newest first when they differ.
    func testAllLibrariesShareASortOnlyWhenTheyAgree() throws {
        let other = { (spec: String) in
            #"{"kind":"library","id":3,"library_id":3,"library":{"id":3,"name":"Other","cover_aspect_ratio":"book","sort_spec":"\#(spec)"}}"#
        }
        let books = [
            sortable(1, title: "b", added: "2024-01-01T00:00:00Z"),
            sortable(2, title: "a", added: "2026-01-01T00:00:00Z"),
            sortable(3, title: "c", added: "2025-01-01T00:00:00Z"),
        ]
        _ = try order("title:asc", books)
        XCTAssertEqual(try store.books(libraryID: nil).map(\.id), [2, 1, 3])
        try store.apply(page(#"{"entries":[\#(other("title:asc"))],"cursor":100}"#))
        XCTAssertEqual(try store.books(libraryID: nil).map(\.id), [2, 1, 3])
        try store.apply(page(#"{"entries":[\#(other("author:asc"))],"cursor":101}"#))
        XCTAssertEqual(try store.books(libraryID: nil).map(\.id), [2, 3, 1])
    }

    /// Levels the app doesn't know, such as one a newer server adds, are
    /// skipped; with none left, it's the default, newest first.
    func testASortTheAppCantApplyFallsBack() throws {
        let books = [
            sortable(1, title: "b", added: "2024-01-01T00:00:00Z"),
            sortable(2, title: "a", added: "2026-01-01T00:00:00Z"),
            sortable(3, title: "c", added: "2025-01-01T00:00:00Z"),
        ]
        XCTAssertEqual(try order("someday_field:desc,title:asc", books), [2, 1, 3])
        XCTAssertEqual(try order("someday_field:desc,other_field:asc", books), [2, 3, 1])
        XCTAssertEqual(try order("", books), [2, 3, 1])
    }

    /// An app that understands more of the feed downloads the catalog again.
    func testANewerFeedVersionStartsOver() throws {
        try store.startOverIfNeeded()
        try store.apply(page(#"{"entries":[\#(library)],"cursor":2}"#))
        try store.pool.write { db in
            try db.execute(sql: "UPDATE sync_state SET feed_version = ? WHERE id = 1", arguments: [CatalogStore.feedVersion - 1])
        }
        try store.startOverIfNeeded()
        XCTAssertEqual(try store.syncState().cursor, 0)
        XCTAssertTrue(try store.syncState().startingOver)
    }

    func testAFreshCopyStartsFromTheStart() throws {
        try store.startOverIfNeeded()
        let state = try store.syncState()
        XCTAssertEqual(state.cursor, 0)
        XCTAssertTrue(state.startingOver)
        // A second call while that first download hasn't started changes
        // nothing.
        try store.startOverIfNeeded()
        XCTAssertEqual(try store.syncState().generation, state.generation)
    }
}
