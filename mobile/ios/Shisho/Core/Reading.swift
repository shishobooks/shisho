import Foundation

/// One Reading Session in an ebook, timed with the web reader's rules
/// (app/libraries/readingSession.ts): time counts per page, from 5 seconds
/// up to 2 minutes a page, and five idle minutes end the session.
///
/// Unlike the web reader's, it survives going to the background briefly:
/// backgrounding or locking ends it there, and coming back within 30
/// seconds goes on with the same session, without counting the time away.
struct ReadingSession: Equatable, Sendable {
    /// Names the session: every snapshot of it carries this, and the server
    /// keeps one session for it.
    var id: String = UUID().uuidString.lowercased()
    var fileID: Int
    var startedAt: Date
    var startPercentage: Double
    var lastPercentage: Double
    /// The last input: a turn, a tap, a scroll.
    var lastActivityAt: Date
    /// Seconds counted on pages already left.
    var countedSeconds: Double = 0
    /// The page showing, as the reader names it.
    var page: String
    /// Time on the page showing before pageSince, from before the app went
    /// away.
    var pageElapsed: Double = 0
    /// When the page showing started counting, or nil while the app is
    /// away.
    var pageSince: Date?
    /// When the app went away, while it's away.
    var awayAt: Date?

    /// Time on the page showing, up to at.
    func onPage(at: Date) -> Double {
        pageElapsed + (pageSince.map { max(0, at.timeIntervalSince($0)) } ?? 0)
    }
}

enum ReadingRules {
    static let idleAfter: TimeInterval = 5 * 60
    static let minPage: TimeInterval = 5
    static let maxPage: TimeInterval = 2 * 60
    /// Coming back from the background this soon goes on with the session.
    static let resumeWithin: TimeInterval = 30

    /// The time one visit to a page counts for.
    static func pageTime(_ seconds: Double) -> Double {
        seconds < minPage ? 0 : min(seconds, maxPage)
    }

    static func begin(fileID: Int, page: String, percentage: Double, at: Date) -> ReadingSession {
        ReadingSession(fileID: fileID, startedAt: at, startPercentage: percentage, lastPercentage: percentage,
                       lastActivityAt: at, page: page, pageSince: at)
    }

    /// The reader shows page now, after a turn or a scroll: the page left
    /// counts its time. The same page again only counts as input.
    static func turned(_ session: ReadingSession, to page: String, percentage: Double, at: Date) -> ReadingSession {
        var session = touched(session, at: at)
        session.lastPercentage = percentage
        guard page != session.page else { return session }
        session.countedSeconds += pageTime(session.onPage(at: at))
        session.page = page
        session.pageElapsed = 0
        session.pageSince = at
        return session
    }

    /// Input that isn't a turn, such as a tap that shows the controls.
    static func touched(_ session: ReadingSession, at: Date) -> ReadingSession {
        var session = session
        session.lastActivityAt = max(session.lastActivityAt, at)
        return session
    }

    static func isIdle(_ session: ReadingSession, at: Date) -> Bool {
        session.awayAt == nil && at.timeIntervalSince(session.lastActivityAt) > idleAfter
    }

    /// The session so far, ending at at, or nil when nothing counted yet.
    static func snapshot(_ session: ReadingSession, at: Date) -> FinishedSession? {
        let end = session.awayAt.map { min($0, at) } ?? at
        let counted = session.countedSeconds + pageTime(session.onPage(at: end))
        let duration = Int(counted.rounded())
        guard duration > 0, end > session.startedAt else { return nil }
        return FinishedSession(sittingID: session.id, fileID: session.fileID, startedAt: session.startedAt, endedAt: end,
                               startPercentage: session.startPercentage, endPercentage: session.lastPercentage,
                               duration: duration)
    }

    /// The session, idle five minutes, ends at its last input, or two
    /// minutes into the page when that's later, as on the web.
    static func endIdle(_ session: ReadingSession) -> FinishedSession? {
        let pageCap = session.pageSince.map { $0.addingTimeInterval(maxPage - session.pageElapsed) } ?? session.lastActivityAt
        return snapshot(session, at: max(session.lastActivityAt, pageCap))
    }

    /// The app went to the background, or the phone locked: the session
    /// ends here, and is kept a while in case the reader comes back.
    static func background(_ session: ReadingSession, at: Date) -> (ReadingSession, FinishedSession?) {
        guard session.awayAt == nil else { return (session, nil) }
        let ended = snapshot(session, at: at)
        var away = session
        away.pageElapsed = session.onPage(at: at)
        away.pageSince = nil
        away.awayAt = at
        return (away, ended)
    }

    /// The reader is back: the same session goes on when it was away under
    /// 30 seconds, or nil when a new one starts.
    static func foreground(_ session: ReadingSession, at: Date) -> ReadingSession? {
        guard let awayAt = session.awayAt else { return session }
        guard at.timeIntervalSince(awayAt) <= resumeWithin else { return nil }
        var session = session
        session.awayAt = nil
        session.pageSince = at
        session.lastActivityAt = at
        return session
    }
}
