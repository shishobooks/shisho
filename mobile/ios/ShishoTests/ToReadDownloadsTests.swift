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

    func testARemovedDownloadStaysRemovedWhileTheBookStaysInTheTop() {
        // The user removed 5's Download.
        let plan = ToReadDownloads.plan(toRead: [5, 3, 9], count: 2, handled: [5, 3])
        XCTAssertEqual(plan.download, [])
    }

    func testABookComingBackIntoTheTopDownloadsAgain() {
        // 3 moved down out of the top, and its Download was removed there.
        let down = ToReadDownloads.plan(toRead: [5, 9, 3], count: 2, handled: [5, 3])
        XCTAssertEqual(down.download, [9])
        XCTAssertEqual(down.handled, [5, 9], "3 is forgotten once it leaves the top")
        let back = ToReadDownloads.plan(toRead: [3, 5, 9], count: 2, handled: down.handled)
        XCTAssertEqual(back.download, [3])
    }

    func testABookMovingIntoTheTopDownloads() {
        let plan = ToReadDownloads.plan(toRead: [9, 5, 3], count: 2, handled: [5, 3])
        XCTAssertEqual(plan.download, [9])
        XCTAssertEqual(plan.handled, [9, 5])
    }

    func testABookLeavingToReadIsForgotten() {
        // Starting 5 took it off To Read; 9 moves up into the top two.
        let plan = ToReadDownloads.plan(toRead: [3, 9], count: 2, handled: [5, 3])
        XCTAssertEqual(plan.download, [9])
        XCTAssertEqual(plan.handled, [3, 9], "5 downloads again if it's put back in the top")
    }

    func testRaisingTheCountDownloadsTheNextBooks() {
        let plan = ToReadDownloads.plan(toRead: [5, 3, 9, 1], count: 3, handled: [5, 3])
        XCTAssertEqual(plan.download, [9])
    }

    func testLoweringTheCountKeepsWhatsDownloaded() {
        // Nothing is removed: the plan only ever downloads.
        let plan = ToReadDownloads.plan(toRead: [5, 3, 9], count: 1, handled: [5, 3])
        XCTAssertEqual(plan.download, [])
        XCTAssertEqual(plan.handled, [5])
    }

    func testBooksAreRememberedOnlyOnceQueued() {
        let plan = ToReadDownloads.plan(toRead: [9, 5, 3], count: 2, handled: [5, 3])
        XCTAssertEqual(ToReadDownloads.remembered(previous: [5, 3], plan: plan, queued: true), [9, 5])
        // The queue write failed: 9 isn't remembered, so it's tried again,
        // and 3, out of the top now, is still forgotten.
        XCTAssertEqual(ToReadDownloads.remembered(previous: [5, 3], plan: plan, queued: false), [5])
    }
}
