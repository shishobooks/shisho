import ShishoAPI
import XCTest
@testable import Shisho

final class CatalogStoreTests: XCTestCase {
    private var store: CatalogStore!

    override func setUpWithError() throws {
        let path = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite").path
        store = try CatalogStore(path: path)
    }

    private func page(_ json: String) throws -> Shishoapp.FeedResponse {
        try makeShishoDecoder().decode(Shishoapp.FeedResponse.self, from: Data(json.utf8))
    }

    private func book(_ id: Int, _ title: String, library: Int = 1) -> String {
        """
        {"kind":"book","id":\(id),"library_id":\(library),"book":{"id":\(id),"library_id":\(library),"title":"\(title)","sort_title":"\(title)",
         "authors":[{"person_id":1,"name":"Ann Author","sort_name":"Author, Ann"}],"files":[{"id":\(id * 10),"file_type":"epub","file_role":"main","size_bytes":10}],"cover_version":"\(id)-1"}}
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
