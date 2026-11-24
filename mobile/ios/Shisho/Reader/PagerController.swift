import UIKit

/// What the pager shows and asks about: three pages, and the rules for
/// taps the page itself takes. foliate supplies them for Reflowable Files
/// (`FoliateSource`); the image pipeline will for Page-based ones (ADR 0025).
@MainActor
protocol PagerSource: AnyObject {
    /// The previous, current, and next pages' views, as first stacked.
    var pagerViews: (back: UIView, current: UIView, forward: UIView) { get }
    /// A turn committed: the source's own roles rotate the same way the
    /// pager's views did, and the page that came free is sent ahead.
    func pagerDidRotate(_ direction: TurnDirection)
    /// A tap on the current page, in its own points. True when the page
    /// took it, as a link does. resting says no turn is moving or waiting:
    /// taps in a run of quick turns are turns, not link taps.
    func pagerTakesTap(at point: CGPoint, resting: Bool) -> Bool
    /// A drag may not turn now, as while text is selected, when its handles
    /// are being dragged.
    var pagerBlocksDrag: Bool { get }
    /// A turn is about to start.
    func pagerWillTurn()
    /// Any input on the page, for timing reading.
    func pagerSawInput()
    /// The middle of the page was tapped.
    func pagerToggleChrome()
    /// A turn was asked for whose page isn't laid out yet, or the last one
    /// waiting turned or was dropped.
    func pagerWaiting(_ waiting: Bool)

    /// The pages zoom, as images do: a double tap zooms, and a drag takes
    /// one finger, leaving two for pinching. Read once, when attached.
    var pagerZooms: Bool { get }
    /// The current page is zoomed in: every tap waits to see whether it's
    /// a double tap, which zooms back out.
    var pagerIsZoomed: Bool { get }
    /// A double tap at point, in the current page's points.
    func pagerDoubleTapped(at point: CGPoint)
    /// Whether a drag starting sideways at velocityX turns, rather than
    /// panning a zoomed page that hasn't reached its edge that way.
    func pagerAllowsDrag(velocityX: CGFloat) -> Bool
}

extension PagerSource {
    var pagerZooms: Bool { false }
    var pagerIsZoomed: Bool { false }
    func pagerDoubleTapped(at point: CGPoint) {}
    func pagerAllowsDrag(velocityX: CGFloat) -> Bool { true }
}

/// The one native pager every File is read in (ADR 0026): pages follow the
/// finger and finish with a spring, edge taps turn, taps queue instead of
/// being dropped, and a page is never revealed before it's ready.
///
/// The three pages stay in the window, full size and unhidden, so the
/// covered ones keep painting. A drag moves the current page and the one it
/// brings in with layer transforms only. When a turn commits, the roles
/// rotate at once, so the page that came free is laid out ahead while the
/// turn still animates.
@MainActor
final class PagerController: UIViewController, UIGestureRecognizerDelegate {
    weak var source: PagerSource?
    /// Pages turn by dragging and tapping. Off in scrolled mode, the page
    /// scrolls itself, a tap only shows the controls, and a clearly
    /// sideways drag brings in the section beside.
    var paging = true {
        didSet {
            guard paging != oldValue else { return }
            settle()
        }
    }
    var rightToLeft = false
    /// The plain background shown where a page isn't ready.
    var backdropColor: UIColor = .white {
        didSet { backdrop.backgroundColor = backdropColor }
    }

    private(set) var turns = PagerTurns() {
        didSet {
            let waiting = turns.queued > 0
            if waiting != (oldValue.queued > 0) { source?.pagerWaiting(waiting) }
        }
    }
    private let stage = PagerStage()
    private let backdrop = UIView()
    private var back: UIView!
    private var current: UIView!
    private var forward: UIView!
    private var animator: UIViewPropertyAnimator?
    /// Which way the drag going on brings a page in, and whether that page
    /// was put under the finger rather than the backdrop.
    private var dragDirection: TurnDirection?
    /// A finger is dragging the pages sideways.
    private(set) var isDragging = false
    private var revealed = false
    private var touchDownAt: TimeInterval = 0
    private var panRecognizer: UIPanGestureRecognizer!
    /// For a source whose pages zoom: which taps are double taps, and the
    /// wait for a second tap.
    private var taps = PagerTaps()
    private var tapWait: Task<Void, Never>?

    /// How close to a link a tap goes to the link instead of turning.
    nonisolated static let linkSlop: CGFloat = 24
    /// A finger held this long before moving is WebKit's long press, which
    /// starts a selection rather than a turn. It's WebKit's own threshold,
    /// so any shorter hold still turns the page (the prototype refused
    /// drags from 0.45 s, which turned some slow starts into selections).
    static let longPress: TimeInterval = 0.5

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .clear
        stage.frame = view.bounds
        stage.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        stage.clipsToBounds = true
        stage.pager = self
        view.addSubview(stage)
        backdrop.backgroundColor = backdropColor
        backdrop.isHidden = true
        backdrop.frame = stage.bounds
        backdrop.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        stage.addSubview(backdrop)

        let tap = UITapGestureRecognizer(target: self, action: #selector(tapped(_:)))
        let pan = UIPanGestureRecognizer(target: self, action: #selector(panned(_:)))
        tap.delegate = self
        pan.delegate = self
        // On the stage rather than over it, so WebKit's own recognizers
        // (selection, the edit menu) see the same touches.
        stage.addGestureRecognizer(tap)
        stage.addGestureRecognizer(pan)
        panRecognizer = pan
    }

    /// Takes the source's three pages.
    func attach(_ source: PagerSource) {
        self.source = source
        loadViewIfNeeded()
        let views = source.pagerViews
        (back, current, forward) = (views.back, views.current, views.forward)
        for page in [back!, forward!, current!] {
            page.frame = stage.bounds
            page.autoresizingMask = [.flexibleWidth, .flexibleHeight]
            stage.addSubview(page)
        }
        // Pages that zoom pinch with two fingers, so a drag takes one.
        if source.pagerZooms { panRecognizer.maximumNumberOfTouches = 1 }
        updateAccessibility()
    }

    // MARK: - What the source tells the pager

    /// The page that way changed, as when it landed.
    func neighbor(_ direction: TurnDirection, is state: PagerTurns.Neighbor) {
        perform(turns.neighbor(direction, is: state))
    }

    /// A jump, a reflow, or a layout change: anything moving stops, and both
    /// neighbors are laid out again.
    func reset() {
        finishAnimation()
        turns.reset()
        settle()
    }

    // MARK: - Turning

    /// Turns a page, now or once the page that way is ready.
    func turn(_ direction: TurnDirection) {
        guard paging else { return }
        finishAnimation()
        perform(turns.ask(direction))
    }

    /// VoiceOver's scroll: a page paginated, the section beside scrolled,
    /// as a sideways drag brings in. False when there's nowhere to go yet.
    func accessibilityTurn(_ direction: TurnDirection) -> Bool {
        guard paging else { return slide(direction) }
        turn(direction)
        return true
    }

    private func perform(_ action: PagerTurns.Action) {
        guard case .turn(let direction) = action else { return }
        finishAnimation()
        source?.pagerWillTurn()
        prepare(direction)
        place(dx: 0, direction: direction)
        animate(direction, commit: true, velocity: 0)
    }

    /// Scrolled, a pull past a section's end or start: the section beside
    /// slides in from below or above over this one, which moves a little
    /// the other way. False when its page isn't ready.
    @discardableResult
    func slide(_ direction: TurnDirection) -> Bool {
        guard turns.available(direction) else { return false }
        finishAnimation()
        source?.pagerWillTurn()
        let height = stage.bounds.height
        let outgoing = current!
        let incoming = page(direction)
        let from: CGFloat = direction == .forward ? 1 : -1
        backdrop.isHidden = true
        stage.bringSubviewToFront(incoming)
        incoming.transform = CGAffineTransform(translationX: 0, y: from * height)
        let animator = UIViewPropertyAnimator(duration: 0.4, timingParameters: UISpringTimingParameters(dampingRatio: 1))
        animator.addAnimations {
            incoming.transform = .identity
            outgoing.transform = CGAffineTransform(translationX: 0, y: -from * height * 0.25)
        }
        animator.addCompletion { [weak self] _ in
            guard let self, self.animator === animator else { return }
            self.animator = nil
            self.settle()
        }
        self.animator = animator
        rotate(direction)
        animator.startAnimation()
        return true
    }

    private func page(_ direction: TurnDirection) -> UIView {
        direction == .forward ? forward : back
    }

    /// Which side of the screen the page that way comes in from: +1 the
    /// right, -1 the left.
    private func side(_ direction: TurnDirection) -> CGFloat {
        (direction == .forward) != rightToLeft ? 1 : -1
    }

    /// Puts the page that way right under the current one, or the plain
    /// backdrop when it isn't ready, so a drag never shows a stale page.
    private func prepare(_ direction: TurnDirection) {
        page(direction.opposite).transform = .identity
        revealed = turns.available(direction)
        if revealed {
            backdrop.isHidden = true
            stage.insertSubview(page(direction), belowSubview: current)
        } else {
            page(direction).transform = .identity
            backdrop.isHidden = false
            stage.insertSubview(backdrop, belowSubview: current)
        }
    }

    private func place(dx: CGFloat, direction: TurnDirection) {
        let width = stage.bounds.width
        current.transform = CGAffineTransform(translationX: dx, y: 0)
        if revealed {
            page(direction).transform = CGAffineTransform(translationX: dx + side(direction) * width, y: 0)
        }
    }

    private func animate(_ direction: TurnDirection, commit: Bool, velocity: CGFloat) {
        let width = stage.bounds.width
        let outgoing = current!
        let incoming = page(direction)
        let target = commit ? -side(direction) * width : 0
        let distance = target - outgoing.transform.tx
        let relative = distance == 0 ? 0 : max(-30, min(30, velocity / distance))
        let timing = UISpringTimingParameters(dampingRatio: 1, initialVelocity: CGVector(dx: relative, dy: 0))
        let animator = UIViewPropertyAnimator(duration: velocity == 0 ? 0.3 : 0.35, timingParameters: timing)
        let moveIncoming = revealed
        animator.addAnimations { [side = side(direction)] in
            outgoing.transform = CGAffineTransform(translationX: target, y: 0)
            if moveIncoming {
                incoming.transform = CGAffineTransform(translationX: target + side * width, y: 0)
            }
        }
        animator.addCompletion { [weak self] _ in
            guard let self, self.animator === animator else { return }
            self.animator = nil
            self.settle()
        }
        self.animator = animator
        if commit {
            // The page that comes free sits under both moving pages, so it
            // can start laying out its new page while the turn animates.
            rotate(direction)
        }
        animator.startAnimation()
    }

    private func rotate(_ direction: TurnDirection) {
        let old = current!
        if direction == .forward {
            (back, current, forward) = (old, forward, back)
        } else {
            (back, current, forward) = (forward, back, old)
        }
        turns.committed(direction)
        taps.turned(at: ProcessInfo.processInfo.systemUptime)
        updateAccessibility()
        source?.pagerDidRotate(direction)
    }

    /// Jumps a moving turn to its end, so nothing is ever left between two
    /// pages when the next one starts.
    private func finishAnimation() {
        guard let animator, animator.state == .active else { return }
        animator.stopAnimation(false)
        animator.finishAnimation(at: .end)
    }

    /// Every page square in the frame, the current one on top.
    private func settle() {
        guard current != nil, animator == nil else { return }
        for page in [back!, current!, forward!] {
            page.transform = .identity
        }
        stage.bringSubviewToFront(current)
        backdrop.isHidden = true
    }

    /// Whether every page is square in the frame with the current one on
    /// top: true whenever nothing is moving or waiting.
    var isSettled: Bool {
        guard current != nil else { return true }
        return [back!, current!, forward!].allSatisfy { $0.transform == .identity }
            && stage.subviews.last === current && backdrop.isHidden
    }

    var isResting: Bool { animator == nil && turns.queued == 0 }

    // MARK: - Gestures

    func gestureRecognizer(_ recognizer: UIGestureRecognizer, shouldReceive touch: UITouch) -> Bool {
        touchDownAt = touch.timestamp
        return true
    }

    func gestureRecognizerShouldBegin(_ recognizer: UIGestureRecognizer) -> Bool {
        // A press held still this long is WebKit's: selection, the edit menu.
        let held = ProcessInfo.processInfo.systemUptime - touchDownAt
        if held >= PagerController.longPress { return false }
        guard let pan = recognizer as? UIPanGestureRecognizer else { return true }
        guard source?.pagerBlocksDrag != true else { return false }
        let velocity = pan.velocity(in: stage)
        // A zoomed page pans to its edge before a drag turns.
        if abs(velocity.x) > abs(velocity.y), source?.pagerAllowsDrag(velocityX: velocity.x) == false { return false }
        // Scrolled, the page scrolls itself, and only a clearly sideways
        // drag is the pager's: it brings in the section beside, as a turn
        // brings in a page.
        if !paging { return abs(velocity.x) > abs(velocity.y) * 2 }
        return abs(velocity.x) > abs(velocity.y)
    }

    func gestureRecognizer(_ recognizer: UIGestureRecognizer,
                           shouldRecognizeSimultaneouslyWith other: UIGestureRecognizer) -> Bool {
        // Taps run alongside WebKit's, which clear a selection; a page drag
        // wins and cancels WebKit's touches.
        recognizer is UITapGestureRecognizer
    }

    @objc private func tapped(_ recognizer: UITapGestureRecognizer) {
        source?.pagerSawInput()
        let point = recognizer.location(in: stage)
        guard let source, source.pagerZooms else {
            single(at: point)
            return
        }
        // A double tap zooms anywhere on the page, so a tap that may be the
        // first of one waits a moment; taps in a run of turns don't.
        let turns = paging && !source.pagerIsZoomed
            && PagerZones.tap(at: point.x / max(1, stage.bounds.width), rightToLeft: rightToLeft) != .chrome
        let (decision, flush) = taps.tap(at: ProcessInfo.processInfo.systemUptime, point: point, turns: turns)
        if let flush { single(at: flush) }
        switch decision {
        case .single:
            single(at: point)
        case .double:
            tapWait?.cancel()
            source.pagerDoubleTapped(at: stage.convert(point, to: current))
        case .wait:
            tapWait?.cancel()
            tapWait = Task { [weak self] in
                try? await Task.sleep(for: .seconds(PagerTaps.doubleTapWindow))
                guard !Task.isCancelled, let self, let waiting = self.taps.expired() else { return }
                self.single(at: waiting)
            }
        }
    }

    /// A single tap at point, in the stage's points: the page's own, as a
    /// link, or a turn or the controls by where it lands.
    private func single(at point: CGPoint) {
        if source?.pagerTakesTap(at: point, resting: isResting) == true { return }
        guard paging else {
            source?.pagerToggleChrome()
            return
        }
        tap(atFraction: point.x / max(1, stage.bounds.width))
    }

    /// A tap at fraction of the width: the outer thirds turn, the middle
    /// shows the controls.
    func tap(atFraction fraction: CGFloat) {
        switch PagerZones.tap(at: fraction, rightToLeft: rightToLeft) {
        case .turn(let direction): turn(direction)
        case .chrome: source?.pagerToggleChrome()
        }
    }

    @objc private func panned(_ recognizer: UIPanGestureRecognizer) {
        let dx = recognizer.translation(in: stage).x
        switch recognizer.state {
        case .began:
            source?.pagerSawInput()
            dragBegan()
        case .changed:
            dragChanged(dx)
        case .ended, .cancelled, .failed:
            dragEnded(dx, velocity: recognizer.velocity(in: stage).x, completed: recognizer.state == .ended)
        default:
            break
        }
    }

    func dragBegan() {
        // A tap still waiting to see whether it's a double tap acts first,
        // as it came first.
        if let waiting = taps.expired() {
            tapWait?.cancel()
            single(at: waiting)
        }
        isDragging = true
        finishAnimation()
        turns.dragBegan()
        dragDirection = nil
    }

    func dragChanged(_ dx: CGFloat) {
        guard let direction = PagerZones.direction(dragX: dx, rightToLeft: rightToLeft) else { return }
        // A page that lands mid-drag is put under the finger then.
        if direction != dragDirection || turns.available(direction) != revealed {
            if direction != dragDirection { source?.pagerWillTurn() }
            dragDirection = direction
            prepare(direction)
        }
        // Without a page to bring in, the drag only gives a little.
        place(dx: revealed ? dx : dx * 0.2, direction: direction)
    }

    func dragEnded(_ dx: CGFloat, velocity: CGFloat, completed: Bool) {
        isDragging = false
        guard let direction = dragDirection else {
            _ = turns.dragEnded(.forward, asked: false, revealed: false)
            settle()
            // A page that landed while the finger was down can turn now.
            perform(turns.drain())
            return
        }
        dragDirection = nil
        let asked = completed && PagerZones.asksToTurn(dragX: dx, velocity: velocity, width: stage.bounds.width)
        let release = turns.dragEnded(direction, asked: asked, revealed: revealed)
        animate(direction, commit: release == .commit, velocity: velocity)
        perform(turns.drain())
    }

    private func updateAccessibility() {
        // VoiceOver reads the current page; the others are off screen.
        for page in [back, forward] {
            page?.accessibilityElementsHidden = true
        }
        current?.accessibilityElementsHidden = false
    }
}

/// The view the pages sit in. VoiceOver's three-finger scroll sideways
/// turns pages, or scrolled, goes to the chapter beside.
final class PagerStage: UIView {
    weak var pager: PagerController?

    override func accessibilityScroll(_ direction: UIAccessibilityScrollDirection) -> Bool {
        guard let pager else { return false }
        let rightToLeft = pager.rightToLeft
        switch direction {
        case .left: return pager.accessibilityTurn(rightToLeft ? .back : .forward)
        case .right: return pager.accessibilityTurn(rightToLeft ? .forward : .back)
        case .next: return pager.accessibilityTurn(.forward)
        case .previous: return pager.accessibilityTurn(.back)
        default: return false
        }
    }

    /// VoiceOver's two-finger double tap shows or hides the reader's bars,
    /// as a tap in the middle of the page does.
    override func accessibilityPerformMagicTap() -> Bool {
        guard let source = pager?.source else { return false }
        source.pagerToggleChrome()
        return true
    }
}
