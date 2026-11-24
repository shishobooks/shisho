import XCTest
@testable import Shisho

/// Back and forward through the places a reader jumped from, by the web
/// reader's rules (app/libraries/locationHistory.ts and its tests, #919).
final class LocationHistoryTests: XCTestCase {
    func testItStartsWithNowhereToGo() {
        var history = LocationHistory()
        XCTAssertFalse(history.canGoBack)
        XCTAssertFalse(history.canGoForward)
        XCTAssertNil(history.back(from: "here"))
        XCTAssertNil(history.forward(from: "here"))
    }

    func testItGoesBackToWhereAJumpLeftFromAndForwardToWhereBackLeft() {
        var history = LocationHistory()
        history.recordJump(from: "start")
        XCTAssertTrue(history.canGoBack)
        // The reader paged on from where the jump landed.
        XCTAssertEqual(history.back(from: "paged-on"), "start")
        XCTAssertFalse(history.canGoBack)
        XCTAssertTrue(history.canGoForward)
        XCTAssertEqual(history.forward(from: "start"), "paged-on")
        XCTAssertFalse(history.canGoForward)
        XCTAssertEqual(history.back(from: "paged-on"), "start")
    }

    func testItGoesBackThroughSeveralJumpsInTurn() {
        var history = LocationHistory()
        history.recordJump(from: "a")
        history.recordJump(from: "b")
        XCTAssertEqual(history.back(from: "c"), "b")
        XCTAssertEqual(history.back(from: "b"), "a")
        XCTAssertEqual(history.forward(from: "a"), "b")
        XCTAssertEqual(history.forward(from: "b"), "c")
    }

    func testANewJumpForgetsTheWayForward() {
        var history = LocationHistory()
        history.recordJump(from: "a")
        _ = history.back(from: "b")
        history.recordJump(from: "a")
        XCTAssertFalse(history.canGoForward)
        XCTAssertEqual(history.back(from: "c"), "a")
    }

    func testItSkipsThePlaceTheReaderIsAlreadyAt() {
        var history = LocationHistory()
        history.recordJump(from: "a")
        history.recordJump(from: "b")
        // The reader paged back to b by hand: going back to it would do nothing.
        XCTAssertEqual(history.back(from: "b"), "a")
        XCTAssertEqual(history.forward(from: "a"), "b")
        XCTAssertFalse(history.canGoForward)
    }

    func testItDropsEntriesThatAreAllWhereTheReaderIs() {
        var history = LocationHistory()
        history.recordJump(from: "a")
        XCTAssertNil(history.back(from: "a"))
        XCTAssertFalse(history.canGoBack)
        XCTAssertFalse(history.canGoForward)
    }

    /// Jumps in one group, as the results of one search, keep only the
    /// first.
    func testItKeepsOneEntryPerGroupOfJumps() {
        var history = LocationHistory()
        history.recordJump(from: "before-search", group: 1)
        history.recordJump(from: "result-1", group: 1)
        history.recordJump(from: "result-2", group: 1)
        XCTAssertEqual(history.back(from: "result-3"), "before-search")
        XCTAssertFalse(history.canGoBack)
    }

    func testAGroupStartsAgainAfterAnotherJumpOrGoingBack() {
        var history = LocationHistory()
        history.recordJump(from: "before-search", group: 1)
        history.recordJump(from: "result-1")
        history.recordJump(from: "link-target", group: 1)
        XCTAssertEqual(history.back(from: "result-2"), "link-target")

        history.recordJump(from: "link-target", group: 1)
        XCTAssertEqual(history.back(from: "result-3"), "link-target")
        XCTAssertEqual(history.back(from: "link-target"), "result-1")
    }
}
