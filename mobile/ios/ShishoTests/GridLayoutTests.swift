import XCTest
@testable import Shisho

final class GridLayoutTests: XCTestCase {
    private func columns(_ width: CGFloat, regular: Bool = false, accessibility: Bool = false) -> Int {
        GridLayout(width: width, regular: regular, accessibility: accessibility).columns
    }

    func testAPhoneShowsThreeUprightAndMoreOnItsSide() {
        XCTAssertEqual(columns(402), 3) // iPhone 17, upright
        XCTAssertEqual(columns(375), 3) // the narrowest iPhone still shows three
        XCTAssertEqual(columns(750), 6) // iPhone 17 on its side, inside the safe area
    }

    func testAnIPadFitsMoreColumns() {
        XCTAssertEqual(columns(820, regular: true), 5)
        XCTAssertEqual(columns(1180, regular: true), 7)
    }

    func testAccessibilitySizesUseWiderColumns() {
        XCTAssertEqual(columns(402, accessibility: true), 2)
        XCTAssertEqual(columns(750, accessibility: true), 4)
    }

    /// Columns and gaps fill the width between the margins and never more,
    /// so the grid can't hold the screen wide.
    func testCellsFitTheWidth() {
        for width in stride(from: CGFloat(320), through: 1400, by: 37) {
            let layout = GridLayout(width: width, regular: width > 700, accessibility: false)
            let used = CGFloat(layout.columns) * layout.cellWidth + CGFloat(layout.columns - 1) * GridLayout.spacing
            XCTAssertLessThanOrEqual(used, width - 2 * GridLayout.margin, "width \(width)")
            XCTAssertGreaterThan(used, width - 2 * GridLayout.margin - CGFloat(layout.columns), "width \(width)")
        }
    }
}
