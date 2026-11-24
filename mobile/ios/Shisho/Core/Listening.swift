import Foundation

/// One sitting of listening to a File, timed in wall-clock time while
/// playing. A pause of up to `ListeningRules.splitAfter` keeps it going
/// without counting the pause; a longer one ends it where the pause began.
/// It's kept in the account's database as it goes, so a sitting the app
/// was killed during still becomes a Reading Session on the next launch.
struct ListeningSitting: Codable, Equatable, Sendable {
    /// Names the sitting's Reading Session: every snapshot of it sent on a
    /// pause carries this, and the server keeps one session for it.
    var id: String = UUID().uuidString.lowercased()
    var fileID: Int
    var startedAt: Date
    var startPercentage: Double
    /// Time played before `playingSince`.
    var playedSeconds: Double
    /// When playback last started, while it plays.
    var playingSince: Date?
    /// The last moment known to belong to the sitting: now while playing (as
    /// of the last save), and the pause while paused.
    var lastActiveAt: Date
    var lastPercentage: Double

    var isPlaying: Bool { playingSince != nil }

    /// Seconds played up to at, which counts only while playing.
    func played(at: Date) -> Double {
        guard let playingSince else { return playedSeconds }
        return playedSeconds + max(0, at.timeIntervalSince(playingSince))
    }
}

/// A Reading Session ready for the journal: a sitting's session so far, or
/// all of it once the sitting ends.
struct FinishedSession: Equatable, Sendable {
    /// The sitting's id, which every snapshot of its session carries.
    var sittingID: String = ""
    let fileID: Int
    let startedAt: Date
    let endedAt: Date
    let startPercentage: Double
    let endPercentage: Double
    /// Seconds played, which leaves out pauses and interruptions.
    let duration: Int
}

/// How listening is timed, the same rules as the web player's: a session
/// is a sitting, a pause longer than five minutes splits it, and only time
/// playing counts.
enum ListeningRules {
    static let splitAfter: TimeInterval = 5 * 60
    /// The position is saved this often while playing, and on pause.
    static let saveInterval: TimeInterval = 5

    /// Starts playing fileID at at. The current sitting goes on unless it's
    /// for another File or was paused too long; then it's finished and
    /// returned, to record, beside the new one.
    static func play(_ current: ListeningSitting?, fileID: Int, at: Date, percentage: Double) -> (ListeningSitting, FinishedSession?) {
        if var sitting = current, sitting.fileID == fileID, !isOver(sitting, at: at) {
            if sitting.playingSince == nil {
                sitting.playingSince = at
            }
            sitting.lastActiveAt = at
            return (sitting, nil)
        }
        let ended = current.flatMap { finish($0, at: at) }
        let fresh = ListeningSitting(fileID: fileID, startedAt: at, startPercentage: percentage, playedSeconds: 0,
                                     playingSince: at, lastActiveAt: at, lastPercentage: percentage)
        return (fresh, ended)
    }

    /// Notes that the sitting is still playing at at, at percentage.
    static func tick(_ sitting: ListeningSitting, at: Date, percentage: Double) -> ListeningSitting {
        var sitting = sitting
        if sitting.isPlaying {
            sitting.lastActiveAt = max(sitting.lastActiveAt, at)
        }
        sitting.lastPercentage = percentage
        return sitting
    }

    /// Pauses the sitting at at. It goes on if playback resumes in time.
    static func pause(_ sitting: ListeningSitting, at: Date, percentage: Double) -> ListeningSitting {
        var sitting = sitting
        sitting.playedSeconds = sitting.played(at: at)
        sitting.playingSince = nil
        sitting.lastActiveAt = max(sitting.lastActiveAt, at)
        sitting.lastPercentage = percentage
        return sitting
    }

    /// Whether the sitting, paused, has been paused long enough at at to be
    /// over.
    static func isOver(_ sitting: ListeningSitting, at: Date) -> Bool {
        !sitting.isPlaying && at.timeIntervalSince(sitting.lastActiveAt) > splitAfter
    }

    /// The Reading Session the sitting makes so far, ending where it was
    /// last known to be going, or nil when nothing was played. Sent on every
    /// pause, and again when the sitting ends. at is now, for a sitting
    /// still playing; a sitting found after a relaunch ends at its last save.
    static func finish(_ sitting: ListeningSitting, at: Date? = nil) -> FinishedSession? {
        let end = sitting.isPlaying ? max(sitting.lastActiveAt, at ?? sitting.lastActiveAt) : sitting.lastActiveAt
        let played = sitting.played(at: end)
        guard played >= 1, end > sitting.startedAt else { return nil }
        return FinishedSession(
            sittingID: sitting.id, fileID: sitting.fileID, startedAt: sitting.startedAt, endedAt: end,
            startPercentage: sitting.startPercentage, endPercentage: sitting.lastPercentage,
            duration: Int(played.rounded())
        )
    }
}

/// A place in a File, as the server last said: seconds in an audiobook,
/// or a CFI and how far through in an ebook.
struct ServerPlace: Equatable, Sendable {
    let seconds: Double
    let occurredAt: Date
    let deviceID: String?
    let source: String
    /// The place in an ebook's text, or nil where the reader that left it
    /// gave only a percentage.
    var cfi: String?
    var percentage: Double = 0
    /// The 0-based page in a Page-based File, or nil where the reader that
    /// left it gave only a percentage.
    var page: Int?
    /// The Device's name as its user sees it, or nil for the web and for a
    /// Device deleted since.
    var deviceName: String?

    /// Whether other is this same move, whatever its Device is called now:
    /// renaming or deleting a Device sends its places again, changed only
    /// in the Device, and a place dismissed stays dismissed.
    func isSameMove(as other: ServerPlace?) -> Bool {
        guard let other else { return false }
        return seconds == other.seconds && occurredAt == other.occurredAt && source == other.source && cfi == other.cfi
            && page == other.page && percentage == other.percentage
    }
}

/// This installation's own place in a File.
struct LocalPlace: Equatable, Sendable {
    let seconds: Double
    let updatedAt: Date
    /// Moves not yet taken in by the server are waiting in the journal.
    let pending: Bool
    /// When the server's place was from, as this installation knew it when
    /// it first moved since its last sync, or nil when the server had none.
    let base: Date?
    /// Another device's place that arrived while this one had moves of its
    /// own to send, kept until the user picks one: once both sent theirs,
    /// the server holds only the newer.
    var offer: ServerPlace?
    /// The place in an ebook's text; seconds is 0 there.
    var cfi: String?
    var percentage: Double = 0
    /// The 0-based page in a Page-based File.
    var page: Int?
}

/// Where a File opens, across devices (#912): the newest synced place when
/// this device hasn't moved since its last sync; its own when it has, with
/// the other place offered when another device moved too.
enum PlaceRules {
    /// Places closer than this are the same place.
    static let sameWithin: Double = 5

    struct Choice: Equatable {
        let start: Double
        /// Another device's place to offer instead, without jumping.
        let offer: ServerPlace?
    }

    /// Places are compared by what this device knew of the server's place
    /// when it began moving (`LocalPlace.base`), never by the phone's clock
    /// against the server's.
    static func open(local: LocalPlace?, server: ServerPlace?, ownDeviceID: String) -> Choice {
        let decision = decide(local: local, server: server, ownDeviceID: ownDeviceID) { server, local in
            abs(server.seconds - local.seconds) > sameWithin
        }
        return Choice(start: decision.useServer ? server?.seconds ?? 0 : local?.seconds ?? 0, offer: decision.offer)
    }

    /// Where an ebook opens: a CFI in a Reflowable File, a 0-based page in
    /// a Page-based one, or how far through the book when the place has
    /// only a percentage, or nil for the start.
    enum EbookStart: Equatable, Sendable {
        case cfi(String)
        case page(Int)
        case fraction(Double)
    }

    struct EbookChoice: Equatable, Sendable {
        let start: EbookStart?
        /// How far through the book start is, which is where it opens when
        /// this copy of the book can't find the CFI.
        let percentage: Double
        let offer: ServerPlace?
    }

    /// The same rules for an ebook, whose places are CFIs, or pages in a
    /// Page-based File.
    static func openEbook(local: LocalPlace?, server: ServerPlace?, ownDeviceID: String) -> EbookChoice {
        let decision = decide(local: local, server: server, ownDeviceID: ownDeviceID) { server, local in
            ebookPlacesDiffer((server.cfi, server.percentage), (local.cfi, local.percentage), pages: (server.page, local.page))
        }
        if decision.useServer, let server {
            return EbookChoice(start: start(cfi: server.cfi, page: server.page, percentage: server.percentage),
                               percentage: server.percentage, offer: decision.offer)
        }
        guard let local else { return EbookChoice(start: nil, percentage: 0, offer: decision.offer) }
        return EbookChoice(start: start(cfi: local.cfi, page: local.page, percentage: local.percentage), percentage: local.percentage,
                           offer: decision.offer)
    }

    private static func start(cfi: String?, page: Int?, percentage: Double) -> EbookStart {
        if let page { return .page(page) }
        return cfi.map(EbookStart.cfi) ?? .fraction(percentage)
    }

    /// Ebook places this close are the same place: one page laid out on
    /// two screens has two CFIs.
    static let ebookSameWithin = 0.005

    /// pages are each place's page in a Page-based File: two pages are
    /// the same place only when they're the same page.
    static func ebookPlacesDiffer(_ a: (cfi: String?, percentage: Double), _ b: (cfi: String?, percentage: Double),
                                  pages: (Int?, Int?) = (nil, nil)) -> Bool {
        if let first = pages.0, let second = pages.1 { return first != second }
        if let cfi = a.cfi, cfi == b.cfi { return false }
        return abs(a.percentage - b.percentage) >= ebookSameWithin
    }

    /// Which side's place a File opens at, and the other device's place to
    /// offer instead. differs says whether a server place and a local one
    /// are different places.
    private static func decide(local: LocalPlace?, server: ServerPlace?, ownDeviceID: String,
                               differs: (ServerPlace, LocalPlace) -> Bool) -> (useServer: Bool, offer: ServerPlace?) {
        guard let server else { return (false, local?.offer) }
        guard let local else { return (true, nil) }
        if let offer = local.offer {
            return (false, differs(offer, local) ? offer : nil)
        }
        // Another device moved since this one began moving. A place this
        // device already knew is older than its own moves, as when the feed
        // hasn't brought back its last move yet.
        let movedElsewhere = server.deviceID != ownDeviceID && server.occurredAt > (local.base ?? .distantPast)
        if !local.pending {
            return (movedElsewhere, nil)
        }
        return (false, movedElsewhere && differs(server, local) ? server : nil)
    }

    /// A place that arrives while listening is only ever offered: when
    /// another device moved after the place the player knew when it opened
    /// the File, somewhere else.
    static func offerDuringListening(server: ServerPlace?, at seconds: Double, known: Date?, ownDeviceID: String) -> ServerPlace? {
        guard let server, server.deviceID != ownDeviceID, server.occurredAt > (known ?? .distantPast),
              abs(server.seconds - seconds) > sameWithin
        else { return nil }
        return server
    }

    /// The same for an ebook, where the reader is at a CFI, or at page in a
    /// Page-based File.
    static func offerWhileReading(server: ServerPlace?, at place: (cfi: String?, percentage: Double), page: Int? = nil, known: Date?,
                                  ownDeviceID: String) -> ServerPlace? {
        guard let server, server.deviceID != ownDeviceID, server.occurredAt > (known ?? .distantPast),
              ebookPlacesDiffer((server.cfi, server.percentage), place, pages: (server.page, page))
        else { return nil }
        return server
    }
}

/// A chapter of an audiobook, from Shisho's chapter list (ADR 0022).
struct AudioChapter: Equatable, Hashable, Sendable {
    let title: String
    let start: Double
}

enum AudioChapters {
    /// The index of the chapter playing at seconds, or nil without chapters.
    /// A chapter counts from a quarter second before its start, so a seek
    /// to its start lands in it.
    static func index(at seconds: Double, in chapters: [AudioChapter]) -> Int? {
        guard !chapters.isEmpty else { return nil }
        var found = 0
        for (index, chapter) in chapters.enumerated() where chapter.start <= seconds + 0.25 {
            found = index
        }
        return found
    }

    /// The chapter's name wherever the player shows it: its title, or its
    /// number when it has none. Never its number beside its title, which can
    /// disagree with it, as "2. Chapter 1" after an introduction.
    static func name(of index: Int, in chapters: [AudioChapter]) -> String {
        let title = chapters[index].title.trimmingCharacters(in: .whitespacesAndNewlines)
        return title.isEmpty ? String(localized: "Chapter \(index + 1)") : title
    }

    /// Where chapter index ends: the next one's start, or the end.
    static func end(of index: Int, in chapters: [AudioChapter], duration: Double) -> Double {
        index + 1 < chapters.count ? chapters[index + 1].start : duration
    }

    /// Where "previous chapter" goes from seconds: the start of this chapter,
    /// or of the one before when this one started under three seconds ago.
    static func previousTarget(from seconds: Double, in chapters: [AudioChapter]) -> Double {
        guard let index = index(at: seconds, in: chapters) else { return 0 }
        if seconds - chapters[index].start > 3 || index == 0 {
            return chapters[index].start
        }
        return chapters[index - 1].start
    }

    /// Where "next chapter" goes from seconds, or nil in the last chapter.
    static func nextTarget(from seconds: Double, in chapters: [AudioChapter]) -> Double? {
        guard let index = index(at: seconds, in: chapters), index + 1 < chapters.count else { return nil }
        return chapters[index + 1].start
    }
}

/// Playback speed, in the web player's 0.1 steps.
enum PlaybackSpeed {
    static let range: ClosedRange<Double> = 0.5...4.0
    static let step = 0.1

    static func clamp(_ rate: Double) -> Double {
        (min(range.upperBound, max(range.lowerBound, rate)) * 10).rounded() / 10
    }

    /// The speed as the web player writes it, such as "1.4x".
    static func label(_ rate: Double) -> String {
        String(format: "%.1fx", rate)
    }

    /// How long playing from seconds to end takes at rate, never below
    /// nothing, as the player's time left shows it.
    static func timeLeft(from seconds: Double, to end: Double, at rate: Double) -> Double {
        max(0, end - seconds) / rate
    }
}

enum TimeLabels {
    /// "4:05", or "1:04:05" from an hour.
    static func clock(_ seconds: Double) -> String {
        // Bounded, so a nonsense time from the server can't trap.
        let total = Int(min(max(0, seconds.isFinite ? seconds : 0), 1e9).rounded(.down))
        if total >= 3600 {
            return String(format: "%d:%02d:%02d", total / 3600, total / 60 % 60, total % 60)
        }
        return String(format: "%d:%02d", total / 60, total % 60)
    }
}
