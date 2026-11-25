import XCTest
@testable import Shisho

final class CountsTests: XCTestCase {
    /// A count outside a Text is inflected, never left as markup.
    func testBooksAreInflected() {
        XCTAssertEqual(Counts.books(1), "1 book")
        XCTAssertEqual(Counts.books(3), "3 books")
    }

    /// A word count in the ballpark, as the web shows it: exact under a
    /// thousand, then rounded.
    func testWordsAreRounded() {
        XCTAssertEqual(FileFacts.words(1), "1 word")
        XCTAssertEqual(FileFacts.words(850), "850 words")
        XCTAssertEqual(FileFacts.words(1500), "1.5K words")
        XCTAssertEqual(FileFacts.words(29327), "29K words")
        XCTAssertEqual(FileFacts.words(139_549), "140K words")
        XCTAssertEqual(FileFacts.words(1_234_567), "1.2M words")
        XCTAssertEqual(FileFacts.exactWords(29327), "29,327 words")
    }
}
