import GRDB
import ShishoAPI
import XCTest
@testable import Shisho

/// The Finished page a Reading Stats breakdown row opens: the finished
/// Read-throughs whose Book has that Genre, Tag, author, or Rating, matched
/// as the server's StatsFinished matches them, from the device's copy.
final class StatsFinishedTests: XCTestCase {
    private var store: CatalogStore!

    override func setUpWithError() throws {
        store = try CatalogStore(path: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path)
        try store.startOverIfNeeded()
        try store.apply(page("""
        {"entries":[{"kind":"library","id":1,"library_id":1,"library":{"id":1,"name":"Books","cover_aspect_ratio":"book"}},
         \(book(1, "Gaudy Night", authors: #"[{"person_id":1,"name":"Dorothy L. Sayers","sort_name":"Sayers, Dorothy L."}]"#,
                genres: ["Mystery"], tags: ["Oxford"])),
         \(book(2, "The Nine Tailors", authors: #"[{"person_id":1,"name":"dorothy l. sayers","sort_name":"Sayers, Dorothy L."}]"#,
                genres: ["mystery", "Classic"], tags: ["Bells"])),
         \(book(3, "Watchmen", authors: #"[{"person_id":2,"name":"Alan Moore","sort_name":"Moore, Alan","role":"writer"},{"person_id":3,"name":"Dave Gibbons","sort_name":"Gibbons, Dave","role":"penciller"}]"#,
                genres: ["Comics"], tags: [])),
         \(book(4, "Busman's Honeymoon", authors: #"[{"person_id":1,"name":"Dorothy L. Sayers","sort_name":"Sayers, Dorothy L."}]"#,
                genres: ["Mystery"], tags: ["Oxford"])),
         \(readThrough(10, book: 1, end: "2026-03-01")),
         \(readThrough(11, book: 1, end: "2026-08-15")),
         \(readThrough(12, book: 2, end: "2025-12-31")),
         \(readThrough(13, book: 3, end: "2026-01-01")),
         \(readThrough(14, book: 4, end: "2026-05-01", outcome: "did_not_finish")),
         \(readThrough(15, book: 4, end: nil, outcome: "underway")),
         {"kind":"read_through","id":16,"read_through":{"id":16,"book_id":null,"end_day":"2026-04-01","outcome":"finished","paused":false,
          "book_title":"Strong Poison","book_authors":["Dorothy L. Sayers"]}},
         {"kind":"user_book","id":1,"user_book":{"book_id":1,"rating":18,"reference":false}},
         {"kind":"user_book","id":2,"user_book":{"book_id":2,"rating":16,"reference":false}},
         {"kind":"user_book","id":3,"user_book":{"book_id":3,"rating":18,"reference":false}}],"cursor":2}
        """))
    }

    private func page(_ json: String) throws -> Shishoapp.FeedResponse {
        try makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data(json.utf8))
    }

    private func book(_ id: Int, _ title: String, authors: String, genres: [String], tags: [String]) -> String {
        let genres = genres.enumerated().map { #"{"genre_id":\#($0.offset + 1),"name":"\#($0.element)"}"# }.joined(separator: ",")
        let tags = tags.enumerated().map { #"{"tag_id":\#($0.offset + 1),"name":"\#($0.element)"}"# }.joined(separator: ",")
        return """
        {"kind":"book","id":\(id),"library_id":1,"book":{"id":\(id),"library_id":1,"title":"\(title)","sort_title":"\(title)",
         "created_at":"2026-01-01T00:00:00Z","cover_version":"","authors":\(authors),"genres":[\(genres)],"tags":[\(tags)],
         "files":[{"id":\(id * 10),"file_type":"epub","file_role":"main","size_bytes":10,"display_name":"\(title).epub"}]}}
        """
    }

    private func readThrough(_ id: Int, book: Int, end: String?, outcome: String = "finished") -> String {
        let end = end.map { #""\#($0)""# } ?? "null"
        return #"{"kind":"read_through","id":\#(id),"read_through":{"id":\#(id),"book_id":\#(book),"end_day":\#(end),"outcome":"\#(outcome)","paused":false}}"#
    }

    private func finished(_ filter: StatsFinishedFilter, year: Int? = nil) throws -> [Int] {
        try store.pool.read { try CatalogStore.statsFinished($0, year: year, filter: filter) }.map(\.id)
    }

    func testAGenreMatchesIgnoringCaseAcrossBooks() throws {
        XCTAssertEqual(try finished(.genre("Mystery")), [11, 10, 12], "each finish, the latest first, whatever the name's case")
        XCTAssertEqual(try finished(.genre("MYSTERY")), [11, 10, 12])
        XCTAssertEqual(try finished(.genre("Classic")), [12])
        XCTAssertEqual(try finished(.genre("Myst")), [], "a whole name, not part of one")
    }

    func testATagMatchesItsBooks() throws {
        XCTAssertEqual(try finished(.tag("oxford")), [11, 10], "a Did Not Finish or one underway isn't finished")
        XCTAssertEqual(try finished(.tag("Bells")), [12])
    }

    func testAnAuthorIsAnyOfTheBooksPeople() throws {
        XCTAssertEqual(try finished(.author("Dorothy L. Sayers")), [11, 10, 12], "not the detached Strong Poison")
        XCTAssertEqual(try finished(.author("Dave Gibbons")), [13], "whatever their role")
    }

    func testARatingIsTheBooksCurrentOneInStars() throws {
        XCTAssertEqual(try finished(.rating(4.5)), [11, 10, 13])
        XCTAssertEqual(try finished(.rating(4)), [12])
        XCTAssertEqual(try finished(.rating(1)), [])
    }

    func testAYearListsOnlyFinishesThatEndedInIt() throws {
        XCTAssertEqual(try finished(.genre("Mystery"), year: 2026), [11, 10])
        XCTAssertEqual(try finished(.genre("Mystery"), year: 2025), [12])
        XCTAssertEqual(try finished(.genre("Comics"), year: 2026), [13], "the year's first day counts")
        XCTAssertEqual(try finished(.genre("Mystery"), year: 2024), [])
    }

    func testEntriesOpenTheirBooks() throws {
        let entries = try store.pool.read { try CatalogStore.statsFinished($0, year: 2026, filter: .tag("Oxford")) }
        XCTAssertEqual(entries.map { $0.book?.id }, [1, 1])
        XCTAssertEqual(entries.first?.title, "Gaudy Night")
        XCTAssertEqual(entries.first?.endDay, "2026-08-15")
    }

    func testTheCountIsOfBooksAsTheBreakdownRowsIs() throws {
        let rereads = try store.pool.read { try CatalogStore.statsFinished($0, year: 2026, filter: .tag("Oxford")) }
        XCTAssertEqual(StatsFinishedText.count(rereads), "1 book · 2 finishes", "one Book finished twice")
        let once = try store.pool.read { try CatalogStore.statsFinished($0, year: 2026, filter: .genre("Comics")) }
        XCTAssertEqual(StatsFinishedText.count(once), "1 book")
    }

    func testTheTitleAndRangeReadAsTheBreakdownsDo() {
        XCTAssertEqual(StatsFinishedText.title(.genre("Mystery")), "Mystery")
        XCTAssertEqual(StatsFinishedText.title(.rating(4.5)), "4.5 stars")
        XCTAssertEqual(StatsFinishedText.range(2026), "Books you finished in 2026")
    }
}
