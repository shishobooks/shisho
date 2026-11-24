import Foundation
import Observation

/// What a search inside the Book asks of the pages.
@MainActor
protocol ReaderSearchEngine: AnyObject {
    /// Starts searching the whole book, returning the token its answers
    /// come with, or nil with no book open to search.
    func startSearch(_ query: String) -> Int?
    /// Stops any search under way.
    func cancelSearch()
    /// Marks the chosen result on the page, or none.
    func markResult(_ cfi: String?)
    /// Goes to a result. One search's moves share a group, one jump in the
    /// reader's history.
    func goToResult(_ cfi: String, group: Int)
}

/// Search inside the Book, as the web reader's works (useBookSearch in
/// app/hooks/useBookSearch.ts): the query searches once typing pauses, a new
/// one cancels the last, results arrive a section at a time, and next and
/// previous wrap at the ends. One search's moves count as one jump in the
/// reader's history.
@MainActor
@Observable
final class ReaderSearch {
    struct Group: Identifiable {
        let chapter: String
        let results: [(result: PageMessage.SearchResult, index: Int)]
        var id: Int { results.first?.index ?? 0 }
    }

    private(set) var query = ""
    private(set) var results: [PageMessage.SearchResult] = [] {
        didSet { groups = ReaderSearch.group(results) }
    }
    /// Neighboring results in one chapter, under one heading. A chapter
    /// split across several sections comes in pieces.
    private(set) var groups: [Group] = []
    private(set) var searching = false
    /// A query that searches only when asked is waiting to be.
    private(set) var waiting = false
    private(set) var current: Int?

    /// The pages it searches, set once the reader has them.
    @ObservationIgnored weak var engine: ReaderSearchEngine?
    @ObservationIgnored private let delay: Duration
    /// The query typed, trimmed, and whether a search for it has started.
    @ObservationIgnored private var trimmed = ""
    @ObservationIgnored private var started = false
    @ObservationIgnored private var token: Int?
    @ObservationIgnored private var pause: Task<Void, Never>?
    /// The first result found is gone to, as when next came before any.
    @ObservationIgnored private var goToFirst = false
    /// This query's search was run again after a page ended mid-search.
    @ObservationIgnored private var retried = false
    /// This query's group of jumps, new for each query.
    @ObservationIgnored private var group = 0

    /// delay is how long typing pauses before a search starts: each reads
    /// the whole book, so one per keystroke would mostly be thrown away.
    init(engine: ReaderSearchEngine? = nil, delay: Duration = .milliseconds(300)) {
        self.engine = engine
        self.delay = delay
    }

    /// Whether there's a search to show: a query, with or without results.
    var active: Bool { !trimmed.isEmpty }

    func changeQuery(_ next: String) {
        query = next
        let trimmedNext = next.trimmingCharacters(in: .whitespacesAndNewlines)
        // Spaces typed between words don't search again.
        guard trimmedNext != trimmed else { return }
        trimmed = trimmedNext
        reset()
        guard !trimmed.isEmpty else { return }
        // A single character matches nearly everywhere, so it searches only
        // when asked. One is still a word in Chinese or Japanese.
        guard trimmed.count > 1 else {
            waiting = true
            return
        }
        searching = true
        let delay = delay
        pause = Task { [weak self] in
            try? await Task.sleep(for: delay)
            guard !Task.isCancelled else { return }
            self?.run()
        }
    }

    /// The keyboard's Search: a query waiting, or still in its pause,
    /// searches now.
    func submit() {
        guard !trimmed.isEmpty, !started else { return }
        run()
    }

    /// Clears the query, the results, and the mark on the page.
    func close() {
        query = ""
        trimmed = ""
        reset()
    }

    private func reset() {
        pause?.cancel()
        pause = nil
        engine?.cancelSearch()
        if current != nil { engine?.markResult(nil) }
        token = nil
        started = false
        goToFirst = false
        retried = false
        group += 1
        current = nil
        results = []
        searching = false
        waiting = false
    }

    private func run() {
        pause?.cancel()
        pause = nil
        guard let engine, !trimmed.isEmpty else { return }
        engine.cancelSearch()
        started = true
        waiting = false
        searching = true
        results = []
        token = engine.startSearch(trimmed)
        // No book to search: nothing will answer.
        if token == nil { searching = false }
    }

    // MARK: - From the pages

    func received(_ found: [PageMessage.SearchResult], token: Int) {
        guard token == self.token, !found.isEmpty else { return }
        let wasEmpty = results.isEmpty
        results.append(contentsOf: found)
        if wasEmpty, goToFirst {
            goToFirst = false
            goToResult(0)
        }
    }

    func finished(token: Int) {
        guard token == self.token else { return }
        searching = false
        goToFirst = false
    }

    /// The page searching ended part way, as when iOS ends its web process:
    /// the search runs again from the start once, rather than its first
    /// part passing for all of it. Searching a large book may be what ended
    /// it, so a second time it stops with what it found, and the keyboard's
    /// Search tries again.
    func interrupted(token: Int) {
        guard token == self.token else { return }
        guard !retried else {
            searching = false
            started = false
            goToFirst = false
            return
        }
        retried = true
        if current != nil { engine?.markResult(nil) }
        current = nil
        run()
    }

    // MARK: - Moving

    func goToResult(_ index: Int) {
        guard results.indices.contains(index) else { return }
        current = index
        let cfi = results[index].cfi
        engine?.markResult(cfi)
        engine?.goToResult(cfi, group: group)
    }

    func next() { step(1) }
    func previous() { step(-1) }

    /// Next or previous from the chosen result, wrapping at the ends. With
    /// none chosen yet, next is the first and previous the last.
    private func step(_ direction: Int) {
        if !trimmed.isEmpty, !started {
            // Typing hasn't paused yet, or the query waits to be asked:
            // search now, and go to the first result found.
            run()
            if direction == 1 { goToFirst = true }
            return
        }
        guard !results.isEmpty else {
            if searching, direction == 1 { goToFirst = true }
            return
        }
        let index = current.map { ($0 + direction + results.count) % results.count }
            ?? (direction == 1 ? 0 : results.count - 1)
        goToResult(index)
    }

    // MARK: - Showing

    /// Where the search is, as the web's panel says it, or nil with no
    /// query.
    var status: String? {
        guard !trimmed.isEmpty else { return nil }
        if let current {
            return String(localized: "\(current + 1) of \(results.count)")
        }
        if !results.isEmpty {
            return String(AttributedString(localized: "^[\(results.count) result](inflect: true)").characters)
        }
        if waiting { return String(localized: "Tap Search on the keyboard") }
        return searching ? String(localized: "Searching…") : String(localized: "No results")
    }

    private static func group(_ results: [PageMessage.SearchResult]) -> [Group] {
        var groups: [(chapter: String, results: [(result: PageMessage.SearchResult, index: Int)])] = []
        for (index, result) in results.enumerated() {
            if let last = groups.indices.last, groups[last].chapter == result.chapter {
                groups[last].results.append((result, index))
            } else {
                groups.append((result.chapter, [(result, index)]))
            }
        }
        return groups.map { Group(chapter: $0.chapter, results: $0.results) }
    }
}
