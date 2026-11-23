import XCTest
@testable import Shisho

/// Pages left in the chapter and in the book, under the reader's bar
/// (#919): exact in the section showing, estimated past it from that
/// section's pages.
final class PagesLeftTests: XCTestCase {
    /// Page 3 of 10 in a section a tenth of the book long, from 0.2 to 0.3.
    private let place = PagesLeft.Section(from: 0.2, to: 0.3, page: 3, pages: 10)

    func testAChapterEndingWithTheSectionCountsItsPagesExactly() {
        XCTAssertEqual(PagesLeft.count(in: place, to: 0.3), 7)
    }

    func testTheLastPageOfASectionHasNoneLeftInIt() {
        let last = PagesLeft.Section(from: 0.2, to: 0.3, page: 10, pages: 10)
        XCTAssertEqual(PagesLeft.count(in: last, to: 0.3), 0)
    }

    /// A chapter that ends partway through the section ends on the page its
    /// end falls on.
    func testAChapterEndingInsideTheSection() {
        // Page 3 starts at 0.22; the chapter ends at 0.265, on page 7.
        XCTAssertEqual(PagesLeft.count(in: place, to: 0.265), 4)
    }

    /// Past the section, pages are as long as this section's.
    func testTheBookIsEstimatedFromThisSectionsPages() {
        // 7 here, then 0.7 of the book at 0.01 a page.
        XCTAssertEqual(PagesLeft.count(in: place, to: 1), 77)
    }

    /// Past the section, a rate measured over more of the book stands in
    /// for this section's, which a short section skews.
    func testAMeasuredRateEstimatesPastTheSection() {
        XCTAssertEqual(PagesLeft.count(in: place, to: 1, pagesPerShare: 50), 42)
    }

    /// The rate weighs each section seen by its share of the book, so a
    /// one-page title page barely moves it.
    func testTheRateWeighsSectionsByTheirShare() {
        let rate = PagesLeft.pagesPerShare([(pages: 1, share: 0.0005), (pages: 30, share: 0.03), (pages: 20, share: 0.02)])
        XCTAssertEqual(rate ?? 0, 51 / 0.0505, accuracy: 0.001)
        XCTAssertNil(PagesLeft.pagesPerShare([]))
    }

    /// A fixed-layout book's sections are a page each.
    func testAFixedLayoutPageIsItsSection() {
        let spread = PagesLeft.Section(from: 0.5, to: 0.55, page: 1, pages: 1)
        XCTAssertEqual(PagesLeft.count(in: spread, to: 1), 9)
    }

    func testNothingIsLeftPastTheEnd() {
        XCTAssertEqual(PagesLeft.count(in: place, to: 0.1), 0)
    }
}
