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
/// Pinch or double-tap zooms the column as the paginated pages zoom: it sits
/// in a zooming scroll view, which scales it and pans it in every direction,
/// while the column itself still scrolls down through the pages. Once a zoom
/// settles, the pages showing decode sharper for it.
@MainActor
final class PageStripView: UIView, UICollectionViewDataSource, UICollectionViewDelegateFlowLayout,
    UICollectionViewDataSourcePrefetching {
    /// The reader scrolled to another page or screen.
    var onMove: (() -> Void)?
    /// Called once the page it opened at has its image.
    var onReady: (() -> Void)?

    private let collection: UICollectionView
    /// Zooms and pans the column.
    private let zoomer = UIScrollView()
    /// What the zoomer scales: the column, at the screen's size.
    private let content = UIView()
    private let sizes: [CGSize]
    private let cache: PageImageCache
    private let label: (Int) -> String
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
    var zoom: CGFloat { zoomer.zoomScale }
    var isZoomed: Bool { zoomer.zoomScale > 1.01 }

    init(sizes: [CGSize], cache: PageImageCache, startPage: Int, label: @escaping (Int) -> String) {
        self.sizes = sizes
        self.cache = cache
        self.label = label
        page = startPage
        pending = startPage
        let layout = UICollectionViewFlowLayout()
        layout.scrollDirection = .vertical
        layout.minimumLineSpacing = 0
        layout.minimumInteritemSpacing = 0
        collection = UICollectionView(frame: .zero, collectionViewLayout: layout)
        super.init(frame: .zero)
        collection.backgroundColor = .clear
        collection.showsVerticalScrollIndicator = false
        collection.contentInsetAdjustmentBehavior = .never
        collection.register(StripCell.self, forCellWithReuseIdentifier: "page")
        collection.dataSource = self
        collection.delegate = self
        collection.prefetchDataSource = self
        collection.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        zoomer.delegate = self
        zoomer.showsHorizontalScrollIndicator = false
        zoomer.showsVerticalScrollIndicator = false
        zoomer.contentInsetAdjustmentBehavior = .never
        zoomer.minimumZoomScale = 1
        zoomer.maximumZoomScale = Self.maximumZoom
        // A pinch out past the screen's width would bounce back through
        // black at the edges: the column never needs to be narrower.
        zoomer.bouncesZoom = false
        zoomer.decelerationRate = .fast
        // Unzoomed there's nothing to pan, and its drag would take the
        // column's.
        zoomer.isScrollEnabled = false
        content.addSubview(collection)
        zoomer.addSubview(content)
        addSubview(zoomer)
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
        if zoomer.zoomScale != 1 { zoomer.setZoomScale(1, animated: false) }
        zoomer.frame = bounds
        content.frame = CGRect(origin: .zero, size: bounds.size)
        collection.frame = content.bounds
        zoomer.contentSize = bounds.size
        collection.collectionViewLayout.invalidateLayout()
        collection.layoutIfNeeded()
        positioning = false
        if widthChanged || pending != nil { go(to: keep, reporting: false) }
    }

    /// Scrolls page to the top, or as far as the column goes. reporting is
    /// false when the reader stays where they were, as on rotation.
    func go(to page: Int, reporting: Bool = true) {
        guard sizes.indices.contains(page) else { return }
        guard laidOutSize.width > 0 else {
            pending = page
            return
        }
        pending = nil
        positioning = true
        collection.layoutIfNeeded()
        if let frame = collection.layoutAttributesForItem(at: IndexPath(item: page, section: 0))?.frame {
            let bottom = max(0, collection.contentSize.height - collection.bounds.height)
            collection.setContentOffset(CGPoint(x: 0, y: min(frame.minY, bottom)), animated: false)
        }
        positioning = false
        // The page asked for, even where the column's end stops it short of
        // the top.
        self.page = page
        screen = currentScreen
        if reporting { onMove?() }
    }

    private var currentScreen: Int {
        let height = max(1, collection.bounds.height)
        return Int((collection.contentOffset.y / height).rounded())
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

    // MARK: - Zoom

    /// Zooms to scale around point, in this view's points, animated as the
    /// pages zoom.
    func setZoom(_ scale: CGFloat, around point: CGPoint) {
        let scale = min(Self.maximumZoom, max(1, scale))
        guard scale > 1.01 else {
            zoomer.setZoomScale(1, animated: true)
            return
        }
        let center = convert(point, to: content)
        let size = CGSize(width: bounds.width / scale, height: bounds.height / scale)
        zoomer.zoom(to: CGRect(x: center.x - size.width / 2, y: center.y - size.height / 2,
                               width: size.width, height: size.height), animated: true)
    }

    /// A double tap at point: zooms in there, or back out when zoomed.
    func toggleZoom(at point: CGPoint) {
        setZoom(isZoomed ? 1 : 2.5, around: point)
    }

    func viewForZooming(in scrollView: UIScrollView) -> UIView? {
        scrollView === zoomer ? content : nil
    }

    func scrollViewDidEndZooming(_ scrollView: UIScrollView, with view: UIView?, atScale scale: CGFloat) {
        guard scrollView === zoomer else { return }
        zoomer.isScrollEnabled = isZoomed
        sharpenVisiblePages()
    }

    /// The pages showing, decoded again for the zoom: sharper zoomed in,
    /// and back to the screen's size zoomed out.
    private func sharpenVisiblePages() {
        for case let cell as StripCell in collection.visibleCells {
            load(cell, page: cell.page, keepingShown: true)
        }
    }

    // MARK: - Pages

    /// The pixels the page fills, as its longer side, at zoom: the
    /// column's own by default.
    private func pixels(_ page: Int, zoom: CGFloat? = nil) -> Int {
        let size = sizes[page]
        let width = bounds.width * (zoom ?? self.zoom)
        let height = width * size.height / max(size.width, 1)
        return Int((max(width, height) * max(1, traitCollection.displayScale)).rounded(.up))
    }

    func collectionView(_ collectionView: UICollectionView, numberOfItemsInSection section: Int) -> Int { sizes.count }

    func collectionView(_ collectionView: UICollectionView, cellForItemAt indexPath: IndexPath) -> UICollectionViewCell {
        let cell = collectionView.dequeueReusableCell(withReuseIdentifier: "page", for: indexPath) as! StripCell
        let page = indexPath.item
        cell.page = page
        cell.accessibilityLabel = label(page)
        cell.accessibilityCustomActions = [
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
        load(cell, page: page, keepingShown: false)
        return cell
    }

    /// Puts page's image in cell at the zoom's size. Zoomed, a page is held
    /// only by its cell, as a zoomed page in the paginated flow is: at four
    /// times the width, a few would fill the cache. keepingShown leaves the
    /// image showing until the new one decodes.
    private func load(_ cell: StripCell, page: Int, keepingShown: Bool) {
        cell.load?.cancel()
        let maxPixel = pixels(page)
        let keep = !isZoomed
        if keep, let image = cache.cached(page: page, maxPixel: maxPixel) {
            cell.imageView.image = image
            imageArrived(page)
            return
        }
        if !keepingShown {
            // The page at the column's unzoomed width, when it's kept,
            // shows until the sharper one decodes, rather than nothing.
            cell.imageView.image = cache.cached(page: page, maxPixel: pixels(page, zoom: 1))
        }
        cell.load = Task { [weak self, weak cell] in
            let image = await self?.cache.image(page: page, maxPixel: maxPixel, need: .now, keep: keep)
            guard let cell, cell.page == page, !Task.isCancelled, let image else { return }
            cell.imageView.image = image
            self?.imageArrived(page)
        }
    }

    private func imageArrived(_ page: Int) {
        guard !ready, page == self.page else { return }
        ready = true
        onReady?()
    }

    func collectionView(_ collectionView: UICollectionView, layout: UICollectionViewLayout,
                        sizeForItemAt indexPath: IndexPath) -> CGSize {
        let size = sizes[indexPath.item]
        let width = collectionView.bounds.width
        return CGSize(width: width, height: max(1, (width * size.height / max(size.width, 1)).rounded()))
    }

    func collectionView(_ collectionView: UICollectionView, prefetchItemsAt indexPaths: [IndexPath]) {
        // At the unzoomed width: the cells decode sharper ones once shown.
        for indexPath in indexPaths {
            cache.prefetch(page: indexPath.item, maxPixel: pixels(indexPath.item, zoom: 1), need: .soon)
        }
    }

    func scrollViewDidScroll(_ scrollView: UIScrollView) {
        movedAt = ProcessInfo.processInfo.systemUptime
        guard scrollView === collection, !positioning else { return }
        updatePlace()
    }

    /// Finds the page and screen showing, and tells of a change.
    private func updatePlace() {
        guard pending == nil, laidOutSize.width > 0 else { return }
        let bounds = collection.bounds
        let atEnd = bounds.maxY >= collection.contentSize.height - 1
        let probe = CGPoint(x: bounds.midX, y: bounds.minY + bounds.height * 0.25)
        let item = atEnd ? sizes.count - 1 : collection.indexPathForItem(at: probe)?.item ?? page
        let screen = currentScreen
        guard item != page || screen != self.screen else { return }
        page = item
        self.screen = screen
        onMove?()
    }

    private final class StripCell: UICollectionViewCell {
        let imageView = UIImageView()
        var page = -1
        /// The page's decode, cancelled when the cell is reused for
        /// another, so a fling doesn't queue every page it passed.
        var load: Task<Void, Never>?

        override init(frame: CGRect) {
            super.init(frame: frame)
            imageView.contentMode = .scaleAspectFit
            imageView.frame = contentView.bounds
            imageView.autoresizingMask = [.flexibleWidth, .flexibleHeight]
            contentView.addSubview(imageView)
            isAccessibilityElement = true
            accessibilityTraits = .image
        }

        required init?(coder: NSCoder) { nil }

        override func prepareForReuse() {
            super.prepareForReuse()
            load?.cancel()
            load = nil
        }
    }
}
