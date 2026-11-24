import XCTest
@testable import Shisho

/// When the pager turns, queues, or springs back (ADR 0026): taps queue
/// instead of being dropped, and a page is never revealed before it's
/// ready.
final class PagerTurnsTests: XCTestCase {
    private func ready() -> PagerTurns {
        var turns = PagerTurns()
        _ = turns.neighbor(.back, is: .ready)
        _ = turns.neighbor(.forward, is: .ready)
        return turns
    }

    func testATapTurnsAtOnceWhenTheNeighborIsReady() {
        var turns = ready()
        XCTAssertEqual(turns.ask(.forward), .turn(.forward))
        XCTAssertEqual(turns.queued, 0)
    }

    /// Ten quick taps all turn: each waits for the page behind it to be
    /// laid out, then turns.
    func testRapidTapsQueueAndEachTurnsOnceItsPageIsReady() {
        var turns = ready()
        XCTAssertEqual(turns.ask(.forward), .turn(.forward))
        turns.committed(.forward)
        var turned = 1
        for _ in 0..<9 {
            if turns.ask(.forward) == .turn(.forward) {
                turned += 1
                turns.committed(.forward)
            }
        }
        XCTAssertEqual(turns.queued, 9)
        while turns.queued > 0 {
            XCTAssertEqual(turns.neighbor(.forward, is: .ready), .turn(.forward))
            turns.committed(.forward)
            turned += 1
        }
        XCTAssertEqual(turned, 10)
    }

    /// Every tap counts: a tap the other way cancels one queued turn rather
    /// than everything queued.
    func testATapTheOtherWayCancelsOneQueuedTurn() {
        var turns = ready()
        XCTAssertEqual(turns.ask(.forward), .turn(.forward))
        turns.committed(.forward)
        for _ in 0..<3 { XCTAssertEqual(turns.ask(.forward), .none) }
        XCTAssertEqual(turns.queued, 3)
        XCTAssertEqual(turns.ask(.back), .none)
        XCTAssertEqual(turns.queued, 2)
    }

    /// Taps back past what was queued turn back once they outnumber it.
    func testTapsBackPastTheQueueTurnBack() {
        var turns = ready()
        XCTAssertEqual(turns.ask(.forward), .turn(.forward))
        turns.committed(.forward)
        XCTAssertEqual(turns.ask(.forward), .none)
        XCTAssertEqual(turns.ask(.back), .none)
        XCTAssertEqual(turns.queued, 0)
        XCTAssertEqual(turns.ask(.back), .turn(.back))
    }

    /// A drag that springs back leaves the taps queued before it.
    func testAShortDragKeepsQueuedTaps() {
        var turns = ready()
        XCTAssertEqual(turns.ask(.forward), .turn(.forward))
        turns.committed(.forward)
        _ = turns.ask(.forward)
        turns.dragBegan()
        XCTAssertEqual(turns.dragEnded(.back, asked: false, revealed: true), .springBack)
        XCTAssertEqual(turns.queued, 1)
    }

    func testNothingQueuesAtTheEndOfTheBook() {
        var turns = ready()
        _ = turns.neighbor(.forward, is: .edge)
        XCTAssertEqual(turns.ask(.forward), .none)
        XCTAssertEqual(turns.queued, 0)
    }

    /// A queued turn that reaches the end of the book is dropped.
    func testAQueuedTurnStopsAtTheEnd() {
        var turns = ready()
        XCTAssertEqual(turns.ask(.forward), .turn(.forward))
        turns.committed(.forward)
        _ = turns.ask(.forward)
        _ = turns.ask(.forward)
        XCTAssertEqual(turns.neighbor(.forward, is: .edge), .none)
        XCTAssertEqual(turns.queued, 0)
    }

    /// A drag commits only onto the page it revealed, never onto a page that
    /// became ready mid-drag behind the plain backdrop.
    func testADragCommitsOnlyOntoARevealedPage() {
        var turns = ready()
        turns.dragBegan()
        XCTAssertEqual(turns.dragEnded(.forward, asked: true, revealed: true), .commit)

        turns = PagerTurns()
        _ = turns.neighbor(.back, is: .ready)
        turns.dragBegan()
        XCTAssertEqual(turns.neighbor(.forward, is: .ready), .none)
        XCTAssertEqual(turns.dragEnded(.forward, asked: true, revealed: false), .springBack)
        // The turn the reader asked for still happens, once the drag is done.
        XCTAssertEqual(turns.queued, 1)
        XCTAssertEqual(turns.drain(), .turn(.forward))
    }

    /// A drag that wasn't far or fast enough springs back and queues nothing.
    func testAShortDragSpringsBack() {
        var turns = ready()
        turns.dragBegan()
        XCTAssertEqual(turns.dragEnded(.forward, asked: false, revealed: true), .springBack)
        XCTAssertEqual(turns.queued, 0)
    }

    /// Nothing turns while a finger is down, even when a queued page lands.
    func testQueuedTurnsWaitForTheDragToEnd() {
        var turns = ready()
        XCTAssertEqual(turns.ask(.forward), .turn(.forward))
        turns.committed(.forward)
        _ = turns.ask(.forward)
        turns.dragBegan()
        XCTAssertEqual(turns.neighbor(.forward, is: .ready), .none)
        XCTAssertEqual(turns.dragEnded(.forward, asked: true, revealed: true), .commit)
        turns.committed(.forward)
        // The queued tap and the flick add up, as two taps do.
        XCTAssertEqual(turns.queued, 1)
    }

    /// After a turn, the old page is the ready one behind, and the page
    /// ahead is being laid out.
    func testCommittingRotatesTheNeighbors() {
        var turns = ready()
        turns.committed(.forward)
        XCTAssertEqual(turns.state(.back), .ready)
        XCTAssertEqual(turns.state(.forward), .pending)
        XCTAssertFalse(turns.available(.forward))
    }

    /// A jump drops the queue, and both neighbors are laid out again.
    func testAJumpStartsOver() {
        var turns = ready()
        turns.committed(.forward)
        _ = turns.ask(.forward)
        turns.reset()
        XCTAssertEqual(turns.queued, 0)
        XCTAssertEqual(turns.state(.forward), .pending)
        XCTAssertEqual(turns.state(.back), .pending)
    }

    // MARK: - Tap zones and Reading Direction

    func testTapZonesFollowTheReadingDirection() {
        XCTAssertEqual(PagerZones.tap(at: 0.1, rightToLeft: false), .turn(.back))
        XCTAssertEqual(PagerZones.tap(at: 0.5, rightToLeft: false), .chrome)
        XCTAssertEqual(PagerZones.tap(at: 0.9, rightToLeft: false), .turn(.forward))
        XCTAssertEqual(PagerZones.tap(at: 0.1, rightToLeft: true), .turn(.forward))
        XCTAssertEqual(PagerZones.tap(at: 0.9, rightToLeft: true), .turn(.back))
    }

    /// In the edge thirds, where a tap turns the page, only a tap right on a
    /// link follows it; in the middle a tap near one does, as links in text
    /// are small.
    func testATapNearALinkFollowsItOnlyInTheMiddle() {
        XCTAssertEqual(PagerZones.linkSlop(at: 0.5), PagerController.linkSlop)
        XCTAssertEqual(PagerZones.linkSlop(at: 0.1), 0)
        XCTAssertEqual(PagerZones.linkSlop(at: 0.9), 0)
    }

    func testANoteReferenceKeepsItsReachWhereOtherLinksNeedAHit() {
        // A plain link and a note reference, each 10 points square, 20
        // points from the tap.
        let link: [Double] = [100, 100, 10, 10, 0, 0]
        let note: [Double] = [100, 200, 10, 10, 1, 1]
        let nearLink = CGPoint(x: 130, y: 105)
        let nearNote = CGPoint(x: 130, y: 205)
        XCTAssertNil(ReaderPage.link(in: [link, note], near: nearLink, slop: 0, noteSlop: 24))
        XCTAssertEqual(ReaderPage.link(in: [link, note], near: nearNote, slop: 0, noteSlop: 24), 1)
        XCTAssertEqual(ReaderPage.link(in: [link, note], near: nearLink, slop: 24, noteSlop: 24), 0)
    }

    /// A finger moving left brings in the page on the right: the next one,
    /// or the previous one in a right-to-left book.
    func testDragsFollowTheReadingDirection() {
        XCTAssertEqual(PagerZones.direction(dragX: -10, rightToLeft: false), .forward)
        XCTAssertEqual(PagerZones.direction(dragX: 10, rightToLeft: false), .back)
        XCTAssertEqual(PagerZones.direction(dragX: -10, rightToLeft: true), .back)
        XCTAssertEqual(PagerZones.direction(dragX: 10, rightToLeft: true), .forward)
        XCTAssertNil(PagerZones.direction(dragX: 0, rightToLeft: false))
    }

    /// A release turns past a quarter of the width or on a flick toward the
    /// page, and never on a flick back.
    func testWhatAReleaseAsksFor() {
        XCTAssertTrue(PagerZones.asksToTurn(dragX: -120, velocity: 0, width: 400))
        XCTAssertFalse(PagerZones.asksToTurn(dragX: -60, velocity: 0, width: 400))
        XCTAssertTrue(PagerZones.asksToTurn(dragX: -20, velocity: -800, width: 400))
        XCTAssertFalse(PagerZones.asksToTurn(dragX: -200, velocity: 600, width: 400))
        XCTAssertTrue(PagerZones.asksToTurn(dragX: 150, velocity: 0, width: 400))
    }
}
