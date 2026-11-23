import XCTest
@testable import Shisho

/// The full player's rules (#916): smart rewind and the sleep timer.
final class PlayerRulesTests: XCTestCase {
    /// A short pause picks up where it stopped; a longer one goes back a
    /// little, more the longer it was.
    func testSmartRewindGoesBackMoreAfterLongerPauses() {
        XCTAssertEqual(SmartRewind.seconds(afterPause: 20), 0)
        XCTAssertEqual(SmartRewind.seconds(afterPause: 59), 0)
        XCTAssertEqual(SmartRewind.seconds(afterPause: 3 * 60), 5)
        XCTAssertEqual(SmartRewind.seconds(afterPause: 20 * 60), 10)
        XCTAssertEqual(SmartRewind.seconds(afterPause: 3 * 3600), 20)
    }

    /// A timer counts down only time played, and fades the last seconds.
    func testASleepTimerCountsPlayedTimeAndFades() {
        var timer = SleepTimer.counting(length: 300, left: 300)
        var ranOut: Bool
        (timer, ranOut) = SleepTimerRules.played(timer, for: 290)
        XCTAssertEqual(timer, .counting(length: 300, left: 10))
        XCTAssertFalse(ranOut)
        XCTAssertEqual(SleepTimerRules.volume(timer), 1, accuracy: 0.001)
        (timer, _) = SleepTimerRules.played(timer, for: 5)
        XCTAssertEqual(SleepTimerRules.volume(timer), 0.5, accuracy: 0.001)
        (timer, ranOut) = SleepTimerRules.played(timer, for: 6)
        XCTAssertEqual(timer, .off)
        XCTAssertTrue(ranOut)
        XCTAssertEqual(SleepTimerRules.volume(timer), 1, accuracy: 0.001)

        XCTAssertEqual(SleepTimerRules.played(.endOfChapter, for: 1000).0, .endOfChapter)
        XCTAssertEqual(SleepTimerRules.played(.off, for: 1000).0, .off)
    }

    /// A shake starts the timer's length over; an end of chapter timer has
    /// nothing to extend.
    func testAShakeStartsTheLengthOver() {
        XCTAssertEqual(SleepTimerRules.extended(.counting(length: 300, left: 4)), .counting(length: 300, left: 300))
        XCTAssertEqual(SleepTimerRules.extended(.endOfChapter), .endOfChapter)
        XCTAssertEqual(SleepTimerRules.extended(.off), .off)
    }

    /// End of chapter stops where the chapter playing ends.
    func testEndOfChapterStopsAtTheEndOfTheChapterPlaying() {
        let chapters = [AudioChapter(title: "One", start: 0), AudioChapter(title: "Two", start: 3), AudioChapter(title: "Three", start: 6)]
        XCTAssertEqual(SleepTimerRules.chapterEnd(after: 0.5, chapters: chapters, duration: 9), 3)
        XCTAssertEqual(SleepTimerRules.chapterEnd(after: 4, chapters: chapters, duration: 9), 6)
        // Just before a boundary, which the player already shows as the next
        // chapter, the boundary is still the one to stop at.
        XCTAssertEqual(SleepTimerRules.chapterEnd(after: 2.8, chapters: chapters, duration: 9), 3)
        // From a chapter's very start, as after going to it, its own end.
        XCTAssertEqual(SleepTimerRules.chapterEnd(after: 3, chapters: chapters, duration: 9), 6)
        // In the last chapter, or without chapters, it's the end of the File.
        XCTAssertEqual(SleepTimerRules.chapterEnd(after: 7, chapters: chapters, duration: 9), 9)
        XCTAssertEqual(SleepTimerRules.chapterEnd(after: 7, chapters: [], duration: 9), 9)
    }
}
