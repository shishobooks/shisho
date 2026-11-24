import Foundation

/// Where the reader is, as the tracker records it: a CFI in a Reflowable
/// File, or a 0-based page in a Page-based one.
struct ReadingPlace: Equatable, Sendable {
    enum Locator: Equatable, Sendable {
        case cfi(String)
        case page(Int)
    }

    /// Nil before foliate names the place, which saves nothing.
    var locator: Locator?
    var fraction: Double
    /// The reader is at the end of the File, which finishes the
    /// Read-through.
    var atEnd: Bool
    /// Which screen shows, for timing reading per page.
    var screen: String

    init(locator: Locator?, fraction: Double, atEnd: Bool, screen: String) {
        self.locator = locator
        self.fraction = fraction
        self.atEnd = atEnd
        self.screen = screen
    }

    init(_ location: PageLocation) {
        self.init(locator: location.cfi.map(Locator.cfi), fraction: location.fraction ?? 0, atEnd: location.atEnd ?? false,
                  screen: location.screen ?? location.cfi ?? "")
    }
}

/// Keeps an ebook File's Reading Position and Reading Sessions current
/// while it's open, in the account's journal (ADR 0024).
///
/// Every move saves the place, a CFI or a page, with the session so far, so
/// a kill keeps both.
/// Where the reader opened isn't reading and saves nothing. Sessions follow
/// `ReadingRules`; one that ends asks for a sync. Read-throughs start and
/// finish on the server from the positions, as for the web readers.
@MainActor
final class ReadingTracker {
    let fileID: Int
    private let store: CatalogStore
    private let requestSync: @MainActor () -> Void
    private let now: () -> Date
    private var session: ReadingSession?
    /// Where the reader is, once it showed a page.
    private var location: ReadingPlace?
    /// The place last saved, so the same place isn't saved twice.
    private var saved: (locator: ReadingPlace.Locator, atEnd: Bool)?
    /// The app is off the screen, so nothing counts until it's back.
    private var away = false
    private var idleCheck: Task<Void, Never>?

    init(fileID: Int, store: CatalogStore, now: @escaping () -> Date = Date.init,
         requestSync: @escaping @MainActor () -> Void) {
        self.fileID = fileID
        self.store = store
        self.now = now
        self.requestSync = requestSync
    }

    /// The first page showed. Opening counts as input, so its time counts
    /// from now, not from while the book was loading.
    func opened(_ location: PageLocation) {
        opened(ReadingPlace(location))
    }

    func opened(_ location: ReadingPlace) {
        // Opened again, as by Try Again: the session before ends here.
        if let earlier = session, earlier.awayAt == nil {
            record(ReadingRules.snapshot(earlier, at: now()))
        }
        self.location = location
        if let locator = location.locator {
            saved = (locator, location.atEnd)
        }
        let begun = ReadingRules.begin(fileID: fileID, page: location.screen, percentage: location.fraction, at: now())
        // A book that finished opening while the app was away counts from
        // when it's back.
        session = away ? ReadingRules.background(begun, at: now()).0 : begun
        startIdleCheck()
    }

    /// The reader moved: the page left counts its time, and the place is
    /// saved with the session so far.
    func moved(_ location: PageLocation) {
        moved(ReadingPlace(location))
    }

    func moved(_ location: ReadingPlace) {
        self.location = location
        let at = now()
        let percentage = location.fraction
        if let current = session {
            session = ReadingRules.turned(current, to: location.screen, percentage: percentage, at: at)
        } else {
            session = ReadingRules.begin(fileID: fileID, page: location.screen, percentage: percentage, at: at)
        }
        guard let locator = location.locator, let current = session else { return }
        let atEnd = location.atEnd
        if let saved, saved.locator == locator, saved.atEnd == atEnd { return }
        saved = (locator, atEnd)
        let (cfi, page): (String?, Int?) = switch locator {
        case .cfi(let cfi): (cfi, nil)
        case .page(let page): (nil, page)
        }
        do {
            try store.recordReading(fileID: fileID, cfi: cfi, page: page, percentage: percentage, atEnd: atEnd, at: at,
                                    sessionID: current.id, sessionStartedAt: current.startedAt,
                                    session: ReadingRules.snapshot(current, at: at))
        } catch {
            log("couldn't save the place: \(Diagnostics.describe(error))")
        }
        if atEnd { sync() }
    }

    /// Input that isn't a move, such as a tap that shows the controls. A
    /// session ended by idling starts again here.
    func input() {
        let at = now()
        if let current = session {
            session = ReadingRules.touched(current, at: at)
        } else if let location {
            session = ReadingRules.begin(fileID: fileID, page: location.screen, percentage: location.fraction, at: at)
        }
    }

    /// The app went to the background or the phone locked: the session ends
    /// here, and goes on if the reader is back within 30 seconds.
    func background() {
        away = true
        if let current = session {
            let (paused, ended) = ReadingRules.background(current, at: now())
            session = paused
            record(ended)
        }
        if hasUnsentMoves { sync() }
    }

    func foreground() {
        away = false
        guard let current = session else { return }
        if let resumed = ReadingRules.foreground(current, at: now()) {
            session = resumed
        } else if let location {
            session = ReadingRules.begin(fileID: fileID, page: location.screen, percentage: location.fraction, at: now())
        }
    }

    /// The reader closed: the session ends now.
    func close() {
        idleCheck?.cancel()
        if let current = session {
            session = nil
            if current.awayAt == nil {
                record(ReadingRules.snapshot(current, at: now()))
            }
        }
        // The place goes out at once, even after pages too quick to count,
        // or a sync asked for earlier that failed.
        if hasUnsentMoves { sync() }
    }

    /// Five idle minutes end the session.
    func checkIdle() {
        guard let current = session, ReadingRules.isIdle(current, at: now()) else { return }
        session = nil
        record(ReadingRules.endIdle(current))
    }

    private func startIdleCheck() {
        idleCheck?.cancel()
        idleCheck = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(15))
                guard let self else { return }
                self.checkIdle()
            }
        }
    }

    private func record(_ ended: FinishedSession?) {
        guard let ended else { return }
        do {
            try store.recordReading(session: ended)
        } catch {
            log("couldn't save the session: \(Diagnostics.describe(error))")
        }
        sync()
    }

    private func sync() {
        requestSync()
    }

    /// Moves of this File wait in the journal.
    private var hasUnsentMoves: Bool {
        let fileID = fileID
        return (try? store.pool.read { try CatalogStore.hasPendingMoves($0, fileID: fileID) }) ?? false
    }

    private func log(_ message: String) {
        let fileID = fileID
        Task { await Diagnostics.shared.log(.reader, "file \(fileID): \(message)") }
    }
}
