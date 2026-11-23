import UIKit

/// Where a jump goes in a Reflowable File.
enum ReaderTarget: Equatable, Sendable {
    case cfi(String)
    /// A link inside the book, as foliate resolves it.
    case href(String)
    /// How far through the book, from 0 to 1.
    case fraction(Double)

    /// The target as the page's script takes it.
    var script: String {
        switch self {
        case .cfi(let value), .href(let value): javaScriptLiteral(value)
        case .fraction(let value): "{\"fraction\":\(min(1, max(0, value.isFinite ? value : 0)))}"
        }
    }
}

/// What the reader screen hears from the pages.
@MainActor
protocol FoliateSourceDelegate: AnyObject {
    /// The book is open, with the file's own table of contents.
    func foliateOpened(toc: [PageMessage.TocItem])
    /// Where the current page first showed, which isn't reading.
    func foliateShowed(_ location: PageLocation)
    /// The reader moved: a turn, a scroll, a jump.
    func foliateMoved(_ location: PageLocation)
    /// The page showing laid out again without moving, as once its images
    /// load or the settings change: its page counts may differ.
    func foliateLaidOut(_ location: PageLocation)
    /// Where each chapter given to `contents` is in the book.
    func foliateContents(_ entries: [PageMessage.ContentsEntry])
    /// A footnote, shown beside its reference (in the pager's points).
    func foliateNote(paragraphs: [String], at rect: CGRect, href: String, font: NoteFont, lineHeight: Double?)
    func foliateExternalLink(_ url: URL)
    /// A page turn is waiting for its page to be laid out.
    func foliateWaiting(_ waiting: Bool)
    /// The book couldn't be shown, or its pages kept failing.
    func foliateFailed()
    func foliateSawInput()
    /// A page turn is starting, which hides the controls.
    func foliateTurning()
    func foliateToggleChrome()
    /// Scrolled, a pull past a section's end or start: which way, how far
    /// toward bringing the section beside in, from 0 to 1 (1 only once
    /// letting go would), and where that section starts along the book; nil
    /// when it stopped.
    func foliatePull(_ direction: TurnDirection?, progress: Double, destination: Double?)
    /// An image to show on its own, as a data URL.
    func foliateImage(_ dataURL: String)
    /// Matches a search found in a section, under its token.
    func foliateSearchResults(_ results: [PageMessage.SearchResult], token: Int)
    /// A search read the whole book, or stopped.
    func foliateSearchDone(token: Int)
    /// The page running a search ended before it finished.
    func foliateSearchInterrupted(token: Int)
    /// The last jump landed, or failed.
    func foliateJumpLanded()
    /// A link inside the book was tapped: the reader jumps there.
    func foliateFollowLink(_ href: String)
}

/// foliate as the pager's page source for Reflowable Files (ADR 0026).
///
/// Three WebViews hold the current page and the pages on either side. A
/// neighbor is moved one page from the current one by section and page,
/// not by CFI (a blank or image-only page's CFI resolves to another page),
/// and counts as ready only once its fonts, the images on its page, and two
/// frames have painted, and only while it's one page from where the current
/// page really is.
@MainActor
final class FoliateSource: PagerSource {
    private struct PageState {
        /// A sync, jump, or open is running; its answer decides ready.
        var inFlight = false
        var token = 0
        /// The current page's [index, page, pages] this neighbor is one
        /// page from.
        var base: [Int]?
        var edge = false
        /// The current page's place this neighbor's syncs failed from, and
        /// how many times in a row.
        var failedBase: [Int]?
        var failures = 0
        /// The current page's place a sync in flight was sent from; nil for
        /// an open or a jump.
        var syncedFrom: [Int]?
    }

    weak var delegate: FoliateSourceDelegate?
    let pager: PagerController
    private let handler: ReaderSchemeHandler
    private var back: ReaderPage
    private var current: ReaderPage
    private var forward: ReaderPage
    private var states: [ObjectIdentifier: PageState] = [:]
    private var tokens = 0
    private(set) var settings: ReaderSettings
    private var opened = false
    /// The current page is going somewhere: its neighbors wait for it.
    private var navigating = false
    /// The last jump sent: jumps land in turn, and the reader is still
    /// jumping until this one has.
    private var jumpToken = 0
    /// Whether a jump hasn't landed yet.
    var isJumping: Bool { navigating }
    private var navigationTimeout: Task<Void, Never>?
    /// The place to reopen at if a page's process ends.
    private var lastCFI: String?
    /// The table of contents to place: Shisho's chapters, whose links are
    /// as the book's navigation document wrote them, or the file's own.
    private var chapters: [ReaderChapter] = []
    private var chaptersFromShisho = true
    /// Names the latest request for the chapters' places, whose answer can
    /// come from a page that has since become a neighbor.
    private var contentsToken = 0
    /// Where the book first opened, and how far through that is.
    private var openStart: ReaderTarget?
    private var fallback: Double?
    /// When pages' content processes ended lately, to stop reopening a book
    /// that keeps failing.
    private var restarts: [Date] = []
    /// The section the book first opened in.
    private var openedSection: Int?
    /// Gives a page the chapters' places, sent again to one that restarts.
    private var chapterTargetsScript: String?
    /// The neighbors waiting to open, with the tokens their open answers.
    private var unopenedNeighbors: [(ReaderPage, TurnDirection)] = []
    /// The search under way, by token, and the page running it: the page
    /// that was current when it started, whichever it is now.
    private var searchTokens = 0
    private var runningSearch: (token: Int, page: ReaderPage)?
    /// Marks the chosen search result, sent again to a page that restarts.
    private var markScript: String?
    /// The search's matches so far, which every page shades, sent again to a
    /// page that restarts.
    private var matchCFIs: [String] = []

    /// How the page opens the book a part at a time: as a zip (EPUB,
    /// KePub) or as MOBI records (MOBI, AZW3), and how big it is.
    private let bookFormat: String
    private let language: String?
    private let bookSize: Int

    /// pages are booted pages from ReaderPagePool.
    /// language is the File's, as Shisho has it, for the page's lang and
    /// direction.
    init(bookFile: URL, fileType: String, settings: ReaderSettings, pages: ReaderPagePool.Pages, language: String? = nil) {
        self.language = language
        handler = pages.handler
        handler.bookFile = bookFile
        bookFormat = ["mobi", "azw3"].contains(fileType) ? "mobi" : "zip"
        bookSize = (try? bookFile.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? 0
        self.settings = settings
        let background = settings.theme.background
        back = pages.back
        current = pages.current
        forward = pages.forward
        pager = PagerController()
        pager.backdropColor = background
        pager.paging = settings.layout == .paginated
        for page in [back, current, forward] {
            page.setBackground(background)
            states[ObjectIdentifier(page)] = PageState()
            page.onMessage = { [weak self, weak page] message in
                guard let self, let page else { return }
                self.handle(message, from: page)
            }
            page.onTerminate = { [weak self, weak page] in
                guard let self, let page else { return }
                self.restart(page)
            }
        }
        pager.attach(self)
    }

    /// A fixed-layout book, which is paged whatever the layout setting, as
    /// foliate lays it out.
    private var fixedLayout = false
    private var paginated: Bool { settings.layout == .paginated || fixedLayout }

    // MARK: - Opening

    /// Opens the book at start. The neighbors open once the current page
    /// shows, each then stepping one page itself: opening all three at once
    /// on a phone's few cores slows the first page. chapters is the table of
    /// contents from Shisho, or nil to use the file's own.
    func open(at start: ReaderTarget?, fallback: Double?, chapters: [ReaderChapter]?) {
        self.chapters = chapters ?? []
        chaptersFromShisho = chapters != nil
        openStart = start
        self.fallback = fallback
        if case .cfi(let cfi) = start { lastCFI = cfi }
        current.call("R.open(\(openArguments(start: start, neighbor: 0, token: 0)))")
        // Waiting for their open, so nothing syncs them before it.
        unopenedNeighbors = [(forward, .forward), (back, .back)]
        for page in [forward, back] { _ = startFlight(page) }
    }

    /// The neighbors' first open, once the current page shows, from where
    /// it is and in the settings then: a token of their own, as settings
    /// applied while the page opened replace the ones they waited with.
    private func openNeighbors() {
        let start = lastCFI.map(ReaderTarget.cfi) ?? openStart
        for (page, direction) in unopenedNeighbors {
            let token = startFlight(page)
            page.call("R.open(\(openArguments(start: start, neighbor: direction.rawValue, token: token)))")
        }
        unopenedNeighbors = []
    }

    private func openArguments(start: ReaderTarget?, neighbor: Int, token: Int, idle: Bool = false) -> String {
        "{url: \(javaScriptLiteral(ReaderSchemeHandler.bookURL)), size: \(bookSize), format: \(javaScriptLiteral(bookFormat)), "
            + "start: \(start?.script ?? "null"), "
            + "fallback: \(fallback.map { String($0) } ?? "null"), settings: \(settings.pageScript), "
            + "neighborDir: \(neighbor), token: \(token), idle: \(idle), "
            + "language: \(language.map(javaScriptLiteral) ?? "null")}"
    }

    /// WebKit ended a page's content process, as under memory pressure: it
    /// opens again where the reader was.
    private func restart(_ page: ReaderPage) {
        let now = Date()
        restarts = restarts.filter { now.timeIntervalSince($0) < 120 } + [now]
        guard restarts.count <= 3 else {
            Task { await Diagnostics.shared.log(.reader, "pages' web content processes keep ending; giving up on the book") }
            delegate?.foliateFailed()
            return
        }
        Task { await Diagnostics.shared.log(.reader, "a page's web content process ended; reopening it") }
        page.reload()
        if let chapterTargetsScript { page.call(chapterTargetsScript) }
        if !matchCFIs.isEmpty { page.call(Self.addMatchesScript(matchCFIs)) }
        if let markScript { page.call(markScript) }
        // A search the page was running ended with it, part way.
        if let running = runningSearch, running.page === page {
            runningSearch = nil
            delegate?.foliateSearchInterrupted(token: running.token)
        }
        let start = lastCFI.map(ReaderTarget.cfi) ?? openStart
        if page === current {
            // The neighbors are sent beside it again once it shows.
            opened = false
            pager.reset()
            current.call("R.open(\(openArguments(start: start, neighbor: 0, token: 0)))")
            for neighbor in [back, forward] {
                tokens += 1
                states[ObjectIdentifier(neighbor)] = PageState(token: tokens)
            }
        } else {
            let token = startFlight(page)
            pager.neighbor(direction(of: page), is: .pending)
            page.call("R.open(\(openArguments(start: start, neighbor: direction(of: page).rawValue, token: token)))")
        }
    }

    // MARK: - Moving

    /// A table of contents or link jump. The neighbors go to the same place
    /// at the same time and step from there, instead of waiting for the
    /// current page to lay out a large section.
    /// fallback, how far through the book, is where to go instead when the
    /// target isn't in this copy of the book.
    func jump(to target: ReaderTarget, fallback: Double? = nil) {
        let fallbackScript = fallback.map { String($0) } ?? "null"
        pager.reset()
        jumpToken += 1
        current.call("R.goTo(\(target.script), \(fallbackScript), \(jumpToken))")
        navigating = true
        // Scrolled, the neighbors follow once the page lands: they're the
        // sections beside it, not the pages.
        if paginated {
            for page in [forward, back] {
                let token = startFlight(page)
                page.call("R.jump(\(target.script), \(direction(of: page).rawValue), \(token), \(fallbackScript))")
            }
        }
        // In case the page never says it landed, as when its process ends.
        navigationTimeout?.cancel()
        navigationTimeout = Task { [weak self] in
            try? await Task.sleep(for: .seconds(4))
            guard let self, !Task.isCancelled, self.navigating else { return }
            self.navigating = false
            self.delegate?.foliateJumpLanded()
            self.reconcile()
        }
    }

    func apply(_ settings: ReaderSettings) {
        let layoutChanged = settings.layout != self.settings.layout
        self.settings = settings
        pager.backdropColor = settings.theme.background
        pager.paging = paginated
        pager.reset()
        for page in [back, current, forward] {
            page.setBackground(settings.theme.background)
            if page !== current {
                // Laid out again from the current page once it has reflowed.
                tokens += 1
                states[ObjectIdentifier(page)] = PageState(token: tokens)
            }
            // The page showing keeps its place by its text.
            page.call("R.apply(\(settings.pageScript), \(page === current))")
        }
        if layoutChanged {
            navigating = false
        }
    }

    // MARK: - Search

    /// Searches the whole book from the current page, which sends each
    /// section's matches as it reads them; returns the token they come
    /// with.
    func search(_ query: String) -> Int {
        cancelSearch()
        searchTokens += 1
        runningSearch = (searchTokens, current)
        current.call("R.search(\(javaScriptLiteral(query)), \(searchTokens))")
        return searchTokens
    }

    /// Stops any search and takes its marks off every page.
    func cancelSearch() {
        runningSearch = nil
        matchCFIs = []
        markScript = nil
        for page in [back, current, forward] { page.call("R.cancelSearch()") }
    }

    private static func addMatchesScript(_ cfis: [String]) -> String {
        let json = (try? JSONSerialization.data(withJSONObject: cfis)).map { String(decoding: $0, as: UTF8.self) } ?? "[]"
        return "R.addMatches(\(json))"
    }

    /// Marks the chosen result on every page, so it shows on whichever the
    /// reader turns to.
    func markResult(_ cfi: String?) {
        let script = "R.markResult(\(cfi.map(javaScriptLiteral) ?? "null"))"
        markScript = cfi == nil ? nil : script
        for page in [back, current, forward] { page.call(script) }
    }

    /// Shisho's chapters changed, as when they were edited on the server.
    func updateChapters(_ chapters: [ReaderChapter]) {
        guard chaptersFromShisho else { return }
        self.chapters = chapters
        if opened { requestContents() }
    }

    /// Asks where each chapter is, for the table of contents and the
    /// scrubber's marks.
    func requestContents() {
        let list = chapters.map { ["title": $0.title, "href": $0.href ?? ""] }
        let json = (try? JSONSerialization.data(withJSONObject: list)).map { String(decoding: $0, as: UTF8.self) } ?? "[]"
        contentsToken += 1
        current.call("R.contents(\(json), \(chaptersFromShisho), \(contentsToken))")
    }

    // MARK: - Messages

    private func state(_ page: ReaderPage) -> PageState {
        states[ObjectIdentifier(page)] ?? PageState()
    }

    private func update(_ page: ReaderPage, _ change: (inout PageState) -> Void) {
        var state = state(page)
        change(&state)
        states[ObjectIdentifier(page)] = state
    }

    private func direction(of page: ReaderPage) -> TurnDirection {
        page === forward ? .forward : .back
    }

    private func neighbor(_ direction: TurnDirection) -> ReaderPage {
        direction == .forward ? forward : back
    }

    private func startFlight(_ page: ReaderPage) -> Int {
        tokens += 1
        let token = tokens
        update(page) { $0 = PageState(inFlight: true, token: token, failedBase: $0.failedBase, failures: $0.failures) }
        return token
    }

    private func handle(_ message: PageMessage, from page: ReaderPage) {
        switch message.type {
        case "error":
            let text = message.message ?? "unknown error"
            Task { await Diagnostics.shared.log(.reader, "page script failed: \(text)") }
            // Before the book shows, it didn't open. After, the page is
            // still there to read, so the failure is only logged.
            if page === current, !opened { delegate?.foliateFailed() }
            return
        case "log":
            let text = message.message ?? ""
            Task { await Diagnostics.shared.log(.reader, "page: \(text)") }
            return
        case "contents":
            guard message.token == contentsToken else { return }
            let entries = message.contents ?? []
            delegate?.foliateContents(entries)
            // Every page counts pages to the next chapter from these.
            let targets = (try? JSONSerialization.data(withJSONObject: entries.compactMap(\.target)))
                .map { String(decoding: $0, as: UTF8.self) } ?? "[]"
            chapterTargetsScript = "R.setChapterTargets(\(targets))"
            for page in [back, current, forward] { page.call(chapterTargetsScript ?? "") }
            return
        case "synced":
            guard page !== current, message.token == state(page).token else { return }
            if message.failed == true {
                // Tried again from the same place once; a page that still
                // can't be laid out is treated as the end, so turns toward
                // it stop queueing.
                // Charged to the place the sync was sent from, which the
                // current page may have moved off since.
                let base = state(page).syncedFrom ?? currentBase
                let failures = state(page).failedBase == base ? state(page).failures + 1 : 1
                update(page) { $0 = PageState(failedBase: base, failures: failures) }
                if failures >= 2 {
                    update(page) { $0.base = base; $0.edge = true }
                }
                reconcile()
                return
            }
            update(page) {
                $0.inFlight = false
                $0.base = message.base
                $0.edge = message.edge ?? false
                $0.failedBase = nil
                $0.failures = 0
            }
            reconcile()
            return
        case "activity":
            if page === current { delegate?.foliateSawInput() }
            return
        case "searchResults":
            guard let token = message.token, token == runningSearch?.token else { return }
            let results = message.results ?? []
            // Every page shades them, whichever shows their section.
            let cfis = results.map(\.cfi)
            matchCFIs += cfis
            let script = Self.addMatchesScript(cfis)
            for page in [back, current, forward] { page.call(script) }
            delegate?.foliateSearchResults(results, token: token)
            return
        case "searchDone":
            guard let token = message.token, token == runningSearch?.token else { return }
            runningSearch = nil
            delegate?.foliateSearchDone(token: token)
            return
        default:
            break
        }
        guard page === current else { return }
        switch message.type {
        case "opened":
            let toc = message.toc ?? []
            if !chaptersFromShisho {
                chapters = toc.map { ReaderChapter(title: $0.label, href: $0.href.isEmpty ? nil : $0.href, depth: $0.depth) }
            }
            pager.rightToLeft = message.rtl ?? false
            if message.fixedLayout == true, !fixedLayout {
                // Paged whatever the layout setting: the neighbors, which
                // open once the page shows, open paged.
                fixedLayout = true
                pager.paging = true
            }
            delegate?.foliateOpened(toc: toc)
        case "ready":
            opened = true
            // Every page judges a first-screen scroll by where the book
            // first opened, whichever page is showing then.
            openedSection = openedSection ?? message.loc?.index
            if let section = openedSection {
                for page in [back, current, forward] {
                    page.call("R.setOpenedSection(\(section))")
                }
            }
            lastCFI = message.loc?.cfi ?? lastCFI
            delegate?.foliateShowed(current.location)
            openNeighbors()
            requestContents()
            reconcile()
        case "relocate":
            let reason = message.reason
            if message.quiet == false, reason != "anchor", reason != nil {
                // A scroll, a jump landing, or a turn inside the page while
                // selecting: the reader moved.
                lastCFI = current.location.cfi ?? lastCFI
                delegate?.foliateMoved(current.location)
            } else if opened {
                // The page laid out again in place, as once a large image
                // loads: the same place, with new page counts.
                delegate?.foliateLaidOut(current.location)
            }
            reconcile()
        case "jumped":
            // The last jump landed, or failed: an earlier one landing
            // leaves the reader jumping still.
            guard message.token == jumpToken, navigating else { return }
            navigating = false
            navigationTimeout?.cancel()
            delegate?.foliateJumpLanded()
            reconcile()
        case "applied":
            if opened { delegate?.foliateLaidOut(current.location) }
            reconcile()
        case "note":
            let box = message.rect ?? []
            let rect = box.count == 4 ? CGRect(x: box[0], y: box[1], width: box[2], height: box[3]) : .zero
            delegate?.foliateNote(paragraphs: message.paragraphs ?? [], at: rect, href: message.href ?? "",
                                  font: NoteFont.resolve(message.fontFamily ?? ""), lineHeight: message.lineHeight)
        case "link":
            if let href = message.href { delegate?.foliateFollowLink(href) }
        case "pull":
            let amount = message.amount ?? 0
            let direction: TurnDirection = (message.dir ?? 1) > 0 ? .forward : .back
            let progress = message.armed == true ? 1 : min(0.99, amount / max(1, message.threshold ?? 80))
            delegate?.foliatePull(amount > 0 ? direction : nil, progress: progress, destination: message.destination)
        case "pullRelease":
            // Not while a sideways drag has the touch, which turns instead.
            guard !pager.isDragging else { return }
            // The section beside slides in when it's laid out; when it isn't
            // yet, the page moves into it in place.
            let direction: TurnDirection = (message.dir ?? 1) > 0 ? .forward : .back
            if !pager.slide(direction) { current.call("R.pullThrough(\(direction.rawValue))") }
        case "image":
            if let data = message.message { delegate?.foliateImage(data) }
        case "external":
            if let href = message.href, let url = URL(string: href), ["http", "https", "mailto"].contains(url.scheme?.lowercased()) {
                delegate?.foliateExternalLink(url)
            }
        case "selectionEdge":
            // A selection dragged past the page's edge: the current page
            // turns in place, keeping the selection, and the neighbors
            // follow.
            if let dir = message.dir, paginated {
                pager.reset()
                current.call(dir > 0 ? "R.next()" : "R.prev()")
            }
        default:
            break
        }
    }

    /// Marks each landed neighbor ready when it's one page from where the
    /// current page really is, and sends it again when it isn't, as when the
    /// current page re-laid out after its fonts loaded.
    private func reconcile() {
        guard opened, !navigating, let base = currentBase else { return }
        for page in [forward, back] {
            let state = state(page)
            if state.inFlight { continue }
            if state.base == base {
                pager.neighbor(direction(of: page), is: state.edge ? .edge : .ready)
            } else {
                sync(page)
            }
        }
    }

    /// Sends a neighbor to the page next to the current one.
    private func sync(_ page: ReaderPage) {
        guard let base = currentBase else { return }
        let token = startFlight(page)
        update(page) { $0.base = nil; $0.syncedFrom = base }
        pager.neighbor(direction(of: page), is: .pending)
        page.call("R.sync(\(base[0]), \(base[1]), \(base[2]), \(direction(of: page).rawValue), \(token))")
    }

    // MARK: - PagerSource

    var pagerViews: (back: UIView, current: UIView, forward: UIView) {
        (back.webView, current.webView, forward.webView)
    }

    func pagerDidRotate(_ direction: TurnDirection) {
        let old = current
        let recycled: ReaderPage
        if direction == .forward {
            recycled = back
            (back, current, forward) = (old, forward, recycled)
        } else {
            recycled = forward
            (back, current, forward) = (recycled, back, old)
        }
        if old.hasSelection { old.clearSelection() }
        lastCFI = current.location.cfi ?? lastCFI
        delegate?.foliateMoved(current.location)
        current.call("R.preloadNotes()")
        if paginated {
            // The old page is one page from the new one: it's where it was.
            update(old) { $0 = PageState(base: self.currentBase) }
        } else {
            // Scrolled, it's the section beside but scrolled to wherever the
            // reader left it; a neighbor shows its section from the top.
            sync(old)
        }
        sync(recycled)
    }

    func pagerTakesTap(at point: CGPoint, resting: Bool) -> Bool {
        if current.hasSelection {
            current.clearSelection()
            return true
        }
        guard paginated else {
            // The page scrolls under the pager, so the page hit-tests.
            Task { [weak self] in
                guard let self else { return }
                let took = await self.current.evaluate("R.tapLink(\(point.x), \(point.y))") as? Bool ?? false
                if !took { self.delegate?.foliateToggleChrome() }
            }
            return true
        }
        let width = current.webView.bounds.width
        let fraction = width > 0 ? point.x / width : 0.5
        let slop = PagerZones.linkSlop(at: fraction)
        let noteSlop = PagerController.linkSlop
        // An image opens on its own from the middle of the page; the edges
        // still turn, so a page that's all photo can.
        if resting, current.link(near: point, slop: slop, noteSlop: noteSlop) == nil, slop > 0, let image = current.image(at: point) {
            current.call("R.openImage(\(image))")
            return true
        }
        guard resting, let id = current.link(near: point, slop: slop, noteSlop: noteSlop) else { return false }
        current.call("R.activateLink(\(id), \(point.x), \(point.y))")
        return true
    }

    var pagerBlocksDrag: Bool { current.hasSelection }

    func pagerWillTurn() {
        // A sideways drag or turn ends any pull past a section's end.
        if !paginated {
            current.call("R.cancelPull()")
            delegate?.foliatePull(nil, progress: 0, destination: nil)
        }
        if current.hasSelection { current.clearSelection() }
        delegate?.foliateTurning()
    }

    func pagerSawInput() {
        delegate?.foliateSawInput()
    }

    func pagerToggleChrome() {
        delegate?.foliateToggleChrome()
    }

    /// Where the neighbors are laid out from: the current page's section,
    /// page, and page count, or scrolled, its section alone, so scrolling
    /// within a section doesn't move them.
    private var currentBase: [Int]? {
        if paginated { return current.location.base }
        return current.location.index.map { [$0, 0, 0] }
    }

    func pagerWaiting(_ waiting: Bool) {
        delegate?.foliateWaiting(waiting)
    }
}
