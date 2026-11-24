import GRDB
import Observation
import UIKit

/// One open Page-based File, a CBZ or a PDF, in the image pager (ADR 0025):
/// the pages, where the reader is, and what's recorded as they read. Places
/// are 0-based pages, as Shisho stores them.
@MainActor
@Observable
final class PageReaderModel: ImageSourceDelegate, Identifiable {
    let file: ReadingFile
    nonisolated var id: Int { file.fileID }
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
    private(set) var settings = PageReaderSettings.load()
    /// The places jumps left from, as pages.
    private(set) var history = LocationHistory()
    private(set) var source: ImageSource?
    /// The flow is the one the user chose for this Book, rather than the
    /// one the pages' shapes picked.
    var flowChosen: Bool { settings.flow(forBook: file.book.id) != nil }
    var flow: PageFlow { source?.flow ?? .paginated }
    /// The File runs right to left, followed or not.
    var fileRightToLeft: Bool { file.rightToLeft }
    var rightToLeft: Bool { source?.rightToLeft ?? false }

    @ObservationIgnored private let session: AccountSession
    @ObservationIgnored private let tracker: ReadingTracker
    @ObservationIgnored private var knownServerAt: Date?
    @ObservationIgnored private var dismissedOffer: ServerPlace?
    @ObservationIgnored private var placeWatch: Task<Void, Never>?
    @ObservationIgnored private var fileWatch: Task<Void, Never>?
    @ObservationIgnored private var detectedFlow = PageFlow.paginated
    @ObservationIgnored private let onGone: @MainActor () -> Void
    @ObservationIgnored private var closed = false
    @ObservationIgnored private var shownChapters: [ReaderChapter]
    /// When the last run of VoiceOver adjustments to the scrubber was.
    @ObservationIgnored private var lastAdjustment: Date?

    init(file: ReadingFile, session: AccountSession, requestSync: @escaping @MainActor () -> Void,
         onGone: @escaping @MainActor () -> Void) {
        self.file = file
        self.session = session
        self.onGone = onGone
        shownChapters = file.chapters
        tracker = ReadingTracker(fileID: file.fileID, store: session.store, requestSync: requestSync)
    }

    // MARK: - Where the reader is

    var fraction: Double { PageLayout.fraction(of: page, pageCount: pageCount) }

    /// The chapter the reader is in: the last starting at or before the
    /// page.
    var currentEntry: ContentsEntry? {
        contents.last { entry in
            if case .page(let start) = entry.target { return start <= page }
            return false
        }
    }

    var chapterTitle: String? { currentEntry?.title }

    /// The chapter page is in, as the scrubber names where it goes.
    func chapterTitle(atPage page: Int) -> String? {
        contents.last { entry in
            if case .page(let start) = entry.target { return start <= page }
            return false
        }?.title
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

    /// Fetches the File if it isn't on the device, then opens it where the
    /// reader left off, here or on another device.
    func load() async {
        phase = .loading
        placeWatch?.cancel()
        fileWatch?.cancel()
        source = nil
        let url: URL
        do {
            url = try await session.ebooks.file(file)
        } catch {
            guard !closed else { return }
            log("couldn't fetch the File: \(Diagnostics.describe(error))")
            phase = .failed(String(localized: "This book couldn't be downloaded. Check your connection to the server and try again."))
            return
        }
        let opened: (pages: PageImageSource, sizes: [CGSize])
        do {
            opened = try await Self.open(url: url, file: file)
        } catch {
            guard !closed else { return }
            log("couldn't open the File: \(Diagnostics.describe(error))")
            // A copy that won't open is fetched again on Try Again.
            session.ebooks.discard(file)
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
        let ownDevice = session.account.deviceID
        let choice = try? await session.store.pool.read { db in
            (try CatalogStore.ebookOpening(db, fileID: fileID, ownDeviceID: ownDevice),
             try CatalogStore.serverPlace(db, fileID: fileID)?.occurredAt)
        }
        guard !closed else { return }
        knownServerAt = choice?.1
        offer = choice?.0.offer
        let start = startPage(choice?.0)
        page = start
        pagesShown = [start]
        contents = entries(shownChapters)
        let source = ImageSource(pages: opened.pages, sizes: opened.sizes, flow: settings.flow(forBook: file.book.id) ?? detectedFlow,
                                 rightToLeft: settings.rightToLeft(file), startPage: start)
        source.delegate = self
        self.source = source
        logMemory("opened")
        watch()
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
        chapters.enumerated().map { index, chapter in
            let start = chapter.startPage.map { min(max(0, $0), max(0, pageCount - 1)) }
            return ContentsEntry(id: index, title: chapter.title, depth: chapter.depth, target: start.map(ReaderTarget.page),
                                 position: start.map { PageLayout.fraction(of: $0, pageCount: pageCount) })
        }
    }

    // MARK: - Moving

    func turn(_ direction: TurnDirection) {
        source?.pager.turn(direction)
    }

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

    /// Picks the flow for this Book, kept for its next open.
    func choose(_ flow: PageFlow) {
        settings.choose(flow, forBook: file.book.id)
        settings.save()
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

    func close() {
        noteReaderPlace()
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
                     atEnd: page == pageCount - 1, screen: String(page))
    }

    func imageSourceShowed(page: Int) {
        self.page = page
        pagesShown = source?.pagesShown ?? [page]
        guard phase != .reading else { return }
        phase = .reading
        tracker.opened(place(page))
    }

    func imageSourceMoved(page: Int) {
        self.page = page
        pagesShown = source?.pagesShown ?? [page]
        guard phase == .reading else { return }
        tracker.moved(place(page))
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

/// The app's own memory, as iOS counts it against the limit past which it
/// ends the app.
enum MemoryFootprint {
    static var bytes: Int {
        var info = task_vm_info_data_t()
        var count = mach_msg_type_number_t(MemoryLayout<task_vm_info_data_t>.size / MemoryLayout<natural_t>.size)
        let result = withUnsafeMutablePointer(to: &info) {
            $0.withMemoryRebound(to: integer_t.self, capacity: Int(count)) {
                task_info(mach_task_self_, task_flavor_t(TASK_VM_INFO), $0, &count)
            }
        }
        return result == KERN_SUCCESS ? Int(info.phys_footprint) : -1
    }

    static var megabytes: Int {
        let bytes = bytes
        return bytes < 0 ? -1 : bytes / 1_048_576
    }
}
