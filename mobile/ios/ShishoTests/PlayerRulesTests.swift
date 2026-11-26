import XCTest
@testable import Shisho

/// The full player's rules (#916): smart rewind, the sleep timer, retrying
/// a stream that stopped, and how a car's and headphones' buttons skip
/// (#917).
final class PlayerRulesTests: XCTestCase {
    /// A notice over Now Playing's artwork takes its height from the
    /// artwork, so the tools under the controls stay on screen.
    func testANoticeTakesItsHeightFromTheArtwork() {
        let phone = CGSize(width: 402, height: 780)
        let plain = NowPlayingLayout.artworkWidth(space: phone, accessibilitySize: false, noticesHeight: 0)
        XCTAssertEqual(plain, 296.4, accuracy: 0.01)
        XCTAssertEqual(NowPlayingLayout.artworkWidth(space: phone, accessibilitySize: false, noticesHeight: 100), plain - 100, accuracy: 0.01)
        XCTAssertEqual(NowPlayingLayout.artworkWidth(space: phone, accessibilitySize: false, noticesHeight: 400), 72,
                       "two notices shrink it further than with none")
        XCTAssertEqual(NowPlayingLayout.artworkWidth(space: CGSize(width: 402, height: 200), accessibilitySize: false, noticesHeight: 0), 120)
    }

    /// A car's seek button skips once, as it's let go.
    func testACarsSeekButtonSkipsOnceAsItsLetGo() {
        XCTAssertFalse(RemoteSkipRules.seekSkips(.began))
        XCTAssertTrue(RemoteSkipRules.seekSkips(.ended), "a car's tap may send only the end")
    }

    /// A stream that stops with an error gets one retry per failure. The
    /// same failure again, even after a quiet spell, is still that failure
    /// while playback hasn't got anywhere, so it shows rather than retrying
    /// forever.
    func testAStreamStopIsNewOnlyOnceListeningGotSomewhere() {
        let then = Date(timeIntervalSince1970: 1_800_000_000)
        XCTAssertTrue(StreamStopRules.isNew(at: then, position: 0, last: nil), "the first")
        XCTAssertFalse(StreamStopRules.isNew(at: then.addingTimeInterval(2), position: 40, last: (then, 0)),
                       "one failure reported again soon after")
        XCTAssertFalse(StreamStopRules.isNew(at: then.addingTimeInterval(6), position: 0.5, last: (then, 0)),
                       "the same place a while later")
        XCTAssertTrue(StreamStopRules.isNew(at: then.addingTimeInterval(60), position: 50, last: (then, 0)),
                      "after listening on")
    }

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

    /// A chapter is named by its title, or by its number when it has none,
    /// wherever the player names it: never "2. Chapter 1" after an
    /// introduction, and never a blank.
    func testAChapterIsNamedByItsTitleOrNumber() {
        let chapters = [AudioChapter(title: "Introduction", start: 0), AudioChapter(title: "  ", start: 60),
                        AudioChapter(title: "\n", start: 120)]
        XCTAssertEqual(AudioChapters.name(of: 0, in: chapters), "Introduction")
        XCTAssertEqual(AudioChapters.name(of: 1, in: chapters), "Chapter 2")
        XCTAssertEqual(AudioChapters.name(of: 2, in: chapters), "Chapter 3")
    }
}
