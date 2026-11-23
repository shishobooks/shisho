import UIKit
import XCTest
@testable import Shisho

/// The pager under a scripted run of rapid taps and swipes, faster than a
/// finger: every turn happens, and no page is ever left between two pages
/// (#919).
@MainActor
final class PagerControllerTests: XCTestCase {
    /// A book of pageCount pages, whose pages take layoutDelay to lay out
    /// after each turn, as a WebView does.
    private final class Book: PagerSource {
        let pager = PagerController()
        let pageCount: Int
        let layoutDelay: Duration
        private(set) var page: Int
        private(set) var rotations = 0
        /// Each time the pager said a turn started or stopped waiting.
        private(set) var waits: [Bool] = []
        private let views = (back: UIView(), current: UIView(), forward: UIView())

        init(pageCount: Int, at page: Int, layoutDelay: Duration) {
            self.pageCount = pageCount
            self.page = page
            self.layoutDelay = layoutDelay
            pager.attach(self)
            pager.neighbor(.back, is: page == 0 ? .edge : .ready)
            pager.neighbor(.forward, is: page == pageCount - 1 ? .edge : .ready)
        }

        var pagerViews: (back: UIView, current: UIView, forward: UIView) { views }

        func pagerDidRotate(_ direction: TurnDirection) {
            page += direction.rawValue
            rotations += 1
            let landed = page
            Task { [weak self] in
                try? await Task.sleep(for: self?.layoutDelay ?? .zero)
                guard let self, self.page == landed else { return }
                let atEdge = direction == .forward ? landed == self.pageCount - 1 : landed == 0
                self.pager.neighbor(direction, is: atEdge ? .edge : .ready)
            }
        }

        func pagerTakesTap(at point: CGPoint, resting: Bool) -> Bool { false }
        var pagerBlocksDrag: Bool { false }
        func pagerWillTurn() {}
        func pagerSawInput() {}
        func pagerToggleChrome() {}
        func pagerSwipe(_ direction: TurnDirection) {}
        func pagerWaiting(_ waiting: Bool) { waits.append(waiting) }
    }

    private var window: UIWindow!

    private func show(_ book: Book) {
        window = UIWindow(frame: CGRect(x: 0, y: 0, width: 390, height: 844))
        window.rootViewController = book.pager
        window.makeKeyAndVisible()
        book.pager.view.layoutIfNeeded()
    }

    /// Takes the window down, since XCTest keeps every test case alive
    /// until the run ends.
    private func hide() {
        window?.isHidden = true
        window = nil
    }

    private func wait(_ milliseconds: Int) async {
        try? await Task.sleep(for: .milliseconds(milliseconds))
    }

    private func flick(_ pager: PagerController, _ direction: TurnDirection) async {
        let sign: CGFloat = direction == .forward ? -1 : 1
        pager.dragBegan()
        for step in 1...4 {
            pager.dragChanged(sign * 30 * CGFloat(step))
            await wait(16)
        }
        pager.dragEnded(sign * 120, velocity: sign * 1500, completed: true)
    }

    /// Settled: every page square in the frame, the current one on top, and
    /// nothing waiting.
    private func assertSettled(_ book: Book, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertTrue(book.pager.isSettled, "a page was left between two pages", file: file, line: line)
        XCTAssertTrue(book.pager.isResting, "turns were still waiting", file: file, line: line)
    }

    func testTenQuickTapsEachTurnAPage() async {
        let book = Book(pageCount: 100, at: 50, layoutDelay: .milliseconds(60))
        show(book)
        for _ in 0..<10 {
            book.pager.tap(atFraction: 0.9)
            await wait(30)
        }
        await wait(1500)
        XCTAssertEqual(book.page, 60)
        assertSettled(book)

        for _ in 0..<10 {
            book.pager.tap(atFraction: 0.1)
            await wait(30)
        }
        await wait(1500)
        XCTAssertEqual(book.page, 50)
        assertSettled(book)
        hide()
    }

    /// A turn asked for before its page is laid out says it's waiting, so
    /// the reader can show it's loading, and stops once the page turns.
    func testATurnWaitingForItsPageSaysSo() async {
        let book = Book(pageCount: 100, at: 50, layoutDelay: .milliseconds(400))
        show(book)
        book.pager.tap(atFraction: 0.9)
        XCTAssertEqual(book.waits, [])
        await wait(30)
        book.pager.tap(atFraction: 0.9)
        XCTAssertEqual(book.waits, [true])
        await wait(1200)
        XCTAssertEqual(book.page, 52)
        XCTAssertEqual(book.waits, [true, false])
        assertSettled(book)
        hide()
    }

    func testFastFlicksEachTurnAPage() async {
        let book = Book(pageCount: 100, at: 50, layoutDelay: .milliseconds(120))
        show(book)
        for _ in 0..<12 {
            await flick(book.pager, .forward)
            await wait(40)
        }
        await wait(2000)
        XCTAssertEqual(book.page, 62)
        assertSettled(book)
        hide()
    }

    /// Taps, flicks, and slow drags mixed at random never leave a page
    /// between two, and stop at the ends of the book.
    func testARandomRunAlwaysSettles() async {
        var generator = SeededGenerator(seed: 919)
        let book = Book(pageCount: 30, at: 3, layoutDelay: .milliseconds(80))
        show(book)
        for _ in 0..<80 {
            switch Int.random(in: 0..<6, using: &generator) {
            case 0, 1: book.pager.tap(atFraction: 0.9)
            case 2: book.pager.tap(atFraction: 0.1)
            case 3: await flick(book.pager, .forward)
            case 4: await flick(book.pager, .back)
            default:
                // A drag that doesn't go far enough springs back.
                book.pager.dragBegan()
                book.pager.dragChanged(-40)
                await wait(16)
                book.pager.dragEnded(-40, velocity: 0, completed: true)
            }
            await wait(Int.random(in: 10...120, using: &generator))
        }
        await wait(2500)
        assertSettled(book)
        XCTAssertTrue((0..<30).contains(book.page))
        hide()
    }
}

/// A repeatable random sequence, so a failing run can be run again.
private struct SeededGenerator: RandomNumberGenerator {
    private var state: UInt64

    init(seed: UInt64) {
        state = seed
    }

    mutating func next() -> UInt64 {
        state &+= 0x9E37_79B9_7F4A_7C15
        var z = state
        z = (z ^ (z >> 30)) &* 0xBF58_476D_1CE4_E5B9
        z = (z ^ (z >> 27)) &* 0x94D0_49BB_1331_11EB
        return z ^ (z >> 31)
    }
}
