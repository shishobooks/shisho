import XCTest
@testable import Shisho

/// Search inside the Book, as the web reader's search works
/// (app/hooks/useBookSearch.ts): a query searches once typing pauses, a new
/// one cancels the last, results come a section at a time, and stepping
/// through them wraps (#920).
@MainActor
final class ReaderSearchTests: XCTestCase {
    private final class Engine: ReaderSearchEngine {
        var started: [(query: String, token: Int)] = []
        var cancels = 0
        var marks: [String?] = []
        var moves: [(cfi: String, group: Int)] = []
        private var tokens = 0

        var opened = true

        func startSearch(_ query: String) -> Int? {
            guard opened else { return nil }
            tokens += 1
            started.append((query, tokens))
            return tokens
        }

        func cancelSearch() { cancels += 1 }
        func markResult(_ cfi: String?) { marks.append(cfi) }
        func goToResult(_ cfi: String, group: Int) { moves.append((cfi, group)) }
    }

    private func result(_ cfi: String, _ chapter: String) -> PageMessage.SearchResult {
        PageMessage.SearchResult(cfi: cfi, pre: "before ", match: "word", post: " after", chapter: chapter)
    }

    private func search(_ engine: Engine) -> ReaderSearch {
        ReaderSearch(engine: engine, delay: .zero)
    }

    /// Waits for the pause after typing, which is zero here.
    private func settle() async {
        for _ in 0..<5 { await Task.yield() }
        try? await Task.sleep(for: .milliseconds(20))
    }

    func testAQuerySearchesOnceTypingPauses() async {
        let engine = Engine()
        let search = search(engine)
        search.changeQuery("l")
        search.changeQuery("la")
        search.changeQuery("lam")
        await settle()
        XCTAssertEqual(engine.started.map(\.query), ["lam"])
        XCTAssertTrue(search.searching)
    }

    /// A single character matches nearly everywhere, so it waits to be
    /// asked; one is still a word in Chinese or Japanese.
    func testOneCharacterWaitsToBeAsked() async {
        let engine = Engine()
        let search = search(engine)
        search.changeQuery("a")
        await settle()
        XCTAssertEqual(engine.started.count, 0)
        XCTAssertTrue(search.waiting)
        search.submit()
        XCTAssertEqual(engine.started.map(\.query), ["a"])
        XCTAssertFalse(search.waiting)
    }

    /// Results from a search that a new query replaced are dropped, and
    /// spaces typed between words don't search again.
    func testOnlyTheLatestSearchsResultsCount() async {
        let engine = Engine()
        let search = search(engine)
        search.changeQuery("harbor")
        await settle()
        let first = engine.started[0].token
        search.changeQuery("harbor ")
        await settle()
        XCTAssertEqual(engine.started.count, 1)
        search.changeQuery("harbor wall")
        await settle()
        let second = engine.started[1].token
        search.received([result("a", "One")], token: first)
        search.received([result("b", "One")], token: second)
        XCTAssertEqual(search.results.map(\.cfi), ["b"])
        search.finished(token: first)
        XCTAssertTrue(search.searching)
        search.finished(token: second)
        XCTAssertFalse(search.searching)
    }

    /// Neighbouring results in one chapter share a heading.
    func testResultsGroupByChapter() async {
        let engine = Engine()
        let search = search(engine)
        search.changeQuery("word")
        await settle()
        let token = engine.started[0].token
        search.received([result("a", "One"), result("b", "One")], token: token)
        search.received([result("c", "Two")], token: token)
        XCTAssertEqual(search.groups.map(\.chapter), ["One", "Two"])
        XCTAssertEqual(search.groups.map { $0.results.map(\.index) }, [[0, 1], [2]])
    }

    /// Going to a result marks it and moves there; one search's moves share
    /// a group, one jump in the reader's history, and stepping wraps at the
    /// ends.
    func testSteppingMovesThroughTheResultsAsOneJump() async {
        let engine = Engine()
        let search = search(engine)
        search.changeQuery("word")
        await settle()
        let token = engine.started[0].token
        search.received([result("a", "One"), result("b", "One"), result("c", "Two")], token: token)
        search.goToResult(1)
        XCTAssertEqual(search.current, 1)
        search.next()
        search.next()
        search.previous()
        XCTAssertEqual(engine.moves.map(\.cfi), ["b", "c", "a", "c"])
        XCTAssertEqual(Set(engine.moves.map(\.group)).count, 1)
        XCTAssertEqual(engine.marks.last, "c")
        XCTAssertEqual(search.current, 2)

        // A new query is a new jump.
        let firstGroup = engine.moves[0].group
        search.changeQuery("other")
        await settle()
        search.received([result("d", "One")], token: engine.started[1].token)
        search.next()
        XCTAssertEqual(engine.moves.last?.cfi, "d")
        XCTAssertNotEqual(engine.moves.last?.group, firstGroup)
    }

    /// Next before any result is found goes to the first one found.
    func testNextBeforeResultsGoesToTheFirstFound() async {
        let engine = Engine()
        let search = search(engine)
        search.changeQuery("word")
        await settle()
        search.next()
        XCTAssertTrue(engine.moves.isEmpty)
        search.received([result("a", "One"), result("b", "One")], token: engine.started[0].token)
        XCTAssertEqual(engine.moves.map(\.cfi), ["a"])
        XCTAssertEqual(search.current, 0)
    }

    /// Closing the search stops it and takes its mark off the page.
    func testClosingStopsTheSearchAndUnmarks() async {
        let engine = Engine()
        let search = search(engine)
        search.changeQuery("word")
        await settle()
        search.received([result("a", "One")], token: engine.started[0].token)
        search.goToResult(0)
        let cancels = engine.cancels
        search.close()
        XCTAssertGreaterThan(engine.cancels, cancels)
        XCTAssertEqual(engine.marks.last, .some(nil))
        XCTAssertEqual(search.query, "")
        XCTAssertTrue(search.results.isEmpty)
        XCTAssertNil(search.current)
    }

    /// What the status line says, as the web's panel does.
    func testTheStatusSaysWhereTheSearchIs() async {
        let engine = Engine()
        let search = search(engine)
        XCTAssertNil(search.status)
        search.changeQuery("a")
        XCTAssertEqual(search.status, "Tap Search on the keyboard")
        search.changeQuery("word")
        await settle()
        XCTAssertEqual(search.status, "Searching…")
        let token = engine.started[0].token
        search.received([result("a", "One"), result("b", "One")], token: token)
        XCTAssertEqual(search.status, "2 results")
        search.goToResult(0)
        XCTAssertEqual(search.status, "1 of 2")
        search.changeQuery("nothing")
        await settle()
        search.finished(token: engine.started[1].token)
        XCTAssertEqual(search.status, "No results")
    }

    /// A page that ends mid-search, as when iOS ends its web process, runs
    /// the search again from the start rather than calling it done.
    func testAnInterruptedSearchRunsAgain() async {
        let engine = Engine()
        let search = search(engine)
        search.changeQuery("word")
        await settle()
        let first = engine.started[0].token
        search.received([result("a", "One")], token: first)
        search.goToResult(0)
        search.interrupted(token: first)
        XCTAssertEqual(engine.started.map(\.query), ["word", "word"])
        XCTAssertTrue(search.results.isEmpty)
        XCTAssertNil(search.current)
        XCTAssertEqual(engine.marks.last, .some(nil))
        XCTAssertTrue(search.searching)
        // The first run's answers no longer count.
        search.finished(token: first)
        XCTAssertTrue(search.searching)
    }

    /// With no book open to search, nothing waits on an answer.
    func testNoBookToSearchLeavesNothingSearching() async {
        let engine = Engine()
        engine.opened = false
        let search = search(engine)
        search.changeQuery("word")
        await settle()
        XCTAssertFalse(search.searching)
        XCTAssertEqual(search.status, "No results")
    }

    /// A second interruption, as when searching the book is what ends the
    /// page, stops with what was found, and the keyboard's Search tries
    /// again.
    func testASecondInterruptionStopsAndCanBeTriedAgain() async {
        let engine = Engine()
        let search = search(engine)
        search.changeQuery("word")
        await settle()
        search.interrupted(token: engine.started[0].token)
        search.received([result("a", "One")], token: engine.started[1].token)
        search.interrupted(token: engine.started[1].token)
        XCTAssertEqual(engine.started.count, 2)
        XCTAssertFalse(search.searching)
        XCTAssertEqual(search.results.map(\.cfi), ["a"])
        search.submit()
        XCTAssertEqual(engine.started.count, 3)
    }
}
