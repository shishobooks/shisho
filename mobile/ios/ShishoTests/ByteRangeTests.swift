import XCTest
@testable import Shisho

/// The reader's page reads the book a part at a time (#919), as the web
/// reader does with HTTP Range: the scheme handler answers the part asked
/// for.
final class ByteRangeTests: XCTestCase {
    func testAClosedRange() {
        XCTAssertEqual(ByteRange.parse("bytes=10-19", size: 100), 10..<20)
    }

    func testAnOpenEndedRangeRunsToTheEnd() {
        XCTAssertEqual(ByteRange.parse("bytes=90-", size: 100), 90..<100)
    }

    func testAnEndPastTheFileIsTheEnd() {
        XCTAssertEqual(ByteRange.parse("bytes=90-500", size: 100), 90..<100)
    }

    func testTheLastBytes() {
        XCTAssertEqual(ByteRange.parse("bytes=-10", size: 100), 90..<100)
    }

    func testRangesThatCantBeServed() {
        XCTAssertNil(ByteRange.parse("bytes=100-", size: 100))
        XCTAssertNil(ByteRange.parse("bytes=20-10", size: 100))
        XCTAssertNil(ByteRange.parse("bytes=0-1,5-9", size: 100))
        XCTAssertNil(ByteRange.parse("items=0-1", size: 100))
        XCTAssertNil(ByteRange.parse(nil, size: 100))
    }
}
