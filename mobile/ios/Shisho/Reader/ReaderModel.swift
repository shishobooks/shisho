import GRDB
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

/// A footnote shown in a popover beside its reference.
struct OpenNote: Identifiable, Equatable {
    let id = UUID()
    let paragraphs: [String]
    /// The reference's box, in the pager's points.
    let rect: CGRect
    let href: String
    /// The book's font around the reference.
    let font: NoteFont
    /// The book's line height around the reference, as a multiple of the
    /// text's size.
    let lineHeight: Double?
}

/// One open Reflowable File: the pages, where the reader is, and what's
/// recorded as they read.
@MainActor
@Observable
final class ReaderModel: FoliateSourceDelegate, Identifiable {
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
    @ObservationIgnored private var pageStart: Double?
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
    @ObservationIgnored private var starts: [(position: Double, index: Int)] = []
    private(set) var note: OpenNote?
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
            // The app's appearance changing under a chosen theme changes
            // nothing on the page.
            guard settings.script != oldValue.script else { return }
            source?.apply(settings)
        }
    }

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
    /// The reader closed, so a load still running stops where it is.
    @ObservationIgnored private var closed = false

    init(file: ReadingFile, session: AccountSession, requestSync: @escaping @MainActor () -> Void,
         onGone: @escaping @MainActor () -> Void) {
        self.file = file
        self.session = session
        self.onGone = onGone
        settings = ReaderSettings.load()
        tracker = ReadingTracker(fileID: file.fileID, store: session.store, requestSync: requestSync)
        contents = ReaderModel.entries(file.chapters)
        shownChapters = file.chapters
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
    /// it. Scrolled there are no pages, and the chapter's count waits for
    /// the contents to place it.
    func pagesLeftLabel(inBook: Bool) -> AttributedString? {
        guard let left = pagesLeft else { return nil }
        if inBook {
            return left.book == 0
                ? AttributedString(localized: "Last page in book")
                : AttributedString(localized: "^[\(left.book) page](inflect: true) left in book")
        }
        guard let chapter = left.chapter else { return nil }
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
        placeWatch?.cancel()
        fileWatch?.cancel()
        let bookFile: URL
        do {
            bookFile = try await session.ebooks.file(file)
        } catch {
            guard !closed else { return }
            log("couldn't fetch the File: \(Diagnostics.describe(error))")
            phase = .failed(String(localized: "This book couldn't be downloaded. Check your connection to the server and try again."))
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
        case nil: nil
        }
        let source = FoliateSource(bookFile: bookFile, fileType: file.fileType, settings: settings, pages: ReaderPagePool.shared.take())
        source.delegate = self
        self.source = source
        source.open(at: start, fallback: fallback, chapters: file.usesShishoChapters ? shownChapters : nil)
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
        source?.jump(to: target)
    }

    /// Goes to fraction of the way through, from the scrubber.
    func go(toFraction value: Double) {
        source?.jump(to: .fraction(value))
    }

    func turn(_ direction: TurnDirection) {
        source?.pager.turn(direction)
    }

    func goToNote() {
        guard let note else { return }
        self.note = nil
        source?.jump(to: .href(note.href))
    }

    func closeNote() {
        note = nil
    }

    func acceptOffer() {
        guard let offer else { return }
        self.offer = nil
        resolveOffer()
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

    // MARK: - The app's life

    func background() {
        noteReaderPlace()
        tracker.background()
    }

    func foreground() {
        tracker.foreground()
    }

    func memoryWarning() {
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
        guard file.usesShishoChapters, chapters != shownChapters else { return }
        shownChapters = chapters
        contents = ReaderModel.entries(chapters)
        source?.updateChapters(chapters)
    }

    // MARK: - FoliateSourceDelegate

    func foliateOpened(toc: [PageMessage.TocItem]) {
        if !file.usesShishoChapters {
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
        note = OpenNote(paragraphs: paragraphs, rect: rect, href: href, font: font, lineHeight: lineHeight)
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
        if phase == .loading {
            // A copy that won't open is fetched again on Try Again.
            session.ebooks.discard(file)
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

    /// The next chapter's start, or the previous chapter's, from the
    /// contents; by section in a book whose contents can't be placed.
    func foliateSkipChapter(_ direction: TurnDirection) {
        let here = start(atOrBefore: pageStart ?? fraction)
        let place = here.map { starts[$0].position } ?? -1
        let candidates: [Int] = direction == .forward
            ? Array(((here ?? -1) + 1)..<starts.count)
            : Array(stride(from: (here ?? 0) - 1, through: 0, by: -1))
        for k in candidates {
            let entry = contents[starts[k].index]
            let further = direction == .forward ? starts[k].position > place + 0.0001 : starts[k].position < place - 0.0001
            if further, entry.target != nil {
                go(to: entry)
                return
            }
        }
        if starts.isEmpty { source?.skipSection(direction) }
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
        var info = task_vm_info_data_t()
        var count = mach_msg_type_number_t(MemoryLayout<task_vm_info_data_t>.size / MemoryLayout<natural_t>.size)
        let result = withUnsafeMutablePointer(to: &info) {
            $0.withMemoryRebound(to: integer_t.self, capacity: Int(count)) {
                task_info(mach_task_self_, task_flavor_t(TASK_VM_INFO), $0, &count)
            }
        }
        let megabytes = result == KERN_SUCCESS ? Int(info.phys_footprint / 1_048_576) : -1
        log("\(event), \(file.fileType), \(file.sizeBytes / 1024) KB; app footprint \(megabytes) MB")
    }

    private func log(_ message: String) {
        let fileID = file.fileID
        Task { await Diagnostics.shared.log(.reader, "file \(fileID): \(message)") }
    }
}
