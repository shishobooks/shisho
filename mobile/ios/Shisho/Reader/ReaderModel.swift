import GRDB
import ImageIO
import Observation
import UIKit

/// An entry in the reader's table of contents.
struct ContentsEntry: Identifiable, Equatable {
    let id: Int
    let title: String
    let depth: Int
    /// Where it goes, or nil for a heading, or a chapter this copy of the
    /// book doesn't have.
    var target: ReaderTarget?
    /// Where it starts along the book, from 0 to 1, once known.
    var position: Double?
}

/// An image from the book, shown on its own to zoom into.
struct OpenImage: Identifiable, Equatable {
    let id = UUID()
    let image: UIImage
}

/// A footnote shown in a card beside its reference.
struct OpenNote: Identifiable, Equatable {
    let id = UUID()
    let paragraphs: [String]
    /// The reference's box, in the pager's points.
    let rect: CGRect
    let href: String
    /// The book's font around the reference, or the one chosen in the
    /// settings.
    let font: NoteFont
    /// The text's size: 90% of the reader's font size.
    let size: CGFloat
    /// The book's line height around the reference, as a multiple of the
    /// text's size.
    let lineHeight: Double?
}

/// One open Reflowable File: the pages, where the reader is, and what's
/// recorded as they read.
@MainActor
@Observable
final class ReaderModel: FoliateSourceDelegate, ReaderSearchEngine, Identifiable {
    enum Phase: Equatable {
        case loading
        case reading
        case failed(String)
    }

    let file: ReadingFile
    nonisolated var id: Int { file.fileID }
    private(set) var phase = Phase.loading
    var chromeVisible = true
    private(set) var fraction: Double = 0
    /// Where the page showing starts, which names its chapter.
    private var pageStart: Double?
    /// The section showing and the page in it, for pages left.
    private var pageSection: PageLocation.SectionPlace?
    /// The page in the section the next chapter starts on, when it does.
    private var nextChapter: PageLocation.NextChapter?
    /// Each section shown, by index: its pages and share of the book, for
    /// estimating pages past the one showing.
    @ObservationIgnored private var seenSections: [Int: (pages: Int, share: Double)] = [:]
    private(set) var contents: [ContentsEntry] = [] {
        didSet { indexStarts() }
    }
    /// Where the chapters start along the book, one per point the scrubber
    /// can show: a table of contents can run to thousands of entries.
    private(set) var marks: [Double] = []
    /// The placed chapters' starts in order along the book, so the chapter
    /// showing is found by halving rather than by reading every entry.
    /// Observed, so the chapter shown follows the contents being placed
    /// after the first page shows.
    private var starts: [(position: Double, index: Int)] = []
    private(set) var note: OpenNote?
    /// An image open on its own, to zoom into.
    var image: OpenImage?
    /// Scrolled, a pull past a section's end or start under way: which way,
    /// how far toward letting go bringing the section beside in, and the
    /// chapter showing at that section's top.
    private(set) var pull: (direction: TurnDirection, progress: Double, chapter: String?)?
    /// A page turn has waited a moment for its page to be laid out, as
    /// while the book is still opening: shown, so the page doesn't look
    /// frozen.
    private(set) var turnWaiting = false
    @ObservationIgnored private var turnWait: Task<Void, Never>?
    /// Another device's place, offered without jumping.
    private(set) var offer: ServerPlace?
    var settings: ReaderSettings {
        didSet {
            guard settings != oldValue else { return }
            settings.save()
            guard settings.restyles(from: oldValue, imported: ImportedFontStore.shared.fonts) else { return }
            source?.apply(settings)
        }
    }

    /// Search inside the Book.
    let search = ReaderSearch()
    /// How far the File's download has got while it opens.
    let download = OpeningDownload()

    private(set) var source: FoliateSource?
    @ObservationIgnored private let session: AccountSession
    @ObservationIgnored private let tracker: ReadingTracker
    @ObservationIgnored private var location: PageLocation?
    /// When the server's place was from that the reader last knew of: a
    /// later one from another device is offered.
    @ObservationIgnored private var knownServerAt: Date?
    @ObservationIgnored private var dismissedOffer: ServerPlace?
    @ObservationIgnored private var placeWatch: Task<Void, Never>?
    @ObservationIgnored private var fileWatch: Task<Void, Never>?
    /// Shisho's chapters as the contents last showed them.
    @ObservationIgnored private var shownChapters: [ReaderChapter]
    /// Asks the app to close the reader, as when the File is gone.
    @ObservationIgnored private let onGone: @MainActor () -> Void
    /// The contents are Shisho's chapters rather than the book's own.
    @ObservationIgnored private var chaptersFromShisho = false
    /// The reader closed, so a load still running stops where it is.
    @ObservationIgnored private var closed = false
    @ObservationIgnored private var reloading: Task<Void, Never>?
    /// The book couldn't open for want of a connection: it opens once the
    /// connection is back.
    @ObservationIgnored private var waitingForConnection = false
    /// A page at the end showed, which finishes the Read-through.
    @ObservationIgnored private var readToEnd = false
    /// The copy this open downloaded, which can go if it won't open, to be
    /// downloaded again on Try Again.
    @ObservationIgnored private var fetchedCopy: UUID?

    init(file: ReadingFile, session: AccountSession, requestSync: @escaping @MainActor () -> Void,
         onGone: @escaping @MainActor () -> Void) {
        self.file = file
        self.session = session
        self.onGone = onGone
        settings = ReaderSettings.load()
        tracker = ReadingTracker(fileID: file.fileID, store: session.store, requestSync: requestSync)
        contents = ReaderModel.entries(file.chapters)
        shownChapters = file.chapters
        search.engine = self
    }

    /// The chapter the reader is in: the last one starting at or before the
    /// place.
    var chapterTitle: String? {
        currentEntry?.title
    }

    /// The table of contents' entry for the chapter the reader is in.
    var currentEntry: ContentsEntry? {
        start(atOrBefore: pageStart ?? fraction).map { contents[starts[$0].index] }
    }

    /// Pages left in the chapter and in the book, or nil scrolled. The
    /// chapter's are nil before the contents know where it ends.
    var pagesLeft: (chapter: Int?, book: Int)? {
        guard let place = pageSection else { return nil }
        let section = PagesLeft.Section(from: place.from, to: place.to, page: place.page, pages: place.pages)
        let rate = PagesLeft.pagesPerShare(Array(seenSections.values))
        // Where the page knows the next chapter's page, it counts to it
        // exactly: a chapter's share of the text misplaces its end among
        // pages that are photos.
        let chapter = nextChapter.map { PagesLeft.count(in: section, toChapterOn: $0.page, atTop: $0.atTop) }
            ?? chapterEnd.map { PagesLeft.count(in: section, to: $0, pagesPerShare: rate) }
        return (chapter, PagesLeft.count(in: section, to: 1, pagesPerShare: rate))
    }

    private func noteSection(_ location: PageLocation) {
        guard let index = location.index, let section = location.section, section.to > section.from else { return }
        seenSections[index] = (section.pages, section.to - section.from)
    }

    /// Where the reader is, as the bar and the readout both tell VoiceOver:
    /// the chapter, pages left, and how far through.
    func placeLabel(percent: String, pagesLeft: AttributedString?) -> String {
        var parts = [chapterTitle, pagesLeft.map { String($0.characters) }].compactMap { $0 }
        parts.append(percent)
        return parts.joined(separator: " · ")
    }

    /// Pages left in the chapter, or the book, as the bar and readout say
    /// it. Scrolled there are no pages. Until the contents place the
    /// chapter, or when they can't, the book's count stands in.
    func pagesLeftLabel(inBook: Bool) -> AttributedString? {
        guard let left = pagesLeft else { return nil }
        if inBook {
            return left.book == 0
                ? AttributedString(localized: "Last page in book")
                : AttributedString(localized: "^[\(left.book) page](inflect: true) left in book")
        }
        guard let chapter = left.chapter else { return pagesLeftLabel(inBook: true) }
        return chapter == 0
            ? AttributedString(localized: "Last page in chapter")
            : AttributedString(localized: "^[\(chapter) page](inflect: true) left in chapter")
    }

    /// Where the chapter showing ends: where the next one starts, or the
    /// end of the book.
    private var chapterEnd: Double? {
        // The start after the chapter's is the next one further on: every
        // start at the chapter's own place sorts before it.
        guard let current = start(atOrBefore: pageStart ?? fraction) else { return nil }
        return current + 1 < starts.count ? starts[current + 1].position : 1
    }

    private func indexStarts() {
        starts = contents.enumerated()
            .compactMap { index, entry in entry.position.map { (position: $0, index: index) } }
            .sorted { ($0.position, $0.index) < ($1.position, $1.index) }
        var seen = Set<Int>()
        marks = starts.compactMap { seen.insert(Int(($0.position * 2000).rounded())).inserted ? $0.position : nil }
    }

    /// The last placed start at or before place, as an index into starts.
    private func start(atOrBefore place: Double) -> Int? {
        var low = 0
        var high = starts.count
        while low < high {
            let middle = (low + high) / 2
            if starts[middle].position <= place + 0.0001 { low = middle + 1 } else { high = middle }
        }
        return low > 0 ? low - 1 : nil
    }

    // MARK: - Opening

    /// Fetches the File if it isn't on the device, then opens it where the
    /// reader left off, here or on another device.
    func load() async {
        phase = .loading
        download.reset()
        // Try Again opens new pages, with no search running in them.
        search.close()
        placeWatch?.cancel()
        fileWatch?.cancel()
        let bookFile: URL
        waitingForConnection = false
        fetchedCopy = nil
        do {
            if let downloaded = session.downloads.localFile(fileID: file.fileID) {
                bookFile = downloaded
            } else if !session.client.reachability.allowsRequests {
                // Not on the device and no connection: say so at once, and
                // open by itself once the connection is back.
                waitingForConnection = true
                phase = .failed(ConnectionRules.book(session.client.reachability.connection))
                return
            } else {
                // The reader needs all of a book before it can show a page,
                // so opening one downloads it, as a Download like any other.
                let fetch = try await session.downloads.fetchNow(fileID: file.fileID, reporting: download.start())
                bookFile = fetch.url
                // One a background transfer finished meanwhile is a Download
                // like any other, kept if it won't open.
                fetchedCopy = fetch.fetched
            }
        } catch {
            guard !closed else { return }
            log("couldn't fetch the File: \(Diagnostics.describe(error))")
            if !session.client.reachability.allowsRequests {
                waitingForConnection = true
                phase = .failed(ConnectionRules.book(session.client.reachability.connection))
            } else {
                phase = .failed(String(localized: "This book couldn't be downloaded. Check your connection to the server and try again."))
            }
            return
        }
        let fileID = file.fileID
        let ownDevice = session.account.deviceID
        let choice = try? await session.store.pool.read { db in
            (try CatalogStore.ebookOpening(db, fileID: fileID, ownDeviceID: ownDevice),
             try CatalogStore.serverPlace(db, fileID: fileID)?.occurredAt)
        }
        guard !closed else { return }
        knownServerAt = choice?.1
        offer = choice?.0.offer
        let fallback = choice?.0.start == nil ? nil : choice?.0.percentage
        let start: ReaderTarget? = switch choice?.0.start {
        case .cfi(let cfi): .cfi(cfi)
        case .fraction(let fraction): .fraction(fraction)
        // A page is a Page-based File's place, never a Reflowable one's.
        case .page: .fraction(choice?.0.percentage ?? 0)
        case nil: nil
        }
        let source = FoliateSource(bookFile: bookFile, fileType: file.fileType, settings: settings, pages: ReaderPagePool.shared.take(),
                                    language: file.language)
        source.delegate = self
        self.source = source
        // An EPUB with no chapters in Shisho, as when the scan couldn't read
        // its navigation, uses the book's own table of contents.
        chaptersFromShisho = file.usesShishoChapters && !shownChapters.isEmpty
        source.open(at: start, fallback: fallback, chapters: chaptersFromShisho ? shownChapters : nil)
        logMemory("opened")
        watch()
    }

    private static func entries(_ chapters: [ReaderChapter]) -> [ContentsEntry] {
        chapters.enumerated().map { index, chapter in
            ContentsEntry(id: index, title: chapter.title, depth: chapter.depth, target: nil, position: nil)
        }
    }

    // MARK: - Moving

    func go(to entry: ContentsEntry) {
        guard let target = entry.target else { return }
        recordJump()
        hideChromeToRead()
        source?.jump(to: target)
    }

    /// After a jump from a sheet, the page shows clear of the bars, which
    /// would otherwise cover its first and last lines. With VoiceOver they
    /// stay, as on a turn, so they don't move out from under its focus.
    func hideChromeToRead() {
        guard !UIAccessibility.isVoiceOverRunning else { return }
        chromeVisible = false
    }

    // MARK: - Back and forward

    /// The places jumps left from (LocationHistory).
    private(set) var history = LocationHistory()

    /// The history before the jump under way, and where it left from: a
    /// jump that lands where it left records nothing, as the web reader's
    /// rules have it (app/libraries/locationHistory.ts).
    @ObservationIgnored private var unmovedCheck: (saved: LocationHistory, origin: String)?

    /// Notes where the reader is before a jump, so they can come back. A
    /// jump made before the last one lands records nothing: the reader is
    /// still where the first left from, which it recorded. Jumps in one
    /// group, as one search's results, record only the first that moves.
    private func recordJump(group: Int? = nil) {
        // Any jump ends a run of the scrubber's adjustments; one of those
        // sets it again after.
        lastAdjustment = nil
        guard source?.isJumping != true, let cfi = location?.cfi else { return }
        unmovedCheck = (history, cfi)
        history.recordJump(from: cfi, group: group)
    }

    func foliateJumpLanded() {
        guard let check = unmovedCheck else { return }
        unmovedCheck = nil
        // The same CFI: the same page when paginated, so nothing moved.
        if location?.cfi == check.origin { history = check.saved }
    }

    /// Back to where the reader was before the last jump. Not while a jump
    /// is landing: the place showing isn't where the reader is going, and
    /// going back from it would drop the entry for it.
    func goBack() {
        guard source?.isJumping != true, let cfi = location?.cfi, let target = history.back(from: cfi) else { return }
        lastAdjustment = nil
        unmovedCheck = nil
        source?.jump(to: .cfi(target))
    }

    /// Forward to where the reader was when they went back.
    func goForward() {
        guard source?.isJumping != true, let cfi = location?.cfi, let target = history.forward(from: cfi) else { return }
        lastAdjustment = nil
        unmovedCheck = nil
        source?.jump(to: .cfi(target))
    }

    func foliatePull(_ direction: TurnDirection?, progress: Double, destination: Double?) {
        let wasArmed = (pull?.progress ?? 0) >= 1
        pull = direction.map { ($0, progress, destination.flatMap(chapterTitle(at:))) }
        // A tick when letting go would turn, as a pull to refresh gives.
        if !wasArmed, progress >= 1, direction != nil {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
        }
    }

    /// The chapter at fraction of the way through, as the scrubber's label
    /// names where it would go.
    func chapterTitle(at fraction: Double) -> String? {
        start(atOrBefore: fraction).map { contents[starts[$0].index].title }
    }

    func foliateImage(_ dataURL: String) {
        Task { [weak self] in
            let image = await Task.detached(priority: .userInitiated) { ReaderModel.decodeImage(dataURL) }.value
            guard let self, !self.closed, let image else { return }
            self.image = OpenImage(image: image)
        }
    }

    /// The longest side, in pixels, an image opens at: about three times a
    /// phone's screen, room to zoom without holding a huge map or comic
    /// page's every pixel.
    nonisolated static let largestImageSide = 3000

    nonisolated static func decodeImage(_ dataURL: String) -> UIImage? {
        guard let comma = dataURL.firstIndex(of: ","),
              let data = Data(base64Encoded: String(dataURL[dataURL.index(after: comma)...])),
              let source = CGImageSourceCreateWithData(data as CFData, nil)
        else { return nil }
        // Scaled down and decoded here, off the main thread, rather than at
        // full size as it first draws.
        let options: [CFString: Any] = [
            kCGImageSourceCreateThumbnailFromImageAlways: true,
            kCGImageSourceCreateThumbnailWithTransform: true,
            kCGImageSourceShouldCacheImmediately: true,
            kCGImageSourceThumbnailMaxPixelSize: largestImageSide,
        ]
        guard let image = CGImageSourceCreateThumbnailAtIndex(source, 0, options as CFDictionary) else { return nil }
        return UIImage(cgImage: image)
    }

    func foliateFollowLink(_ href: String) {
        recordJump()
        source?.jump(to: .href(href))
    }

    /// Goes to fraction of the way through, from the scrubber. A run of
    /// VoiceOver's adjustments, each a few seconds from the last, is one
    /// jump, so going back returns to where the run started.
    func go(toFraction value: Double, adjusting: Bool = false) {
        let now = Date()
        if !adjusting || lastAdjustment.map({ now.timeIntervalSince($0) > 3 }) ?? true {
            recordJump()
        }
        if adjusting { lastAdjustment = now }
        source?.jump(to: .fraction(value))
    }

    @ObservationIgnored private var lastAdjustment: Date?

    func turn(_ direction: TurnDirection) {
        source?.pager.turn(direction)
    }

    func goToNote() {
        guard let note else { return }
        self.note = nil
        recordJump()
        source?.jump(to: .href(note.href))
    }

    func closeNote() {
        note = nil
    }

    func acceptOffer() {
        guard let offer else { return }
        self.offer = nil
        resolveOffer()
        recordJump()
        source?.jump(to: offer.cfi.map(ReaderTarget.cfi) ?? .fraction(offer.percentage), fallback: offer.percentage)
    }

    func dismissOffer() {
        dismissedOffer = offer
        offer = nil
        // Staying keeps this place for the next open, even with no turn of
        // this device's to send.
        if let cfi = location?.cfi {
            try? session.store.stayHere(fileID: file.fileID, cfi: cfi, percentage: fraction, at: Date())
        } else {
            resolveOffer()
        }
    }

    private func resolveOffer() {
        try? session.store.resolveOffer(fileID: file.fileID)
    }

    // MARK: - Search

    func startSearch(_ query: String) -> Int? {
        source?.search(query)
    }

    func cancelSearch() {
        source?.cancelSearch()
    }

    func markResult(_ cfi: String?) {
        source?.markResult(cfi)
    }

    func goToResult(_ cfi: String, group: Int) {
        recordJump(group: group)
        source?.jump(to: .cfi(cfi))
    }

    func foliateSearchResults(_ results: [PageMessage.SearchResult], token: Int) {
        search.received(results, token: token)
    }

    func foliateSearchDone(token: Int) {
        search.finished(token: token)
    }

    func foliateSearchInterrupted(token: Int) {
        search.interrupted(token: token)
    }

    // MARK: - The app's life

    func background() {
        noteReaderPlace()
        tracker.background()
    }

    func foreground() {
        tracker.foreground()
    }

    /// The connection came back: a book that couldn't open for want of it
    /// opens now.
    func connectionChanged(_ connection: Connection) {
        guard connection == .online, waitingForConnection, !closed else { return }
        waitingForConnection = false
        reload()
    }

    /// Loads again, as Try Again and a connection coming back do. Closing
    /// the reader cancels it, as it cancels the first load, so a download
    /// under way carries on as a Download (`DownloadManager.fetchNow`).
    func reload() {
        reloading?.cancel()
        reloading = Task { await load() }
    }

    func memoryWarning() {
        logMemory("memory warning")
    }

    func close() {
        noteReaderPlace()
        if !closed {
            session.downloads.released(fileID: file.fileID)
        }
        closed = true
        reloading?.cancel()
        turnWait?.cancel()
        tracker.close()
        placeWatch?.cancel()
        fileWatch?.cancel()
        logMemory("closed")
    }

    // MARK: - Following the catalog

    /// Follows the File in the catalog: chapter edits show at once, and a
    /// place another device synced while reading is offered, never jumped
    /// to.
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
        // A File removed from the user's Libraries closes the reader, as it
        // closes the player.
        fileWatch = Task { [weak self] in
            let files = ValueObservation.tracking { db in try CatalogStore.readingFile(db, fileID: fileID) }.removeDuplicates()
            do {
                for try await updated in files.values(in: pool) {
                    guard let self else { return }
                    guard let updated else {
                        self.onGone()
                        return
                    }
                    self.chaptersChanged(updated.chapters)
                }
            } catch {}
        }
    }

    private func serverPlaceChanged(_ place: ServerPlace?) {
        guard let place, !place.isSameMove(as: dismissedOffer) else { return }
        // A renamed Device sends its place again: the offer shown takes the
        // new name.
        if place.isSameMove(as: offer) {
            if place != offer { offer = place }
            return
        }
        let here = (cfi: location?.cfi, percentage: location?.fraction ?? fraction)
        if let offered = PlaceRules.offerWhileReading(server: place, at: here, known: knownServerAt,
                                                      ownDeviceID: session.account.deviceID) {
            offer = offered
        }
    }

    private func chaptersChanged(_ chapters: [ReaderChapter]) {
        guard chaptersFromShisho, chapters != shownChapters else { return }
        shownChapters = chapters
        contents = ReaderModel.entries(chapters)
        source?.updateChapters(chapters)
    }

    // MARK: - FoliateSourceDelegate

    func foliateOpened(toc: [PageMessage.TocItem]) {
        if !chaptersFromShisho {
            contents = toc.enumerated().map { index, item in
                ContentsEntry(id: index, title: item.label, depth: item.depth, target: nil, position: nil)
            }
        }
    }

    func foliateShowed(_ location: PageLocation) {
        self.location = location
        fraction = location.fraction ?? 0
        pageStart = location.start
        pageSection = location.section
        nextChapter = location.nextChapter
        noteSection(location)
        // A page reopened after WebKit ended its process shows the place
        // again; the session goes on.
        guard phase != .reading else { return }
        phase = .reading
        tracker.opened(location)
    }

    func foliateMoved(_ location: PageLocation) {
        self.location = location
        pageStart = location.start
        pageSection = location.section
        nextChapter = location.nextChapter
        noteSection(location)
        fraction = location.fraction ?? fraction
        tracker.moved(location)
        if location.atEnd == true, !readToEnd {
            readToEnd = true
            // The pages read the file as they open, so it goes once the
            // reader closes, or at the next launch.
            session.downloads.finished(fileID: file.fileID, holding: true)
        } else if location.atEnd != true, readToEnd {
            // Back from the end, which undoes an automatic finish.
            readToEnd = false
            session.downloads.unfinished(fileID: file.fileID)
        }
    }

    func foliateLaidOut(_ location: PageLocation) {
        guard phase == .reading else { return }
        self.location = location
        pageStart = location.start ?? pageStart
        pageSection = location.section
        nextChapter = location.nextChapter
        noteSection(location)
        fraction = location.fraction ?? fraction
    }

    func foliateContents(_ entries: [PageMessage.ContentsEntry]) {
        guard entries.count == contents.count else { return }
        // Changed as a copy and set once, so the starts are indexed once.
        var placed = contents
        for (index, entry) in entries.enumerated() {
            placed[index].target = entry.target.map(ReaderTarget.href)
            placed[index].position = entry.position
        }
        contents = placed
    }

    func foliateNote(paragraphs: [String], at rect: CGRect, href: String, font: NoteFont, lineHeight: Double?) {
        // A chosen font replaces the book's in the note as on the page.
        let chosen = ReaderFonts.noteFont(for: settings.font, store: ImportedFontStore.shared)
        // The reader's line spacing, as the web's popover takes it, rather
        // than whatever the book sets around the reference.
        note = OpenNote(paragraphs: paragraphs, rect: rect, href: href, font: chosen ?? font,
                        size: NoteFont.size(forReaderSize: settings.size), lineHeight: settings.lineSpacing)
    }

    func foliateWaiting(_ waiting: Bool) {
        turnWait?.cancel()
        guard waiting else {
            turnWaiting = false
            return
        }
        // Not for a page that lands in a moment, so quick turns don't
        // flicker.
        turnWait = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }
            self?.turnWaiting = true
        }
    }

    func foliateExternalLink(_ url: URL) {
        UIApplication.shared.open(url)
    }

    func foliateFailed() {
        guard phase == .loading || source != nil else { return }
        // A copy this open just downloaded that won't open is downloaded
        // again on Try Again. One downloaded before stays: a page that fails
        // under memory pressure, say on a plane, mustn't cost the only copy.
        // So does a newer one a transfer put in its place meanwhile.
        if phase == .loading, let fetchedCopy {
            session.downloads.discard(fileID: file.fileID, copy: fetchedCopy)
        }
        phase = .failed(String(localized: "This book couldn't be opened."))
        // The session ends with the page; time on the error screen isn't
        // reading.
        tracker.close()
    }

    func foliateSawInput() {
        tracker.input()
    }

    func foliateTurning() {
        // With VoiceOver the bars stay: hiding them on every turn would
        // move them out from under the reader's focus.
        guard !UIAccessibility.isVoiceOverRunning else { return }
        chromeVisible = false
    }

    /// Keeps the chapter and pages left for the Book's page, which can't
    /// work them out without the book.
    private func noteReaderPlace() {
        guard phase == .reading, location != nil else { return }
        try? session.store.noteReaderPlace(fileID: file.fileID, chapter: chapterTitle, pagesLeft: pagesLeft?.book,
                                           percentage: fraction)
    }

    func foliateToggleChrome() {
        chromeVisible.toggle()
    }

    // MARK: - Diagnostics

    /// Three WebViews each hold a copy of the book (ADR 0026), so the log
    /// notes the book's size and the app's memory as a book opens and
    /// closes, and on memory warnings. The WebViews' own memory is in
    /// WebKit's processes, which the app can't measure; when iOS ends one,
    /// FoliateSource logs it.
    private func logMemory(_ event: String) {
        log("\(event), \(file.fileType), \(file.sizeBytes / 1024) KB; app footprint \(MemoryFootprint.megabytes) MB")
    }

    private func log(_ message: String) {
        let fileID = file.fileID
        Task { await Diagnostics.shared.log(.reader, "file \(fileID): \(message)") }
    }
}
