import GRDB
import ShishoAPI
import Observation
import UIKit

/// One open Page-based File, a CBZ or a PDF, in the image pager (ADR 0025):
/// the pages, where the reader is, and what's recorded as they read. Places
/// are 0-based pages, as Shisho stores them.
@MainActor
@Observable
final class PageReaderModel: ImageSourceDelegate, Identifiable {
    /// Read again from the catalog on each load, so Try Again after a
    /// sync opens the File as it is now, its page list included.
    private(set) var file: ReadingFile
    /// The File's id, which a load reading it again never changes.
    nonisolated let id: Int
    private(set) var phase = ReaderModel.Phase.loading
    var chromeVisible = true
    /// The page the reader is on, 0-based.
    private(set) var page = 0
    private(set) var pageCount = 0
    /// The pages on screen, in reading order: two in a spread.
    private(set) var pagesShown: [Int] = []
    /// Shisho's chapters, each going to the page it starts on.
    private(set) var contents: [ContentsEntry] = []
    /// A page turn has waited a moment for its page to decode.
    private(set) var turnWaiting = false
    @ObservationIgnored private var turnWait: Task<Void, Never>?
    /// Another device's place, offered without jumping.
    private(set) var offer: ServerPlace?
    private(set) var settings: PageReaderSettings
    /// The places jumps left from, as pages.
    private(set) var history = LocationHistory()
    private(set) var source: ImageSource?
    /// The flow is the one the user chose for this Book, rather than the
    /// one the pages' shapes picked.
    var flowChosen: Bool { chosenFlow != nil }
    /// The flow the user chose for the Book, here or on the web, which
    /// otherwise follows the pages' shapes.
    private(set) var chosenFlow: PageFlow?
    var flow: PageFlow { source?.flow ?? .paginated }
    /// The pages turn right to left, the File's direction followed: kept
    /// here, where the scrubber sees it change.
    var rightToLeft: Bool { settings.rightToLeft(file) }

    @ObservationIgnored private let session: AccountSession
    @ObservationIgnored private let tracker: ReadingTracker
    @ObservationIgnored private var knownServerAt: Date?
    @ObservationIgnored private var dismissedOffer: ServerPlace?
    @ObservationIgnored private var placeWatch: Task<Void, Never>?
    @ObservationIgnored private var fileWatch: Task<Void, Never>?
    @ObservationIgnored private var detectedFlow = PageFlow.paginated
    @ObservationIgnored private let onGone: @MainActor () -> Void
    @ObservationIgnored private let requestSync: @MainActor () -> Void
    @ObservationIgnored private var closed = false
    /// The File couldn't open for want of a connection: it opens once the
    /// connection is back.
    @ObservationIgnored private var waitingForConnection = false
    /// The last page showed, which finishes the Read-through.
    @ObservationIgnored private var readToEnd = false
    @ObservationIgnored private var shownChapters: [ReaderChapter]
    /// When the last run of VoiceOver adjustments to the scrubber was.
    @ObservationIgnored private var lastAdjustment: Date?

    /// The page a tile of the File's page grid opened, rather than the
    /// place.
    @ObservationIgnored private let askedPage: Int?

    init(file: ReadingFile, session: AccountSession, page: Int? = nil, requestSync: @escaping @MainActor () -> Void,
         onGone: @escaping @MainActor () -> Void) {
        self.file = file
        id = file.fileID
        self.session = session
        askedPage = page
        self.onGone = onGone
        shownChapters = file.chapters
        settings = PageReaderSettings.load()
        self.requestSync = requestSync
        tracker = ReadingTracker(fileID: file.fileID, store: session.store, requestSync: requestSync)
    }

    // MARK: - Where the reader is

    var fraction: Double { PageLayout.fraction(of: page, pageCount: pageCount) }

    /// The chapter the reader is in: the last starting at or before the
    /// page.
    var currentEntry: ContentsEntry? { PageLayout.entry(atPage: page, in: contents) }

    var chapterTitle: String? { currentEntry?.title }

    /// Pages left in the chapter, or the book, as the ebook reader says
    /// it. Without chapters, the book's count stands in.
    func pagesLeftLabel(inBook: Bool) -> AttributedString? {
        guard pageCount > 0 else { return nil }
        let left = PageLayout.pagesLeft(shown: pagesShown, at: page, pageCount: pageCount, contents: contents)
        let count = inBook ? left.book : left.chapter ?? left.book
        let ofBook = inBook || left.chapter == nil
        // The footer asks on every move of a scroll, and the label only
        // changes with the count; the bundle named saves looking it up.
        if let last = lastPagesLeft, last.count == count, last.ofBook == ofBook { return last.label }
        let label: AttributedString = switch (ofBook, count) {
        case (true, 0): AttributedString(localized: "Last page in book", bundle: .main)
        case (true, _): AttributedString(localized: "^[\(count) page](inflect: true) left in book", bundle: .main)
        case (false, 0): AttributedString(localized: "Last page in chapter", bundle: .main)
        case (false, _): AttributedString(localized: "^[\(count) page](inflect: true) left in chapter", bundle: .main)
        }
        lastPagesLeft = (count, ofBook, label)
        return label
    }

    @ObservationIgnored private var lastPagesLeft: (count: Int, ofBook: Bool, label: AttributedString)?

    /// The chapter page is in, as the scrubber names where it goes.
    func chapterTitle(atPage page: Int) -> String? {
        PageLayout.entry(atPage: page, in: contents)?.title
    }

    /// Where the chapters start, as fractions, for the scrubber's marks.
    var marks: [Double] { contents.compactMap(\.position) }

    /// "Page 3 of 20", or "Pages 4–5 of 20" for a spread.
    func pagesLabel(_ pages: [Int]? = nil) -> String {
        let pages = pages ?? pagesShown
        if pages.count > 1, let first = pages.first, let last = pages.last {
            return String(localized: "Pages \(first + 1)–\(last + 1) of \(pageCount)")
        }
        return String(localized: "Page \((pages.first ?? page) + 1) of \(pageCount)")
    }

    var percentShown: String {
        fraction.formatted(.percent.precision(.fractionLength(0)))
    }

    /// Where the reader is, as the bar and readout tell VoiceOver.
    var placeLabel: String {
        [chapterTitle, pagesLabel(), percentShown].compactMap { $0 }.joined(separator: " · ")
    }

    // MARK: - Opening

    /// Downloads the File if it isn't on the device, then opens it where
    /// the reader left off, here or on another device.
    func load() async {
        phase = .loading
        placeWatch?.cancel()
        fileWatch?.cancel()
        source = nil
        waitingForConnection = false
        let opening = id
        if let current = try? await session.store.pool.read({ try CatalogStore.readingFile($0, fileID: opening) }) {
            file = current
        }
        guard !closed else { return }
        let url: URL
        // The copy this open downloaded, which can go if it won't open, to
        // be downloaded again on Try Again.
        var fetchedCopy: UUID?
        do {
            if let downloaded = session.downloads.localFile(fileID: file.fileID) {
                url = downloaded
            } else if !session.client.reachability.allowsRequests {
                // Not on the device and no connection: say so at once, and
                // open by itself once the connection is back.
                waitForConnection()
                return
            } else {
                // The pages are drawn from the whole File, so opening one
                // downloads it, as a Download like any other.
                let fetch = try await session.downloads.fetchNow(fileID: file.fileID)
                url = fetch.url
                // One a background transfer finished meanwhile is a Download
                // like any other, kept if it won't open.
                fetchedCopy = fetch.fetched
            }
        } catch {
            guard !closed else { return }
            log("couldn't fetch the File: \(Diagnostics.describe(error))")
            if !session.client.reachability.allowsRequests {
                waitForConnection()
            } else {
                phase = .failed(String(localized: "This book couldn't be downloaded. Check your connection to the server and try again."))
            }
            return
        }
        let opened: (pages: PageImageSource, sizes: [CGSize])
        do {
            opened = try await Self.open(url: url, file: file)
        } catch {
            guard !closed else { return }
            log("couldn't open the File: \(Diagnostics.describe(error))")
            // Only the copy this open downloaded: a newer one a transfer put
            // in its place meanwhile stays.
            if let fetchedCopy, Self.removesCopy(after: error) {
                session.downloads.discard(fileID: file.fileID, copy: fetchedCopy)
            }
            phase = .failed(String(localized: "This book couldn't be opened."))
            return
        }
        guard !closed else { return }
        guard !opened.sizes.isEmpty else {
            phase = .failed(String(localized: "This book has no pages."))
            return
        }
        pageCount = opened.sizes.count
        detectedFlow = PageLayout.detectedFlow(sizes: opened.sizes)
        let fileID = file.fileID
        let bookID = file.book.id
        let ownDevice = session.account.deviceID
        let choice = try? await session.store.pool.read { db in
            (try CatalogStore.ebookOpening(db, fileID: fileID, ownDeviceID: ownDevice),
             try CatalogStore.serverPlace(db, fileID: fileID)?.occurredAt,
             try CatalogStore.pageFlow(db, bookID: bookID))
        }
        guard !closed else { return }
        chosenFlow = choice?.2
        knownServerAt = choice?.1
        offer = choice?.0.offer
        let start = askedPage.map { min(max(0, $0), pageCount - 1) } ?? startPage(choice?.0)
        page = start
        pagesShown = [start]
        contents = entries(shownChapters)
        let source = ImageSource(pages: opened.pages, sizes: opened.sizes, flow: chosenFlow ?? detectedFlow,
                                 rightToLeft: settings.rightToLeft(file), startPage: start)
        source.delegate = self
        self.source = source
        logMemory("opened")
        watch()
    }

    /// Not on the device and no connection: the reader says so, and opens
    /// once the connection is back (connectionChanged).
    private func waitForConnection() {
        waitingForConnection = true
        phase = .failed(ConnectionRules.book(session.client.reachability.connection))
    }

    /// Whether a copy this open downloaded that won't open is removed, so
    /// Try Again downloads it again: only when downloading it again could
    /// help, unlike a CBZ whose pages the server couldn't read or a PDF with
    /// a password. One downloaded before stays whatever the error: failing
    /// under memory pressure, say on a plane, mustn't cost the only copy.
    nonisolated static func removesCopy(after error: Error) -> Bool {
        error as? PDFPages.Failure != .locked
            && ![CBZPages.Failure.noPages, .notThisCopy].contains(error as? CBZPages.Failure)
    }

    /// The pages and their shapes, read off the main thread.
    nonisolated private static func open(url: URL, file: ReadingFile) async throws -> (pages: PageImageSource, sizes: [CGSize]) {
        try await Task.detached(priority: .userInitiated) {
            let pages: PageImageSource = file.fileType == "pdf"
                ? try PDFPages(url: url)
                : try CBZPages(url: url, pageNames: file.pages)
            return (pages, await pages.pageSizes())
        }.value
    }

    /// The page a place opens at: its page, or the page its percentage
    /// lands on when it has none, as from a reader that saves only that.
    private func startPage(_ choice: PlaceRules.EbookChoice?) -> Int {
        switch choice?.start {
        case .page(let page): min(max(0, page), pageCount - 1)
        case .fraction(let fraction): PageLayout.page(at: fraction, pageCount: pageCount)
        case .cfi, nil: PageLayout.page(at: choice?.percentage ?? 0, pageCount: pageCount)
        }
    }

    private func entries(_ chapters: [ReaderChapter]) -> [ContentsEntry] {
        PageLayout.contents(chapters, pageCount: pageCount)
    }

    // MARK: - Moving

    func go(to entry: ContentsEntry) {
        guard case .page(let page) = entry.target else { return }
        lastAdjustment = nil
        jump(to: page)
    }

    /// Goes to page from the scrubber. A run of VoiceOver's adjustments, each
    /// a few seconds from the last, is one jump, so going back returns to
    /// where the run started.
    func go(toPage page: Int, adjusting: Bool = false) {
        let now = Date()
        let startsRun = !adjusting || lastAdjustment.map { now.timeIntervalSince($0) > 3 } ?? true
        lastAdjustment = adjusting ? now : nil
        jump(to: page, recording: startsRun)
    }

    /// Goes to target, noting where it left from unless it's part of a run
    /// of adjustments already noted.
    private func jump(to target: Int, recording: Bool = true) {
        guard target != page else { return }
        if recording { history.recordJump(from: String(page)) }
        source?.jump(to: target)
    }

    func goBack() {
        guard let target = history.back(from: String(page)).flatMap(Int.init) else { return }
        lastAdjustment = nil
        source?.jump(to: target)
    }

    func goForward() {
        guard let target = history.forward(from: String(page)).flatMap(Int.init) else { return }
        lastAdjustment = nil
        source?.jump(to: target)
    }

    func acceptOffer() {
        guard let offer else { return }
        self.offer = nil
        lastAdjustment = nil
        try? session.store.resolveOffer(fileID: file.fileID)
        jump(to: offer.page ?? PageLayout.page(at: offer.percentage, pageCount: pageCount))
    }

    func dismissOffer() {
        dismissedOffer = offer
        offer = nil
        // Staying keeps this page for the next open, even with no turn of
        // this device's to send.
        try? session.store.stayHere(fileID: file.fileID, cfi: nil, page: page, percentage: fraction, at: Date())
    }

    // MARK: - Settings

    /// Picks the flow for this Book, kept with it on the server, so it's
    /// the flow on the web and every device too.
    func choose(_ flow: PageFlow) {
        chosenFlow = flow
        do {
            try session.store.choosePageFlow(flow, bookID: file.book.id, at: Date())
            requestSync()
        } catch {
            log("couldn't save the flow: \(Diagnostics.describe(error))")
        }
        source?.apply(flow: flow, rightToLeft: settings.rightToLeft(file))
    }

    var followReadingDirection: Bool {
        get { settings.followReadingDirection }
        set {
            settings.followReadingDirection = newValue
            settings.save()
            source?.apply(flow: flow, rightToLeft: settings.rightToLeft(file))
        }
    }

    // MARK: - The app's life

    func background() {
        noteReaderPlace()
        tracker.background()
    }

    func foreground() {
        tracker.foreground()
    }

    func memoryWarning() {
        source?.memoryWarning()
        logMemory("memory warning")
    }

    /// The connection came back: a File that couldn't open for want of it
    /// opens now.
    func connectionChanged(_ connection: Connection) {
        guard connection == .online, waitingForConnection, !closed else { return }
        waitingForConnection = false
        Task { await load() }
    }

    func close() {
        noteReaderPlace()
        if !closed {
            session.downloads.released(fileID: file.fileID)
        }
        closed = true
        turnWait?.cancel()
        tracker.close()
        placeWatch?.cancel()
        fileWatch?.cancel()
        logMemory("closed")
    }

    /// Keeps the chapter and pages left for the Book's page.
    private func noteReaderPlace() {
        guard phase == .reading else { return }
        try? session.store.noteReaderPlace(fileID: file.fileID, chapter: chapterTitle, pagesLeft: pageCount - 1 - page,
                                           percentage: fraction)
    }

    // MARK: - Following the catalog

    private func watch() {
        let pool = session.store.pool
        let fileID = file.fileID
        placeWatch = Task { [weak self] in
            let places = ValueObservation.tracking { db in try CatalogStore.serverPlace(db, fileID: fileID) }.removeDuplicates()
            do {
                for try await place in places.values(in: pool) {
                    self?.serverPlaceChanged(place)
                }
            } catch {}
        }
        // A File removed from the user's Libraries closes the reader.
        fileWatch = Task { [weak self] in
            let files = ValueObservation.tracking { db in try CatalogStore.readingFile(db, fileID: fileID) }.removeDuplicates()
            do {
                for try await updated in files.values(in: pool) {
                    guard let self else { return }
                    guard let updated else {
                        self.onGone()
                        return
                    }
                    if updated.chapters != self.shownChapters {
                        self.shownChapters = updated.chapters
                        self.contents = self.entries(updated.chapters)
                    }
                }
            } catch {}
        }
    }

    private func serverPlaceChanged(_ place: ServerPlace?) {
        guard let place, !place.isSameMove(as: dismissedOffer) else { return }
        if place.isSameMove(as: offer) {
            if place != offer { offer = place }
            return
        }
        if let offered = PlaceRules.offerWhileReading(server: place, at: (cfi: nil, percentage: fraction), page: page,
                                                      known: knownServerAt, ownDeviceID: session.account.deviceID) {
            offer = offered
        }
    }

    // MARK: - ImageSourceDelegate

    private func place(_ page: Int) -> ReadingPlace {
        ReadingPlace(locator: .page(page), fraction: PageLayout.fraction(of: page, pageCount: pageCount),
                     atEnd: page == pageCount - 1, screen: source?.screen ?? String(page))
    }

    func imageSourceShowed(page: Int) {
        self.page = page
        pagesShown = source?.pagesShown ?? [page]
        guard phase != .reading else { return }
        phase = .reading
        // Where it opened isn't reading, whether at the place or at a page
        // tapped in the File's grid, so a look at a page changes nothing.
        // Reading on from there is: the first turn moves the place, and
        // starts a Read-through as any reading past the first page does, so
        // a reader can skip the pages before the story.
        tracker.opened(place(page))
    }

    func imageSourceMoved(page: Int) {
        self.page = page
        pagesShown = source?.pagesShown ?? [page]
        guard phase == .reading else { return }
        let place = place(page)
        tracker.moved(place)
        if place.atEnd, !readToEnd {
            readToEnd = true
            // The pages are read from the File as they show, so it goes
            // once the reader closes, or at the next launch.
            session.downloads.finished(fileID: file.fileID, holding: true)
        } else if !place.atEnd, readToEnd {
            // Back from the end, which undoes an automatic finish.
            readToEnd = false
            session.downloads.unfinished(fileID: file.fileID)
        }
    }

    func imageSourceLaidOut() {
        page = source?.currentPage ?? page
        pagesShown = source?.pagesShown ?? [page]
    }

    func imageSourceWaiting(_ waiting: Bool) {
        turnWait?.cancel()
        guard waiting else {
            turnWaiting = false
            return
        }
        turnWait = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }
            self?.turnWaiting = true
        }
    }

    func imageSourceSawInput() {
        tracker.input()
    }

    func imageSourceTurning() {
        guard !UIAccessibility.isVoiceOverRunning else { return }
        chromeVisible = false
    }

    func imageSourceToggleChrome() {
        chromeVisible.toggle()
    }

    // MARK: - Diagnostics

    /// The app's memory as a File opens and closes and on memory warnings,
    /// since a large scan is where memory runs out.
    private func logMemory(_ event: String) {
        log("\(event), \(file.fileType), \(file.sizeBytes / 1024) KB; app footprint \(MemoryFootprint.megabytes) MB")
    }

    private func log(_ message: String) {
        let fileID = file.fileID
        Task { await Diagnostics.shared.log(.reader, "file \(fileID): \(message)") }
    }
}
