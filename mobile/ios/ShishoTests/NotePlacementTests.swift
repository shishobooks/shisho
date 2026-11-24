import XCTest
@testable import Shisho

/// Where a footnote's card sits beside its reference (#919).
final class NotePlacementTests: XCTestCase {
    private let page = CGSize(width: 390, height: 800)
    private let card = CGSize(width: 358, height: 150)

    func testItSitsAboveAReferenceWithRoomAbove() {
        let placement = NotePlacement(reference: CGRect(x: 300, y: 600, width: 10, height: 20), card: card, in: page)
        XCTAssertTrue(placement.above)
        XCTAssertEqual(placement.origin.y + card.height + NotePlacement.arrowHeight, 600 - NotePlacement.gap, accuracy: 0.5)
    }

    func testItSitsBelowAReferenceNearTheTop() {
        let placement = NotePlacement(reference: CGRect(x: 40, y: 60, width: 10, height: 20), card: card, in: page)
        XCTAssertFalse(placement.above)
        XCTAssertEqual(placement.origin.y, 80 + NotePlacement.gap + NotePlacement.arrowHeight, accuracy: 0.5)
    }

    /// The card stays inside the page's margins, and its arrow still
    /// points at the reference, short of the card's rounded corners.
    func testTheCardStaysOnThePageAndTheArrowOnTheReference() {
        let placement = NotePlacement(reference: CGRect(x: 375, y: 600, width: 10, height: 20), card: card, in: page)
        XCTAssertEqual(placement.origin.x, 390 - NotePlacement.margin - card.width, accuracy: 0.5)
        XCTAssertEqual(placement.arrowX, card.width - NotePlacement.radius - NotePlacement.arrowWidth / 2, accuracy: 0.5)

        let middle = NotePlacement(reference: CGRect(x: 190, y: 600, width: 10, height: 20), card: card, in: page)
        XCTAssertEqual(middle.origin.x + middle.arrowX, 195, accuracy: 0.5)
    }
}
