import UIKit

/// What the page reader hears from its pages.
@MainActor
protocol ImageSourceDelegate: AnyObject {
    /// The first page showed, which isn't reading.
    func imageSourceShowed(page: Int)
    /// The reader moved to page: a turn, a scroll, a jump.
    func imageSourceMoved(page: Int)
    /// A page turn is waiting for its page to decode.
    func imageSourceWaiting(_ waiting: Bool)
    func imageSourceSawInput()
    /// A page turn is starting, which hides the controls.
    func imageSourceTurning()
    func imageSourceToggleChrome()
}

/// The image pipeline as the pager's page source for Page-based Files
/// (ADR 0025): three screens, each a page or a spread, the current one and
/// one either side, decoded ahead so a turn never shows an empty page.
/// Scrolled, the current screen holds every page in one column instead.
@MainActor
final class ImageSource: PagerSource {
    /// A screen's place in the pager: a plain view the pager moves, holding
    /// the page view, which zooms inside it.
    final class Slot: UIView {
        let page = ImagePageView()
        var onResize: (() -> Void)?
        private var lastSize: CGSize = .zero

        override init(frame: CGRect) {
            super.init(frame: frame)
            // Opaque, as foliate's pages are: the screen settled under it
            // must not show around a page narrower than the screen.
            backgroundColor = .black
            page.frame = bounds
            page.autoresizingMask = [.flexibleWidth, .flexibleHeight]
            addSubview(page)
        }

        required init?(coder: NSCoder) { nil }

        override func layoutSubviews() {
            super.layoutSubviews()
            guard bounds.size != lastSize else { return }
            lastSize = bounds.size
            onResize?()
        }
    }

    weak var delegate: ImageSourceDelegate?
    let pager = PagerController()
    let cache: PageImageCache
    let pageCount: Int
    private let sizes: [CGSize]
    private var back = Slot()
    private var current = Slot()
    private var forward = Slot()
    private(set) var flow: PageFlow
    private(set) var rightToLeft: Bool
    /// The screens: pages paired into spreads in landscape.
    private(set) var spreads: [[Int]] = []
    private var spreadIndex = 0
    private var twoUp = false
    /// The scrolled column, while scrolled.
    private var strip: PageStripView?
    /// The page shown first.
    private var startPage: Int
    private var opened = false
    private var reconcileScheduled = false
    /// The page a jump or a rotation went to, which stays the reader's
    /// place while its spread shows, so rotating back returns to it rather
    /// than to the spread's first page.
    private var anchorPage: Int?
    /// The page last told to the delegate.
    private var reportedPage: Int?

    init(pages: PageImageSource, sizes: [CGSize], flow: PageFlow, rightToLeft: Bool, startPage: Int) {
        cache = PageImageCache(source: pages)
        pageCount = sizes.count
        self.sizes = sizes
        self.flow = flow
        self.rightToLeft = rightToLeft
        self.startPage = min(max(0, startPage), max(0, sizes.count - 1))
        for slot in [back, current, forward] {
            slot.page.onReady = { [weak self] view in self?.pageReady(view) }
            slot.onResize = { [weak self, weak slot] in
                guard let self, let slot, slot === self.current else { return }
                self.currentResized()
            }
        }
        pager.backdropColor = .black
        pager.rightToLeft = rightToLeft
        pager.paging = flow == .paginated
        pager.attach(self)
    }

    /// The page the reader is on: a spread's first, or the last page when
    /// the spread shows it.
    var currentPage: Int {
        if let strip { return strip.page }
        guard spreads.indices.contains(spreadIndex) else { return startPage }
        let spread = spreads[spreadIndex]
        if let anchorPage, spread.contains(anchorPage), !spread.contains(pageCount - 1) { return anchorPage }
        return PageLayout.place(of: spread, pageCount: pageCount)
    }

    /// The pages on screen, in reading order.
    var pagesShown: [Int] {
        if let strip { return [strip.page] }
        return spreads.indices.contains(spreadIndex) ? spreads[spreadIndex] : []
    }

    // MARK: - Laying out

    /// The screen's size is known, or changed, as on rotation: landscape
    /// pairs pages into spreads, keeping the page the reader was on.
    private func currentResized() {
        let size = current.bounds.size
        guard size.width > 0, size.height > 0 else { return }
        let landscape = size.width > size.height
        if !opened {
            opened = true
            twoUp = landscape
            spreads = PageLayout.spreads(sizes: sizes, twoUp: twoUp)
            show(page: startPage)
            return
        }
        guard landscape != twoUp else { return }
        let keep = currentPage
        twoUp = landscape
        spreads = PageLayout.spreads(sizes: sizes, twoUp: twoUp)
        if flow == .paginated { show(page: keep) }
    }

    /// Goes to page: every screen is laid out again around it.
    func jump(to page: Int) {
        guard opened else {
            startPage = min(max(0, page), max(0, pageCount - 1))
            return
        }
        show(page: page)
        reportMove()
    }

    /// Tells the delegate the reader is on another page, once the first
    /// page showed: where the reader opened isn't a move.
    private func reportMove() {
        let page = currentPage
        guard showedFirst, page != reportedPage else { return }
        reportedPage = page
        delegate?.imageSourceMoved(page: page)
    }

    private func show(page: Int) {
        let page = min(max(0, page), max(0, pageCount - 1))
        anchorPage = page
        pager.reset()
        if flow == .scrolled {
            showStrip(at: page)
        } else {
            removeStrip()
            spreadIndex = PageLayout.spread(of: page, in: spreads)
            load(current, spread: spreadIndex)
            load(back, spread: spreadIndex - 1)
            load(forward, spread: spreadIndex + 1)
        }
        scheduleReconcile()
        prefetch()
    }

    private func label(for pages: [Int]) -> String {
        switch pages.count {
        case 0: ""
        case 1: String(localized: "Page \(pages[0] + 1) of \(pageCount)")
        default: String(localized: "Pages \(pages[0] + 1) and \(pages[1] + 1) of \(pageCount)")
        }
    }

    private func load(_ slot: Slot, spread index: Int) {
        let pages = spreads.indices.contains(index) ? spreads[index] : []
        slot.page.show(pages: pages, sizes: pages.map { sizes[$0] }, rightToLeft: rightToLeft, cache: cache, label: label(for: pages))
    }

    /// A flow or Reading Direction chosen while reading, keeping the page.
    func apply(flow: PageFlow, rightToLeft: Bool) {
        guard flow != self.flow || rightToLeft != self.rightToLeft else { return }
        let keep = currentPage
        self.flow = flow
        self.rightToLeft = rightToLeft
        pager.rightToLeft = rightToLeft
        pager.paging = flow == .paginated
        guard opened else { return }
        show(page: keep)
    }

    private func showStrip(at page: Int) {
        for slot in [back, forward] { slot.page.show(pages: [], sizes: [], rightToLeft: rightToLeft, cache: cache, label: "") }
        current.page.show(pages: [], sizes: [], rightToLeft: rightToLeft, cache: cache, label: "")
        current.page.isHidden = true
        if let strip {
            strip.go(to: page)
            return
        }
        let strip = PageStripView(sizes: sizes, cache: cache, startPage: page) { [weak self] in
            self?.label(for: [$0]) ?? ""
        }
        strip.frame = current.bounds
        strip.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        strip.onPage = { [weak self] _ in self?.reportMove() }
        strip.onReady = { [weak self] in self?.currentShowed() }
        current.addSubview(strip)
        self.strip = strip
    }

    private func removeStrip() {
        strip?.removeFromSuperview()
        strip = nil
        current.page.isHidden = false
    }

    // MARK: - Readiness

    private func pageReady(_ view: ImagePageView) {
        if view === current.page {
            currentShowed()
        } else {
            scheduleReconcile()
        }
    }

    private var showedFirst = false

    private func currentShowed() {
        guard !showedFirst else { return }
        showedFirst = true
        reportedPage = currentPage
        delegate?.imageSourceShowed(page: currentPage)
    }

    /// Tells the pager where each neighbor stands, after the current change
    /// is done: telling it while a turn commits could start a queued turn
    /// inside that one.
    private func scheduleReconcile() {
        guard !reconcileScheduled else { return }
        reconcileScheduled = true
        Task { @MainActor [weak self] in
            guard let self else { return }
            self.reconcileScheduled = false
            self.reconcile()
        }
    }

    private func reconcile() {
        for (direction, slot) in [(TurnDirection.back, back), (.forward, forward)] {
            let index = spreadIndex + direction.rawValue
            if flow == .scrolled || !spreads.indices.contains(index) {
                pager.neighbor(direction, is: .edge)
            } else if slot.page.isReady, slot.page.pages == spreads[index] {
                pager.neighbor(direction, is: .ready)
            }
        }
    }

    /// Decodes the screens two either side ahead, and stops decoding any
    /// the reader moved away from.
    private func prefetch() {
        guard flow == .paginated, spreads.indices.contains(spreadIndex) else {
            cache.cancel(except: Set(pagesShown))
            return
        }
        var keep = Set<Int>()
        let box = current.bounds.size
        let scale = max(1, current.traitCollection.displayScale)
        for offset in [0, 1, -1, 2, -2] {
            let index = spreadIndex + offset
            guard spreads.indices.contains(index) else { continue }
            let pages = spreads[index]
            keep.formUnion(pages)
            guard abs(offset) == 2, box.width > 0 else { continue }
            for (page, frame) in zip(pages, PageLayout.frames(sizes: pages.map { sizes[$0] }, in: box)) {
                cache.prefetch(page: page, maxPixel: Int((max(frame.width, frame.height) * scale).rounded(.up)), need: .soon)
            }
        }
        cache.cancel(except: keep)
    }

    func memoryWarning() {
        cache.purge()
    }

    // MARK: - PagerSource

    var pagerViews: (back: UIView, current: UIView, forward: UIView) {
        (back, current, forward)
    }

    func pagerDidRotate(_ direction: TurnDirection) {
        let old = current
        if direction == .forward {
            (back, current, forward) = (old, forward, back)
        } else {
            (back, current, forward) = (forward, back, old)
        }
        old.page.resetZoom()
        spreadIndex += direction.rawValue
        anchorPage = nil
        // The screen that came free goes on ahead.
        load(direction == .forward ? forward : back, spread: spreadIndex + direction.rawValue)
        scheduleReconcile()
        prefetch()
        reportMove()
    }

    func pagerTakesTap(at point: CGPoint, resting: Bool) -> Bool {
        // Zoomed, a tap anywhere shows the controls rather than turning
        // away from what the reader zoomed into.
        guard flow == .paginated, current.page.isZoomed else { return false }
        delegate?.imageSourceToggleChrome()
        return true
    }

    var pagerBlocksDrag: Bool { false }

    func pagerWillTurn() {
        delegate?.imageSourceTurning()
    }

    func pagerSawInput() {
        delegate?.imageSourceSawInput()
    }

    func pagerToggleChrome() {
        delegate?.imageSourceToggleChrome()
    }

    func pagerWaiting(_ waiting: Bool) {
        delegate?.imageSourceWaiting(waiting)
    }

    var pagerZooms: Bool { true }

    var pagerIsZoomed: Bool { flow == .paginated && current.page.isZoomed }

    func pagerDoubleTapped(at point: CGPoint) {
        guard flow == .paginated else { return }
        current.page.toggleZoom(at: current.convert(point, to: current.page))
    }

    func pagerAllowsDrag(velocityX: CGFloat) -> Bool {
        // Scrolled, the column scrolls, and there's no screen beside it.
        flow == .paginated && current.page.yieldsDrag(velocityX: velocityX)
    }
}
