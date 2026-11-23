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
    private(set) var contents: [ContentsEntry] = []
    private(set) var note: OpenNote?
    /// Another device's place, offered without jumping.
    private(set) var offer: ServerPlace?
    var settings: ReaderSettings {
        didSet {
            guard settings != oldValue else { return }
            settings.save()
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
        let place = pageStart ?? fraction
        return contents.last { ($0.position ?? .infinity) <= place + 0.0001 }?.title
    }

    /// Where the chapters start along the book, for the scrubber.
    var marks: [Double] {
        contents.compactMap(\.position)
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
        knownServerAt = choice?.1
        offer = choice?.0.offer
        let fallback = choice?.0.start == nil ? nil : choice?.0.percentage
        let start: ReaderTarget? = switch choice?.0.start {
        case .cfi(let cfi): .cfi(cfi)
        case .fraction(let fraction): .fraction(fraction)
        case nil: nil
        }
        let source = FoliateSource(bookFile: bookFile, settings: settings)
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
        source?.jump(to: offer.cfi.map(ReaderTarget.cfi) ?? .fraction(offer.percentage))
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
        tracker.background()
    }

    func foreground() {
        tracker.foreground()
    }

    func memoryWarning() {
        logMemory("memory warning")
    }

    func close() {
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
        guard let place, place != dismissedOffer, place != offer else { return }
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
        // A page reopened after WebKit ended its process shows the place
        // again; the session goes on.
        guard phase != .reading else { return }
        phase = .reading
        tracker.opened(location)
    }

    func foliateMoved(_ location: PageLocation) {
        self.location = location
        pageStart = location.start
        fraction = location.fraction ?? fraction
        tracker.moved(location)
    }

    func foliateContents(_ entries: [PageMessage.ContentsEntry]) {
        guard entries.count == contents.count else { return }
        for (index, entry) in entries.enumerated() {
            contents[index].target = entry.target.map(ReaderTarget.href)
            contents[index].position = entry.position
        }
    }

    func foliateNote(paragraphs: [String], at rect: CGRect, href: String) {
        note = OpenNote(paragraphs: paragraphs, rect: rect, href: href)
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
    }

    func foliateSawInput() {
        tracker.input()
    }

    func foliateTurning() {
        chromeVisible = false
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
