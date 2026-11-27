import XCTest
@testable import Shisho

/// Downloading the top of To Read: which Books download, and which are
/// remembered so a Download the user removed stays removed.
final class ToReadDownloadsTests: XCTestCase {
    func testDownloadsTheTopBooksInOrder() {
        let plan = ToReadDownloads.plan(toRead: [5, 3, 9, 1], count: 2, handled: [])
        XCTAssertEqual(plan.download, [5, 3])
        XCTAssertEqual(plan.handled, [5, 3])
    }

    func testOffDownloadsNothingAndForgetsEverything() {
        let plan = ToReadDownloads.plan(toRead: [5, 3], count: 0, handled: [5])
        XCTAssertEqual(plan.download, [])
        XCTAssertEqual(plan.handled, [], "turning it on again downloads the top as it is then")
    }

    func testABookDownloadedOnceIsntDownloadedAgain() {
        // The user removed 5's Download: it stays removed while it's on To Read.
        let plan = ToReadDownloads.plan(toRead: [5, 3, 9], count: 2, handled: [5, 3])
        XCTAssertEqual(plan.download, [])
    }

    func testABookMovingIntoTheTopDownloads() {
        let plan = ToReadDownloads.plan(toRead: [9, 5, 3], count: 2, handled: [5, 3])
        XCTAssertEqual(plan.download, [9])
        XCTAssertEqual(plan.handled, [9, 5, 3], "3 moved down but is still on To Read, so it isn't downloaded again if it comes back up")
    }

    func testABookLeavingToReadIsForgotten() {
        // Starting 5 took it off To Read; 9 moves up into the top two.
        let plan = ToReadDownloads.plan(toRead: [3, 9], count: 2, handled: [5, 3])
        XCTAssertEqual(plan.download, [9])
        XCTAssertEqual(plan.handled, [3, 9], "5 downloads again if it's put back on To Read")
    }

    func testRaisingTheCountDownloadsTheNextBooks() {
        let plan = ToReadDownloads.plan(toRead: [5, 3, 9, 1], count: 3, handled: [5, 3])
        XCTAssertEqual(plan.download, [9])
    }

    func testLoweringTheCountKeepsWhatsDownloaded() {
        let plan = ToReadDownloads.plan(toRead: [5, 3, 9], count: 1, handled: [5, 3])
        XCTAssertEqual(plan.download, [])
        XCTAssertEqual(plan.handled, [5, 3], "nothing is removed, and lowering then raising it doesn't download 3 again")
    }
}
