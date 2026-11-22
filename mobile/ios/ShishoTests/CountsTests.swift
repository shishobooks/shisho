import XCTest
@testable import Shisho

final class CountsTests: XCTestCase {
    /// A count outside a Text is inflected, never left as markup.
    func testBooksAreInflected() {
        XCTAssertEqual(Counts.books(1), "1 book")
        XCTAssertEqual(Counts.books(3), "3 books")
    }
}
