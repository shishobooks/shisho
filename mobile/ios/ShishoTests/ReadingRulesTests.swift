import XCTest
@testable import Shisho

/// Reading Sessions are timed with the web reader's rules (#919): 5 idle
/// minutes end one, each page counts 5 seconds to 2 minutes, backgrounding
/// or locking ends one, and coming back within 30 seconds resumes it.
final class ReadingRulesTests: XCTestCase {
    private let start = Date(timeIntervalSince1970: 1_800_000_000)

    private func at(_ seconds: TimeInterval) -> Date {
        start.addingTimeInterval(seconds)
    }

    /// Each page counts from 5 seconds up to 2 minutes; pages flipped past
    /// add nothing.
    func testEachPageCountsFiveSecondsToTwoMinutes() throws {
        var session = ReadingRules.begin(fileID: 7, page: "0:1", percentage: 0.1, at: at(0))
        session = ReadingRules.turned(session, to: "0:2", percentage: 0.11, at: at(3)) // 3 s: nothing
        session = ReadingRules.turned(session, to: "0:3", percentage: 0.12, at: at(33)) // 30 s
        session = ReadingRules.turned(session, to: "0:4", percentage: 0.13, at: at(333)) // 300 s: 120

        let snapshot = try XCTUnwrap(ReadingRules.snapshot(session, at: at(343))) // 10 s so far
        XCTAssertEqual(snapshot.fileID, 7)
        XCTAssertEqual(snapshot.sittingID, session.id)
        XCTAssertEqual(snapshot.startedAt, at(0))
        XCTAssertEqual(snapshot.endedAt, at(343))
        XCTAssertEqual(snapshot.duration, 30 + 120 + 10)
        XCTAssertEqual(snapshot.startPercentage, 0.1)
        XCTAssertEqual(snapshot.endPercentage, 0.13)
    }

    /// Reopening the same page isn't a turn: its time keeps adding up.
    func testTheSamePageKeepsCounting() throws {
        var session = ReadingRules.begin(fileID: 7, page: "0:1", percentage: 0.1, at: at(0))
        session = ReadingRules.turned(session, to: "0:1", percentage: 0.1, at: at(4))
        let snapshot = try XCTUnwrap(ReadingRules.snapshot(session, at: at(8)))
        XCTAssertEqual(snapshot.duration, 8)
    }

    /// A session with no counted time isn't one.
    func testASessionWithNothingCountedIsLeftOut() {
        let session = ReadingRules.begin(fileID: 7, page: "0:1", percentage: 0.1, at: at(0))
        XCTAssertNil(ReadingRules.snapshot(session, at: at(4)))
    }

    /// Five minutes without input end the session at its last input, or two
    /// minutes into the page when that's later.
    func testFiveIdleMinutesEndTheSession() throws {
        var session = ReadingRules.begin(fileID: 7, page: "0:1", percentage: 0.1, at: at(0))
        session = ReadingRules.turned(session, to: "0:2", percentage: 0.2, at: at(60))
        XCTAssertFalse(ReadingRules.isIdle(session, at: at(60 + 300)))
        XCTAssertTrue(ReadingRules.isIdle(session, at: at(60 + 301)))

        let ended = try XCTUnwrap(ReadingRules.endIdle(session))
        XCTAssertEqual(ended.endedAt, at(60 + 120))
        XCTAssertEqual(ended.duration, 60 + 120)
    }

    /// Input that isn't a turn, such as a tap that shows the controls,
    /// keeps the session going.
    func testInputKeepsTheSessionGoing() {
        var session = ReadingRules.begin(fileID: 7, page: "0:1", percentage: 0.1, at: at(0))
        session = ReadingRules.touched(session, at: at(250))
        XCTAssertFalse(ReadingRules.isIdle(session, at: at(500)))
    }

    /// Going to the background ends the session there; coming back within
    /// 30 seconds goes on with it, without counting the time away.
    func testComingBackWithinThirtySecondsResumes() throws {
        var session = ReadingRules.begin(fileID: 7, page: "0:1", percentage: 0.1, at: at(0))
        session = ReadingRules.turned(session, to: "0:2", percentage: 0.2, at: at(40))
        let (away, ended) = ReadingRules.background(session, at: at(70))
        let first = try XCTUnwrap(ended)
        XCTAssertEqual(first.endedAt, at(70))
        XCTAssertEqual(first.duration, 70)

        let resumed = try XCTUnwrap(ReadingRules.foreground(away, at: at(95)))
        XCTAssertEqual(resumed.id, session.id)
        let later = try XCTUnwrap(ReadingRules.snapshot(ReadingRules.turned(resumed, to: "0:3", percentage: 0.3, at: at(105)), at: at(105)))
        XCTAssertEqual(later.sittingID, first.sittingID)
        XCTAssertEqual(later.startedAt, at(0))
        // The page shown when the app went away counts 30 s before and 10 s
        // after, as one visit.
        XCTAssertEqual(later.duration, 40 + 30 + 10)
    }

    func testComingBackLaterStartsAnotherSession() {
        let session = ReadingRules.begin(fileID: 7, page: "0:1", percentage: 0.1, at: at(0))
        let (away, _) = ReadingRules.background(session, at: at(70))
        XCTAssertNil(ReadingRules.foreground(away, at: at(101)))
    }

    /// Time away from the reader isn't idle time, so coming back within
    /// 30 seconds never finds the session over.
    func testTimeAwayIsntIdle() throws {
        var session = ReadingRules.begin(fileID: 7, page: "0:1", percentage: 0.1, at: at(0))
        session = ReadingRules.turned(session, to: "0:2", percentage: 0.2, at: at(290))
        let (away, _) = ReadingRules.background(session, at: at(295))
        let resumed = try XCTUnwrap(ReadingRules.foreground(away, at: at(320)))
        XCTAssertFalse(ReadingRules.isIdle(resumed, at: at(330)))
    }
}
