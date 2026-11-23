import XCTest
@testable import ShishoAPI

/// The generated types decode leniently (ADR 0021), so a newer server can't
/// break an older app. These cases match the Kotlin decode tests.
final class DecodeTests: XCTestCase {
    private let entry = Data("""
    {
      "kind": "book",
      "id": 7,
      "deleted": false,
      "library_id": 1,
      "a_field_from_a_newer_server": {"anything": [1, 2]},
      "book": {
        "id": 7,
        "library_id": 1,
        "title": "Dune",
        "sort_title": "Dune",
        "authors": null,
        "files": [{"id": 3, "file_type": "djvu", "file_role": "main", "size_bytes": 12345678901}],
        "cover_version": "3-1700000000"
      }
    }
    """.utf8)

    func testIgnoresAnExtraField() throws {
        let decoded = try makeShishoDecoder().decode(Shishoapp.FeedEntry.self, from: entry)
        XCTAssertEqual(decoded.id, 7)
        XCTAssertEqual(decoded.kind, .book)
    }

    func testLeavesAMissingOptionalFieldEmpty() throws {
        let book = try XCTUnwrap(makeShishoDecoder().decode(Shishoapp.FeedEntry.self, from: entry).book)
        // subtitle is a pointer in Go, so it's nil when left out.
        XCTAssertNil(book.subtitle)
        // series was left out and authors sent as null: both are empty.
        XCTAssertEqual(book.series, [])
        XCTAssertEqual(book.authors, [])
    }

    func testKeepsAnUnknownEnumValue() throws {
        let book = try XCTUnwrap(makeShishoDecoder().decode(Shishoapp.FeedEntry.self, from: entry).book)
        XCTAssertEqual(book.files.first?.fileType, .unknown("djvu"))
        XCTAssertEqual(book.files.first?.fileType.rawValue, "djvu")
        XCTAssertEqual(book.files.first?.sizeBytes, 12_345_678_901)

        let response = try makeShishoDecoder().decode(
            Shishoapp.FeedResponse.self,
            from: Data(#"{"entries":[{"kind":"highlight","id":1}],"cursor":9}"#.utf8)
        )
        XCTAssertEqual(response.entries.first?.kind, .unknown("highlight"))
        XCTAssertEqual(response.cursor, 9)
    }

    func testEncodesKnownAndUnknownValuesAsSent() throws {
        let data = try JSONEncoder().encode(Shishoapp.FeedEntry(kind: .unknown("highlight"), id: 1))
        let json = try XCTUnwrap(String(data: data, encoding: .utf8))
        XCTAssertTrue(json.contains(#""kind":"highlight""#), json)
    }
}
