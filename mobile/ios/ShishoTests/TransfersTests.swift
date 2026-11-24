import XCTest
@testable import Shisho

/// Which transfer is a File's own, so an event from one that was replaced
/// never changes the one that replaced it.
final class TransfersTests: XCTestCase {
    func testOnlyTheCurrentTransfersEventsCount() {
        var transfers = Transfers<String>()
        transfers.started(fileID: 1, id: "a")
        XCTAssertTrue(transfers.isCurrent(fileID: 1, id: "a"))
        XCTAssertTrue(transfers.busy(fileID: 1))

        // Removed, then downloaded again before the first one's end arrives.
        XCTAssertEqual(transfers.drop(fileID: 1), "a")
        transfers.started(fileID: 1, id: "b")
        XCTAssertEqual(transfers.ended(fileID: 1, id: "a"), .stale)
        XCTAssertTrue(transfers.isCurrent(fileID: 1, id: "b"), "the late end leaves the new transfer alone")
        XCTAssertEqual(transfers.ended(fileID: 1, id: "b"), .current)
        XCTAssertFalse(transfers.busy(fileID: 1))
    }

    /// One stopped on purpose, to carry on later, keeps the File busy until
    /// its end arrives with what to carry on from, so it isn't started twice.
    func testAPausedTransferStaysBusyUntilItEnds() {
        var transfers = Transfers<String>()
        transfers.started(fileID: 1, id: "a")
        XCTAssertEqual(transfers.pause(fileID: 1), "a")
        XCTAssertFalse(transfers.isCurrent(fileID: 1, id: "a"))
        XCTAssertTrue(transfers.busy(fileID: 1))
        XCTAssertEqual(transfers.ended(fileID: 1, id: "a"), .paused)
        XCTAssertFalse(transfers.busy(fileID: 1))
    }

    /// iOS cancels transfers when the app is force-quit and reports them at
    /// the next launch, when nothing is running for the File.
    func testAnEndWithNothingRunningIsAnOrphan() {
        var transfers = Transfers<String>()
        XCTAssertEqual(transfers.ended(fileID: 1, id: "old"), .orphan)
        transfers.started(fileID: 1, id: "new")
        XCTAssertEqual(transfers.ended(fileID: 1, id: "old"), .stale)
    }
}
