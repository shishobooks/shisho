import CoreGraphics
import Foundation

/// Which way a page turn goes through the book.
enum TurnDirection: Int, Sendable {
    case back = -1
    case forward = 1

    var opposite: TurnDirection { self == .forward ? .back : .forward }
}

/// When the pager turns (ADR 0026). It holds no views: the pager asks it
/// what a tap, a release, or a page landing means, and does that.
///
/// The pages on either side of the current one are laid out ahead. A turn
/// toward a page that isn't ready yet queues and happens as soon as the page
/// lands, so fast taps and flicks are never dropped, and a page is never
/// revealed before it's ready.
struct PagerTurns: Equatable, Sendable {
    /// A page next to the current one.
    enum Neighbor: Equatable, Sendable {
        /// Being laid out.
        case pending
        /// Laid out and painted, one page from the current one.
        case ready
        /// The book has no page this way.
        case edge
    }

    enum Action: Equatable, Sendable {
        case none
        case turn(TurnDirection)
    }

    enum Release: Equatable, Sendable {
        case commit
        case springBack
    }

    private var back = Neighbor.pending
    private var forward = Neighbor.pending
    /// Turns asked for while the page that way was still being laid out,
    /// signed by direction.
    private(set) var queued = 0
    private var dragging = false

    func state(_ direction: TurnDirection) -> Neighbor {
        direction == .forward ? forward : back
    }

    func available(_ direction: TurnDirection) -> Bool {
        state(direction) == .ready
    }

    private mutating func set(_ direction: TurnDirection, _ state: Neighbor) {
        if direction == .forward { forward = state } else { back = state }
    }

    /// A tap or a key asks for a turn. It turns now, or queues behind the
    /// turns already waiting. Every tap counts: one the other way cancels a
    /// queued turn, and turns back once it outnumbers them.
    mutating func ask(_ direction: TurnDirection) -> Action {
        if queued == 0 {
            if available(direction) { return .turn(direction) }
            if state(direction) == .edge { return .none }
        }
        queued += direction.rawValue
        return drain()
    }

    /// The page that way changed, as when it landed: a turn waiting for it
    /// happens now.
    mutating func neighbor(_ direction: TurnDirection, is state: Neighbor) -> Action {
        set(direction, state)
        return drain()
    }

    /// The next queued turn, when its page is ready and no finger is down.
    mutating func drain() -> Action {
        guard queued != 0, !dragging else { return .none }
        let direction: TurnDirection = queued > 0 ? .forward : .back
        if state(direction) == .edge {
            queued = 0
            return .none
        }
        guard available(direction) else { return .none }
        queued -= direction.rawValue
        return .turn(direction)
    }

    mutating func dragBegan() {
        dragging = true
    }

    /// A drag let go. asked is whether it went far or fast enough to turn,
    /// and revealed whether the page that way was put under the finger,
    /// rather than the plain backdrop shown while it was laid out: a turn
    /// only commits onto what was revealed. A turn asked for onto a page
    /// still being laid out queues, as a tap does; a drag that springs back
    /// leaves the taps queued before it.
    mutating func dragEnded(_ direction: TurnDirection, asked: Bool, revealed: Bool) -> Release {
        dragging = false
        let commit = asked && revealed && available(direction)
        if asked, !commit, state(direction) != .edge {
            queued += direction.rawValue
        }
        return commit ? .commit : .springBack
    }

    /// A turn committed: the old current page is the ready one behind, and
    /// the page that was behind it is moved ahead, to be laid out again.
    mutating func committed(_ direction: TurnDirection) {
        set(direction.opposite, .ready)
        set(direction, .pending)
    }

    /// A jump, a reflow, or a new book: both neighbors are laid out again,
    /// and nothing queued still applies.
    mutating func reset() {
        back = .pending
        forward = .pending
        queued = 0
    }
}

/// Where taps and drags turn, by Reading Direction: in a right-to-left
/// book the left edge goes forward.
enum PagerZones {
    enum Tap: Equatable {
        case turn(TurnDirection)
        case chrome
    }

    /// A tap at fraction of the page's width: the outer thirds turn, the
    /// middle shows or hides the controls.
    static func tap(at fraction: CGFloat, rightToLeft: Bool) -> Tap {
        if fraction < 1.0 / 3 { return .turn(rightToLeft ? .forward : .back) }
        if fraction > 2.0 / 3 { return .turn(rightToLeft ? .back : .forward) }
        return .chrome
    }

    /// How far from a link a tap at fraction of the page's width still
    /// follows it. In the edge thirds, where a tap turns the page, only a
    /// tap on the link does: a page of links, as a book's own contents, would
    /// otherwise leave nowhere to turn. A note reference, too small to hit,
    /// keeps its reach everywhere (FoliateSource).
    static func linkSlop(at fraction: CGFloat) -> CGFloat {
        fraction < 1.0 / 3 || fraction > 2.0 / 3 ? 0 : PagerController.linkSlop
    }

    /// The page a drag brings in: a finger moving left brings in the page on
    /// the right.
    static func direction(dragX: CGFloat, rightToLeft: Bool) -> TurnDirection? {
        guard dragX != 0 else { return nil }
        return (dragX < 0) != rightToLeft ? .forward : .back
    }

    /// Whether a release asks to turn: past a quarter of the width, or a
    /// flick the way it was dragged, and never a flick back.
    static func asksToTurn(dragX: CGFloat, velocity: CGFloat, width: CGFloat) -> Bool {
        let flick: CGFloat = 300
        if dragX < 0 {
            return (dragX < -width * 0.25 || velocity < -flick) && velocity < flick
        }
        return (dragX > width * 0.25 || velocity > flick) && velocity > -flick
    }
}

/// When a tap on a page that zooms is a tap and when it's half of a double
/// tap, which zooms anywhere on the page. A tap waits a moment to see
/// whether a second follows close by; taps in a run of turns, as when
/// tapping through pages quickly, act at once, so tapping through stays
/// quick and only the first turn of a run waits.
struct PagerTaps: Sendable {
    /// How soon a second tap has to follow for a double tap, which is also
    /// how long a single tap waits: well under the system's, so a tap that
    /// turns a page doesn't feel slow.
    static let doubleTapWindow: TimeInterval = 0.175
    /// How near the first a second tap has to land.
    static let doubleTapReach: CGFloat = 50
    /// How long after a turn a tap that turns still acts at once.
    static let turnRun: TimeInterval = 0.8

    enum Decision: Equatable, Sendable {
        /// A tap: act on it now.
        case single
        /// Maybe the first of a double tap: act on it once the window
        /// passes with no second (`expired`).
        case wait
        /// The second of a double tap.
        case double
    }

    private var pending: (time: TimeInterval, point: CGPoint)?
    private var lastTurn: TimeInterval?

    /// A tap at point, at time. turns says whether it would turn a page on
    /// its own. flush is an earlier tap still waiting, which isn't half of
    /// a double tap with this one: act on it first.
    mutating func tap(at time: TimeInterval, point: CGPoint, turns: Bool) -> (decision: Decision, flush: CGPoint?) {
        var flush: CGPoint?
        if let pending {
            self.pending = nil
            let near = hypot(point.x - pending.point.x, point.y - pending.point.y) <= Self.doubleTapReach
            if time - pending.time <= Self.doubleTapWindow, near { return (.double, nil) }
            flush = pending.point
        }
        if turns, let lastTurn, time - lastTurn <= Self.turnRun { return (.single, flush) }
        pending = (time, point)
        return (.wait, flush)
    }

    /// The window passed with no second tap: the tap waiting, to act on
    /// now, if there is one.
    mutating func expired() -> CGPoint? {
        defer { pending = nil }
        return pending?.point
    }

    /// A tap turned a page.
    mutating func turned(at time: TimeInterval) {
        lastTurn = time
    }
}
