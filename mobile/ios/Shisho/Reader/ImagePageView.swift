import UIKit

/// One screen of a Page-based File in the pager: a page, or a spread of two
/// side by side, fitted to the screen. Pinch or double-tap zooms it, and
/// only then is it drawn at full resolution. A zoomed page pans by drag
/// until it reaches its edge; from there a drag turns the page.
@MainActor
final class ImagePageView: UIScrollView, UIScrollViewDelegate {
    /// The pages shown, in reading order.
    private(set) var pages: [Int] = []
    /// Called when every page shown has its screen-size image, or couldn't
    /// be read: the page is ready to turn to.
    var onReady: ((ImagePageView) -> Void)?
    private(set) var isReady = false

    private let content = UIView()
    private var imageViews: [UIImageView] = []
    /// The pages left to right on screen, with their shapes.
    private var visual: [(page: Int, size: CGSize)] = []
    private var cache: PageImageCache?
    /// Bumped on each change of pages or size, so a decode that lands
    /// after one is dropped.
    private var token = 0
    private var fittedTo: CGSize = .zero
    /// Showing full-resolution images while zoomed.
    private var sharp = false
    /// Screen-size images still being decoded.
    private var waiting = 0

    static let maximumZoom: CGFloat = 4

    override init(frame: CGRect) {
        super.init(frame: frame)
        delegate = self
        backgroundColor = .clear
        showsHorizontalScrollIndicator = false
        showsVerticalScrollIndicator = false
        contentInsetAdjustmentBehavior = .never
        decelerationRate = .fast
        minimumZoomScale = 1
        maximumZoomScale = Self.maximumZoom
        bouncesZoom = true
        addSubview(content)
        // VoiceOver's swipe up and down zooms, for those who can't pinch.
        isAccessibilityElement = true
        accessibilityTraits = [.image, .adjustable]
        accessibilityHint = String(localized: "Swipe up or down to zoom")
    }

    /// How far in the page is zoomed, as VoiceOver says it.
    override var accessibilityValue: String? {
        get { Double(zoomScale).formatted(.percent.precision(.fractionLength(0))) }
        set {}
    }

    required init?(coder: NSCoder) { nil }

    var isZoomed: Bool { zoomScale > 1.01 }

    /// Shows pages, in reading order, with their shapes; right to left puts
    /// the first on the right. Empty pages show nothing, as past the end.
    func show(pages: [Int], sizes: [CGSize], rightToLeft: Bool, cache: PageImageCache, label: String) {
        token += 1
        self.pages = pages
        self.cache = cache
        let pairs = Array(zip(pages, sizes))
        visual = rightToLeft ? pairs.reversed() : pairs
        accessibilityLabel = label
        sharp = false
        isReady = false
        if zoomScale != 1 { setZoomScale(1, animated: false) }
        while imageViews.count < visual.count {
            let view = UIImageView()
            view.contentMode = .scaleAspectFit
            view.backgroundColor = .clear
            content.addSubview(view)
            imageViews.append(view)
        }
        while imageViews.count > visual.count { imageViews.removeLast().removeFromSuperview() }
        // A page kept already shows at once, rather than after a frame
        // of nothing.
        fittedTo = .zero
        for view in imageViews { view.image = nil }
        setNeedsLayout()
        layoutIfNeeded()
    }

    /// Back to the screen-size images and no zoom, as when the page leaves
    /// the screen.
    func resetZoom() {
        if zoomScale != 1 { setZoomScale(1, animated: false) }
        if sharp {
            sharp = false
            loadScreenImages()
        }
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        guard bounds.size != fittedTo, bounds.width > 0, bounds.height > 0, !isZooming else {
            center()
            return
        }
        fittedTo = bounds.size
        if zoomScale != 1 { setZoomScale(1, animated: false) }
        let frames = PageLayout.frames(sizes: visual.map(\.size), in: bounds.size)
        let union = frames.reduce(CGRect.null) { $0.union($1) }
        let area = union.isNull ? CGRect(origin: .zero, size: bounds.size) : union
        content.frame = CGRect(origin: .zero, size: area.size)
        for (view, frame) in zip(imageViews, frames) {
            view.frame = frame.offsetBy(dx: -area.minX, dy: -area.minY)
        }
        contentSize = area.size
        center()
        token += 1
        sharp = false
        loadScreenImages()
    }

    /// Keeps a page smaller than the screen in its middle.
    private func center() {
        let x = max(0, (bounds.width - contentSize.width) / 2)
        let y = max(0, (bounds.height - contentSize.height) / 2)
        let inset = UIEdgeInsets(top: y, left: x, bottom: y, right: x)
        if contentInset != inset { contentInset = inset }
    }

    private var screenScale: CGFloat { max(1, traitCollection.displayScale) }

    /// The pixels a page fills on screen at zoom.
    private func pixels(for view: UIImageView, zoom: CGFloat) -> Int {
        Int((max(view.bounds.width, view.bounds.height) * screenScale * zoom).rounded(.up))
    }

    private func loadScreenImages() {
        guard let cache, fittedTo != .zero else { return }
        let token = token
        guard !visual.isEmpty else {
            markReady()
            return
        }
        waiting = visual.count
        for (index, page) in visual.enumerated() {
            let view = imageViews[index]
            let maxPixel = pixels(for: view, zoom: 1)
            if let image = cache.cached(page: page.page, maxPixel: maxPixel) {
                view.image = image
                waiting -= 1
                continue
            }
            Task { [weak self] in
                let image = await cache.image(page: page.page, maxPixel: maxPixel, need: .now)
                guard let self, self.token == token else { return }
                view.image = image
                self.waiting -= 1
                if self.waiting == 0 { self.markReady() }
            }
        }
        if waiting == 0 { markReady() }
    }

    private func markReady() {
        guard !isReady else { return }
        isReady = true
        onReady?(self)
    }

    /// Full resolution for the zoomed page, held by this view alone.
    private func loadSharpImages() {
        guard let cache, !sharp else { return }
        sharp = true
        let token = token
        let zoom = zoomScale
        for (index, page) in visual.enumerated() {
            let view = imageViews[index]
            let maxPixel = pixels(for: view, zoom: zoom)
            Task { [weak self] in
                let image = await cache.image(page: page.page, maxPixel: maxPixel, need: .now, keep: false)
                guard let self, self.token == token, self.sharp, let image else { return }
                view.image = image
            }
        }
    }

    // MARK: - Zoom

    /// A double tap: zooms in to point, in this view's points, or back out
    /// when zoomed.
    func toggleZoom(at point: CGPoint) {
        if isZoomed {
            setZoomScale(1, animated: true)
            return
        }
        let inContent = convert(point, to: content)
        let scale: CGFloat = 2.5
        let size = CGSize(width: bounds.width / scale, height: bounds.height / scale)
        let x = min(max(inContent.x, 0), content.bounds.width)
        let y = min(max(inContent.y, 0), content.bounds.height)
        zoom(to: CGRect(x: x - size.width / 2, y: y - size.height / 2, width: size.width, height: size.height), animated: true)
    }

    /// Whether a sideways drag at velocityX is the pager's to turn with:
    /// always when not zoomed, and zoomed, once the page is at its edge that
    /// way.
    func yieldsDrag(velocityX: CGFloat) -> Bool {
        guard isZoomed else { return true }
        let leftmost = -contentInset.left
        let rightmost = contentSize.width + contentInset.right - bounds.width
        // A finger moving right shows more of the left.
        return velocityX > 0 ? contentOffset.x <= leftmost + 1 : contentOffset.x >= rightmost - 1
    }

    override func gestureRecognizerShouldBegin(_ recognizer: UIGestureRecognizer) -> Bool {
        if recognizer === panGestureRecognizer {
            guard isZoomed else { return false }
            let velocity = panGestureRecognizer.velocity(in: self)
            if abs(velocity.x) > abs(velocity.y), yieldsDrag(velocityX: velocity.x) { return false }
        }
        return super.gestureRecognizerShouldBegin(recognizer)
    }

    func viewForZooming(in scrollView: UIScrollView) -> UIView? { content }

    func scrollViewDidZoom(_ scrollView: UIScrollView) { center() }

    func scrollViewDidEndZooming(_ scrollView: UIScrollView, with view: UIView?, atScale scale: CGFloat) {
        if scale > 1.05 {
            // Sharper again for a deeper zoom.
            sharp = false
            loadSharpImages()
        } else if sharp {
            sharp = false
            loadScreenImages()
        }
    }

    // MARK: - VoiceOver

    override func accessibilityIncrement() {
        setZoomScale(min(maximumZoomScale, zoomScale * 2), animated: true)
    }

    override func accessibilityDecrement() {
        setZoomScale(max(minimumZoomScale, zoomScale / 2), animated: true)
    }
}

/// The scrolled Page flow: every page in one column, edge to edge, as a
/// webtoon is read. Pages decode as they near the screen, at the column's
/// width however tall they are.
///
/// One zooming scroll view holds the whole column, so a pan, zoomed or not,
/// is the scroll view's own in every direction, with its own momentum, as a
/// zoomed page pans in the paginated flow. Only the pages near the screen
/// have views; the rest of the column is space. A page coming on screen
/// while the column moves shows at the column's size, which is quick and
/// kept; once a zoomed column rests, the pages showing are drawn sharp.
@MainActor
final class PageStripView: UIView, UIScrollViewDelegate {
    /// The reader scrolled to another page or screen.
    var onMove: (() -> Void)?
    /// Called once the page it opened at has its image.
    var onReady: (() -> Void)?

    private let scroller = UIScrollView()
    /// What the scroller zooms: the column at the screen's width, as tall as
    /// every page together.
    private let column = UIView()
    private let sizes: [CGSize]
    private let cache: PageImageCache
    private let label: (Int) -> String
    /// Each page's place in the column, at its width.
    private var frames: [CGRect] = []
    /// The pages near the screen, with their views.
    private var tiles: [Int: Tile] = [:]
    /// Views of pages that went out of range, for the next to come in.
    private var spare: [Tile] = []
    /// The pages that had views at the last move, so a move within them,
    /// as most are, does nothing more.
    private var tiled: ClosedRange<Int>?
    /// A double tap's zoom is animating: page views are only added, and
    /// those that went out of range let go once it ends.
    private var zoomAnimating = false
    /// The page the reader is on: the one a quarter of the way down the
    /// screen, or the last once the column is scrolled to its end.
    private(set) var page = 0
    /// Which screen's worth down the column shows, for timing reading per
    /// screen as the web reader does: a strip can take several screens, or
    /// pass in a moment.
    private(set) var screen = 0
    private var pending: Int?
    private var laidOutSize: CGSize = .zero
    private var ready = false
    /// The strip is moving itself, as to a page: that's no scroll of the
    /// reader's.
    private var positioning = false
    /// When the column last moved, by a drag or its momentum.
    private var movedAt: TimeInterval = 0
    /// The last touch came down on a moving column, which it only stopped.
    private var touchStoppedScroll = false

    static let maximumZoom: CGFloat = 4
    var zoom: CGFloat { scroller.zoomScale }
    var isZoomed: Bool { scroller.zoomScale > 1.01 }

    init(sizes: [CGSize], cache: PageImageCache, startPage: Int, label: @escaping (Int) -> String) {
        self.sizes = sizes
        self.cache = cache
        self.label = label
        page = startPage
        pending = startPage
        super.init(frame: .zero)
        scroller.delegate = self
        scroller.backgroundColor = .clear
        scroller.showsHorizontalScrollIndicator = false
        scroller.showsVerticalScrollIndicator = false
        scroller.contentInsetAdjustmentBehavior = .never
        scroller.minimumZoomScale = 1
        scroller.maximumZoomScale = Self.maximumZoom
        // A pinch out past the screen's width would bounce back through
        // black at the edges: the column never needs to be narrower.
        scroller.bouncesZoom = false
        scroller.addSubview(column)
        addSubview(scroller)
    }

    required init?(coder: NSCoder) { nil }

    override func layoutSubviews() {
        super.layoutSubviews()
        guard bounds.width > 0, bounds.size != laidOutSize else { return }
        let widthChanged = bounds.width != laidOutSize.width
        laidOutSize = bounds.size
        let keep = pending ?? page
        // Laying out again moves the column; the reader didn't.
        positioning = true
        if scroller.zoomScale != 1 { scroller.setZoomScale(1, animated: false) }
        scroller.frame = bounds
        if widthChanged || frames.isEmpty { layOutColumn() }
        positioning = false
        if widthChanged || pending != nil {
            go(to: keep, reporting: false)
        } else {
            showTiles()
        }
    }

    /// Places every page down the column at the screen's width.
    private func layOutColumn() {
        let width = bounds.width
        var top: CGFloat = 0
        frames = sizes.map { size in
            let height = max(1, (width * size.height / max(size.width, 1)).rounded())
            defer { top += height }
            return CGRect(x: 0, y: top, width: width, height: height)
        }
        column.frame = CGRect(x: 0, y: 0, width: width, height: top)
        scroller.contentSize = column.frame.size
        for tile in tiles.values { recycle(tile) }
        tiles.removeAll()
        tiled = nil
    }

    /// Scrolls page to the top, or as far as the column goes. reporting is
    /// false when the reader stays where they were, as on rotation.
    func go(to page: Int, reporting: Bool = true) {
        guard sizes.indices.contains(page) else { return }
        guard laidOutSize.width > 0, frames.indices.contains(page) else {
            pending = page
            return
        }
        pending = nil
        positioning = true
        let bottom = max(0, scroller.contentSize.height - scroller.bounds.height)
        scroller.setContentOffset(CGPoint(x: scroller.contentOffset.x, y: min(frames[page].minY * zoom, bottom)), animated: false)
        positioning = false
        // The page asked for, even where the column's end stops it short of
        // the top.
        self.page = page
        screen = currentScreen
        showTiles()
        if reporting { onMove?() }
    }

    /// The part of the column on screen, in the column's own points.
    private var visibleColumn: CGRect {
        let scale = max(zoom, 0.01)
        return CGRect(x: scroller.contentOffset.x / scale, y: scroller.contentOffset.y / scale,
                      width: scroller.bounds.width / scale, height: scroller.bounds.height / scale)
    }

    private var currentScreen: Int {
        Int((visibleColumn.minY / max(1, bounds.height)).rounded())
    }

    /// The page at y down the column.
    private func page(at y: CGFloat) -> Int {
        var low = 0
        var high = frames.count - 1
        while low < high {
            let middle = (low + high + 1) / 2
            if frames[middle].minY <= y { low = middle } else { high = middle - 1 }
        }
        return max(0, low)
    }

    // MARK: - Taps

    override func hitTest(_ point: CGPoint, with event: UIEvent?) -> UIView? {
        // A touch on a column still moving, or just stopped by it, only
        // stops it: it's no tap, so it doesn't show the controls.
        if event?.type == .touches {
            touchStoppedScroll = ProcessInfo.processInfo.systemUptime - movedAt < 0.1
        }
        return super.hitTest(point, with: event)
    }

    /// Whether the tap ending now only stopped the column, which it
    /// answers once.
    func takeStoppedScroll() -> Bool {
        defer { touchStoppedScroll = false }
        return touchStoppedScroll
    }

    // MARK: - Moving

    func scrollViewDidScroll(_ scrollView: UIScrollView) {
        showTiles()
        guard !positioning else { return }
        movedAt = ProcessInfo.processInfo.systemUptime
        updatePlace()
    }

    func scrollViewDidEndDragging(_ scrollView: UIScrollView, willDecelerate decelerate: Bool) {
        if !decelerate { settled() }
    }

    func scrollViewDidEndDecelerating(_ scrollView: UIScrollView) {
        settled()
    }

    /// The column or its zoom is moving.
    private var isMoving: Bool {
        scroller.isDragging || scroller.isDecelerating || scroller.isZooming
    }

    /// Zoomed, the pages that came on screen while it moved show at the
    /// column's size; once it rests, they're drawn sharp for the zoom.
    private func settled() {
        guard isZoomed, !isMoving else { return }
        sharpenVisiblePages()
    }

    /// Finds the page and screen showing, and tells of a change.
    private func updatePlace() {
        guard pending == nil, laidOutSize.width > 0, !frames.isEmpty else { return }
        let visible = visibleColumn
        let atEnd = scroller.contentOffset.y + scroller.bounds.height >= scroller.contentSize.height - 1
        let item = atEnd ? sizes.count - 1 : page(at: visible.minY + visible.height * 0.25)
        let screen = currentScreen
        guard item != page || screen != self.screen else { return }
        page = item
        self.screen = screen
        onMove?()
    }

    // MARK: - Zoom

    /// Zooms to scale around point, in this view's points, animated as the
    /// pages zoom.
    func setZoom(_ scale: CGFloat, around point: CGPoint) {
        let scale = min(Self.maximumZoom, max(1, scale))
        // Only a zoom that changes ends, and lets go of the views again.
        guard abs(scale - zoom) > 0.01 else { return }
        zoomAnimating = true
        guard scale > 1.01 else {
            scroller.setZoomScale(1, animated: true)
            return
        }
        let center = convert(point, to: column)
        let size = CGSize(width: bounds.width / scale, height: bounds.height / scale)
        scroller.zoom(to: CGRect(x: center.x - size.width / 2, y: center.y - size.height / 2,
                                 width: size.width, height: size.height), animated: true)
    }

    /// A double tap at point: zooms in there, or back out when zoomed.
    func toggleZoom(at point: CGPoint) {
        setZoom(isZoomed ? 1 : 2.5, around: point)
    }

    func viewForZooming(in scrollView: UIScrollView) -> UIView? {
        column
    }

    func scrollViewDidEndZooming(_ scrollView: UIScrollView, with view: UIView?, atScale scale: CGFloat) {
        zoomAnimating = false
        tiled = nil
        showTiles()
        // Sharper zoomed in, and back to the column's size zoomed out.
        sharpenVisiblePages()
    }

    /// The pages showing, decoded again for the zoom.
    private func sharpenVisiblePages() {
        let visible = visibleColumn
        for (page, tile) in tiles where frames[page].intersects(visible) {
            load(tile, page: page, keepingShown: true)
        }
    }

    // MARK: - Pages

    /// Gives the pages near the screen their views, a screen's worth either
    /// side so a page has its image before it shows, and decodes a few
    /// more ahead at the column's size.
    private func showTiles() {
        guard !frames.isEmpty, pending == nil else { return }
        let visible = visibleColumn
        let first = page(at: max(0, visible.minY - visible.height))
        let last = page(at: visible.maxY + visible.height)
        guard tiled != first...last else { return }
        tiled = first...last
        // A page's view goes straight to its place. Within a double tap's
        // animated zoom, a view taken for another page would otherwise
        // slide there from the page it last showed, reshuffling the pages
        // on screen; and the range mid-animation may not be the one that
        // ends up showing, so none is let go until it ends.
        UIView.performWithoutAnimation {
            if !zoomAnimating {
                for (page, tile) in tiles where !(first...last).contains(page) {
                    recycle(tile)
                    tiles[page] = nil
                }
            }
            for page in first...last where tiles[page] == nil {
                let tile = spare.popLast() ?? makeTile()
                tile.page = page
                tile.frame = frames[page]
                tile.isHidden = false
                tile.accessibilityLabel = label(page)
                tiles[page] = tile
                load(tile, page: page, keepingShown: false)
            }
        }
        for page in [first - 2, first - 1, last + 1, last + 2, last + 3] where sizes.indices.contains(page) {
            cache.prefetch(page: page, maxPixel: pixels(page, zoom: 1), need: .soon)
        }
    }

    private func makeTile() -> Tile {
        let tile = Tile()
        tile.accessibilityCustomActions = [
            UIAccessibilityCustomAction(name: String(localized: "Zoom In")) { [weak self] _ in
                guard let self else { return false }
                self.setZoom(self.zoom * 2, around: CGPoint(x: self.bounds.midX, y: self.bounds.midY))
                return true
            },
            UIAccessibilityCustomAction(name: String(localized: "Zoom Out")) { [weak self] _ in
                guard let self else { return false }
                self.setZoom(self.zoom / 2, around: CGPoint(x: self.bounds.midX, y: self.bounds.midY))
                return true
            },
        ]
        column.addSubview(tile)
        return tile
    }

    private func recycle(_ tile: Tile) {
        tile.load?.cancel()
        tile.load = nil
        tile.image = nil
        tile.shownPixel = 0
        tile.page = -1
        tile.isHidden = true
        spare.append(tile)
    }

    /// The pixels the page fills, as its longer side, at zoom: the
    /// column's own by default.
    private func pixels(_ page: Int, zoom: CGFloat? = nil) -> Int {
        let size = sizes[page]
        let width = bounds.width * (zoom ?? self.zoom)
        let height = width * size.height / max(size.width, 1)
        return Int((max(width, height) * max(1, traitCollection.displayScale)).rounded(.up))
    }

    /// Puts page's image in tile at the zoom's size. Zoomed, a page is held
    /// only by its view, as a zoomed page in the paginated flow is: at four
    /// times the width, a few would fill the cache. While the column moves,
    /// a page coming on screen shows at the column's size rather than
    /// decoding a whole scan for the zoom mid-pan; settled() sharpens it.
    /// keepingShown leaves the image showing until the new one decodes.
    private func load(_ tile: Tile, page: Int, keepingShown: Bool) {
        let sharp = isZoomed && !isMoving
        let maxPixel = pixels(page, zoom: sharp ? nil : 1)
        // Already drawn at this size: nothing to do.
        if keepingShown, tile.page == page, tile.shownPixel == maxPixel, tile.image != nil { return }
        tile.load?.cancel()
        let keep = !sharp
        if keep, let image = cache.cached(page: page, maxPixel: maxPixel) {
            tile.image = image
            tile.shownPixel = maxPixel
            imageArrived(page)
            return
        }
        if !keepingShown {
            // The page at the column's width, when it's kept, shows until
            // the sharper one decodes, rather than nothing.
            tile.image = cache.cached(page: page, maxPixel: pixels(page, zoom: 1))
            tile.shownPixel = 0
        }
        tile.load = Task { [weak self, weak tile] in
            let image = await self?.cache.image(page: page, maxPixel: maxPixel, need: .now, keep: keep)
            guard let tile, tile.page == page, !Task.isCancelled, let image else { return }
            tile.image = image
            tile.shownPixel = maxPixel
            self?.imageArrived(page)
        }
    }

    private func imageArrived(_ page: Int) {
        guard !ready, page == self.page else { return }
        ready = true
        onReady?()
    }

    /// One page's view in the column.
    private final class Tile: UIImageView {
        var page = -1
        /// The size, in pixels, the image showing was decoded at.
        var shownPixel = 0
        /// The page's decode, cancelled when the view goes to another page,
        /// so a fling doesn't queue every page it passed.
        var load: Task<Void, Never>?

        init() {
            super.init(frame: .zero)
            contentMode = .scaleAspectFit
            isAccessibilityElement = true
            accessibilityTraits = .image
        }

        required init?(coder: NSCoder) { nil }
    }
}
