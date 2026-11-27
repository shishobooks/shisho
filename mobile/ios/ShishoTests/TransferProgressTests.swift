import XCTest
@testable import Shisho

/// What a download in front of the user reports as it goes: only whole
/// percents, so the screen showing it redraws rarely.
final class TransferProgressTests: XCTestCase {
    func testTheFirstReadingIsReported() {
        XCTAssertEqual(TransferProgress.fraction(received: 0, expected: 200, last: nil), 0)
        XCTAssertEqual(TransferProgress.fraction(received: 50, expected: 200, last: nil), 0.25)
    }

    func testAnUnknownSizeReportsNothing() {
        XCTAssertNil(TransferProgress.fraction(received: 50, expected: -1, last: nil))
        XCTAssertNil(TransferProgress.fraction(received: 50, expected: 0, last: nil))
    }

    func testLessThanAPercentIsntReported() {
        XCTAssertNil(TransferProgress.fraction(received: 1005, expected: 100_000, last: 0.01))
        XCTAssertEqual(TransferProgress.fraction(received: 2500, expected: 100_000, last: 0.01), 0.025)
    }

    func testTheEndIsAlwaysReported() {
        XCTAssertEqual(TransferProgress.fraction(received: 100_000, expected: 100_000, last: 0.995), 1)
        XCTAssertNil(TransferProgress.fraction(received: 100_000, expected: 100_000, last: 1))
    }

    func testMoreThanExpectedStopsAtTheEnd() {
        XCTAssertEqual(TransferProgress.fraction(received: 300, expected: 200, last: nil), 1)
    }
}
