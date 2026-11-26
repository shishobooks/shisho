import Foundation

/// When a stream that stopped with an error is played again on its own
/// (`AudioPlayer.retryAfterStop`): once per failure.
enum StreamStopRules {
    /// How long after a failure the player may play again on its own.
    static let window: TimeInterval = 5
    /// How far listening must get past the last failure for the next to be
    /// a failure of its own.
    static let progress: Double = 2

    /// Whether a stop at position is a failure of its own, with its own
    /// retry, rather than the last one again: one failure is reported several
    /// times in a row, and a stream that can't play fails at the same place
    /// each time it's played again. A seek gives one its own retry too
    /// (`AudioPlayer.move(to:)`).
    static func isNew(at now: Date, position: Double, last: (at: Date, position: Double)?) -> Bool {
        guard let last else { return true }
        return now.timeIntervalSince(last.at) > window && abs(position - last.position) > progress
    }
}

/// How a car's and headphones' buttons skip
/// (`AudioPlayer.setUpRemoteCommands`).
enum RemoteSkipRules {
    /// A seek button's press and release.
    enum SeekPhase: Sendable {
        case began, ended
    }

    /// A seek skips once, as the button is let go: a car's tap may send only
    /// that end.
    static func seekSkips(_ phase: SeekPhase) -> Bool {
        phase == .ended
    }
}

/// Going back a little after a long pause, so the listener picks up the
/// thread. The longer the pause, the further back.
enum SmartRewind {
    static func seconds(afterPause pause: TimeInterval) -> Double {
        switch pause {
        case ..<60: 0
        case ..<(10 * 60): 5
        case ..<3600: 10
        default: 20
        }
    }
}

/// The sleep timer: off, counting down a length of listening, or stopping
/// at the end of the chapter playing.
enum SleepTimer: Equatable, Sendable {
    case off
    /// Counts only time played, so a pause doesn't run it out.
    case counting(length: TimeInterval, left: TimeInterval)
    case endOfChapter
}

enum SleepTimerRules {
    /// The lengths offered, in seconds.
    static let lengths: [TimeInterval] = [5, 10, 15, 30, 45, 60].map { $0 * 60 }
    /// The last seconds fade out, so stopping doesn't jolt the listener
    /// awake.
    static let fadeOver: TimeInterval = 10

    /// The timer after seconds more were played, and whether it ran out.
    static func played(_ timer: SleepTimer, for seconds: TimeInterval) -> (SleepTimer, Bool) {
        guard case let .counting(length, left) = timer else { return (timer, false) }
        let remaining = left - max(0, seconds)
        if remaining <= 0 {
            return (.off, true)
        }
        return (.counting(length: length, left: remaining), false)
    }

    /// A shake starts the length over.
    static func extended(_ timer: SleepTimer) -> SleepTimer {
        guard case let .counting(length, _) = timer else { return timer }
        return .counting(length: length, left: length)
    }

    /// How loud playback is while the timer runs: full until the last
    /// seconds, then fading to nothing.
    static func volume(_ timer: SleepTimer) -> Float {
        guard case let .counting(_, left) = timer, left < fadeOver else { return 1 }
        return Float(max(0, left) / fadeOver)
    }

    /// Where an end of chapter timer set at position stops: the next chapter
    /// start after it, or the end of the File in the last chapter or without
    /// chapters. It's the next start strictly after, not the end of the
    /// chapter the player shows, which counts from a moment before its
    /// start: just before a boundary, that boundary is the one to stop at.
    static func chapterEnd(after position: Double, chapters: [AudioChapter], duration: Double) -> Double {
        chapters.first { $0.start > position }?.start ?? duration
    }
}

/// The player's settings on this installation, the same for every account.
enum PlayerSettings {
    /// The skip lengths offered, each with a system icon.
    static let skipLengths: [Double] = [5, 10, 15, 30, 45, 60, 75, 90]
    static let defaultSkip: Double = 30

    private static let skipBackKey = "player.skipBack"
    private static let skipForwardKey = "player.skipForward"
    private static let smartRewindKey = "player.smartRewind"

    static var skipBack: Double {
        get { skip(UserDefaults.standard.double(forKey: skipBackKey)) }
        set { UserDefaults.standard.set(skip(newValue), forKey: skipBackKey) }
    }

    static var skipForward: Double {
        get { skip(UserDefaults.standard.double(forKey: skipForwardKey)) }
        set { UserDefaults.standard.set(skip(newValue), forKey: skipForwardKey) }
    }

    /// On unless turned off.
    static var smartRewind: Bool {
        get { UserDefaults.standard.object(forKey: smartRewindKey) as? Bool ?? true }
        set { UserDefaults.standard.set(newValue, forKey: smartRewindKey) }
    }

    /// A length offered, or the default for anything else.
    private static func skip(_ seconds: Double) -> Double {
        skipLengths.contains(seconds) ? seconds : defaultSkip
    }
}
