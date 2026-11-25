import XCTest
@testable import Shisho

/// How a Page-based File's pages lie on the screen (#921): spreads in
/// landscape, the flow a Book opens in, and pages as places.
final class PageLayoutTests: XCTestCase {
    private let portrait = CGSize(width: 2, height: 3)
    private let wide = CGSize(width: 4, height: 3)
    private let strip = CGSize(width: 800, height: 2400)

    func testPortraitShowsOnePageAScreen() {
        XCTAssertEqual(PageLayout.spreads(sizes: Array(repeating: portrait, count: 3), twoUp: false), [[0], [1], [2]])
    }

    /// The cover stands alone, then pages pair up; a last page with no
    /// partner stands alone.
    func testLandscapePairsPagesAfterTheCover() {
        XCTAssertEqual(PageLayout.spreads(sizes: Array(repeating: portrait, count: 6), twoUp: true), [[0], [1, 2], [3, 4], [5]])
    }

    /// A wide page, two printed pages scanned as one, stands alone, and the
    /// page before it doesn't pair across it.
    func testAWidePageStandsAlone() {
        let sizes = [portrait, portrait, wide, portrait, portrait]
        XCTAssertEqual(PageLayout.spreads(sizes: sizes, twoUp: true), [[0], [1], [2], [3, 4]])
    }

    func testTheSpreadShowingAPage() {
        let spreads = [[0], [1, 2], [3, 4], [5]]
        XCTAssertEqual(PageLayout.spread(of: 4, in: spreads), 2)
        XCTAssertEqual(PageLayout.spread(of: 0, in: spreads), 0)
    }

    /// A spread's place is its first page, unless it shows the last, so
    /// reaching the last spread is reaching the end.
    func testASpreadsPlace() {
        XCTAssertEqual(PageLayout.place(of: [3, 4], pageCount: 10), 3)
        XCTAssertEqual(PageLayout.place(of: [8, 9], pageCount: 10), 9)
    }

    /// Scrolled when the pages after the cover are tall strips, as the web
    /// reader judges a webtoon, whatever shape the cover is.
    func testAWebtoonOpensScrolled() {
        XCTAssertEqual(PageLayout.detectedFlow(sizes: [portrait, strip, strip, strip]), .scrolled)
        XCTAssertEqual(PageLayout.detectedFlow(sizes: [strip, portrait, portrait]), .paginated)
        XCTAssertEqual(PageLayout.detectedFlow(sizes: [strip]), .scrolled)
        XCTAssertEqual(PageLayout.detectedFlow(sizes: [portrait, CGSize(width: 10, height: 24)]), .paginated)
    }

    /// Pages count through the File as the web reader counts them.
    func testPagesAndFractions() {
        XCTAssertEqual(PageLayout.fraction(of: 0, pageCount: 5), 0)
        XCTAssertEqual(PageLayout.fraction(of: 4, pageCount: 5), 1)
        XCTAssertEqual(PageLayout.fraction(of: 0, pageCount: 1), 0)
        XCTAssertEqual(PageLayout.page(at: 0.5, pageCount: 5), 2)
        XCTAssertEqual(PageLayout.page(at: 1.2, pageCount: 5), 4)
        XCTAssertEqual(PageLayout.page(at: .nan, pageCount: 5), 0)
    }

    /// A spread's pages share one height and sit side by side, centered.
    func testASpreadFitsTheScreen() {
        let frames = PageLayout.frames(sizes: [portrait, portrait], in: CGSize(width: 1000, height: 600))
        XCTAssertEqual(frames, [CGRect(x: 100, y: 0, width: 400, height: 600), CGRect(x: 500, y: 0, width: 400, height: 600)])
        let tall = PageLayout.frames(sizes: [portrait], in: CGSize(width: 300, height: 600))
        XCTAssertEqual(tall, [CGRect(x: 0, y: 75, width: 300, height: 450)])
    }

    /// Whether right-to-left Files turn that way is kept on the device.
    func testFollowingReadingDirectionIsKept() throws {
        let defaults = try XCTUnwrap(UserDefaults(suiteName: UUID().uuidString))
        var settings = PageReaderSettings.load(defaults)
        XCTAssertTrue(settings.followReadingDirection)
        settings.followReadingDirection = false
        settings.save(defaults)
        XCTAssertFalse(PageReaderSettings.load(defaults).followReadingDirection)
    }

    /// A right-to-left File, as manga, turns right to left while Follow
    /// Reading Direction is on, and left to right once it's off; any other
    /// File always turns left to right.
    func testFollowingReadingDirection() {
        var settings = PageReaderSettings()
        XCTAssertTrue(settings.rightToLeft(fileRunsRightToLeft: true))
        XCTAssertFalse(settings.rightToLeft(fileRunsRightToLeft: false))
        settings.followReadingDirection = false
        XCTAssertFalse(settings.rightToLeft(fileRunsRightToLeft: true))
    }

    /// A PDF's outline, as Shisho's chapters, is the contents: each entry
    /// goes to the page it starts on, placed along the book, and the chapter
    /// showing is the last starting on or before the page.
    func testAnOutlineIsTheContents() {
        let chapters = [
            ReaderChapter(title: "Getting Started", href: nil, depth: 0, startPage: 0),
            ReaderChapter(title: "Part", href: nil, depth: 0),
            ReaderChapter(title: "Troubleshooting", href: nil, depth: 1, startPage: 4),
            ReaderChapter(title: "Past the end", href: nil, depth: 0, startPage: 40),
        ]
        let contents = PageLayout.contents(chapters, pageCount: 6)
        XCTAssertEqual(contents.map(\.target), [.page(0), nil, .page(4), .page(5)])
        XCTAssertEqual(contents.map(\.position), [0, nil, 0.8, 1])
        XCTAssertEqual(PageLayout.entry(atPage: 3, in: contents)?.title, "Getting Started")
        XCTAssertEqual(PageLayout.entry(atPage: 4, in: contents)?.title, "Troubleshooting")
        // An outline out of page order still names the chapter the page is
        // in, the one counted for pages left.
        let unordered = PageLayout.contents([
            ReaderChapter(title: "One", href: nil, depth: 0, startPage: 0),
            ReaderChapter(title: "Three", href: nil, depth: 0, startPage: 8),
            ReaderChapter(title: "Two", href: nil, depth: 0, startPage: 3),
        ], pageCount: 10)
        XCTAssertEqual(PageLayout.entry(atPage: 9, in: unordered)?.title, "Three")
        XCTAssertEqual(PageLayout.entry(atPage: 5, in: unordered)?.title, "Two")
        XCTAssertEqual(PageLayout.pagesLeft(shown: [9], pageCount: 10, contents: unordered).chapter, 0)
    }

    /// Pages left are counted as the ebook reader counts them: to the end
    /// of the chapter, which ends where the next starts, and to the end of
    /// the book, from the last page showing. Without chapters there's only
    /// the book's count.
    func testPagesLeftInTheChapterAndTheBook() {
        let contents = PageLayout.contents([
            ReaderChapter(title: "One", href: nil, depth: 0, startPage: 0),
            ReaderChapter(title: "Two", href: nil, depth: 0, startPage: 4),
        ], pageCount: 10)
        let first = PageLayout.pagesLeft(shown: [1], pageCount: 10, contents: contents)
        XCTAssertEqual(first.chapter, 2)
        XCTAssertEqual(first.book, 8)
        // A spread counts from its last page.
        XCTAssertEqual(PageLayout.pagesLeft(shown: [2, 3], pageCount: 10, contents: contents).chapter, 0)
        let last = PageLayout.pagesLeft(shown: [9], pageCount: 10, contents: contents)
        XCTAssertEqual(last.chapter, 0)
        XCTAssertEqual(last.book, 0)
        XCTAssertNil(PageLayout.pagesLeft(shown: [3], pageCount: 10, contents: []).chapter)
        // A jump to a chapter's first page in a spread that starts in the
        // chapter before counts the chapter jumped to, as its title says.
        let jumped = PageLayout.pagesLeft(shown: [3, 4], at: 4, pageCount: 10, contents: contents)
        XCTAssertEqual(jumped.chapter, 5)
    }
}
