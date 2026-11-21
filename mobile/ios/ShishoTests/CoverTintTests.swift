import UIKit
import XCTest
@testable import Shisho

final class CoverTintTests: XCTestCase {
    private func image(_ fill: (CGContext) -> Void) -> UIImage {
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        return UIGraphicsImageRenderer(size: CGSize(width: 40, height: 60), format: format).image { context in
            fill(context.cgContext)
        }
    }

    func testASolidCoverIsItsOwnColor() async throws {
        let red = image { context in
            context.setFillColor(UIColor.red.cgColor)
            context.fill(CGRect(x: 0, y: 0, width: 40, height: 60))
        }
        let average = await CoverTint.average(of: red)
        let tint = try XCTUnwrap(average)
        XCTAssertEqual(tint.red, 1, accuracy: 0.02)
        XCTAssertEqual(tint.green, 0, accuracy: 0.02)
        XCTAssertEqual(tint.blue, 0, accuracy: 0.02)
    }

    func testTwoColorsAverageToTheirMix() async throws {
        let halves = image { context in
            context.setFillColor(UIColor.red.cgColor)
            context.fill(CGRect(x: 0, y: 0, width: 40, height: 30))
            context.setFillColor(UIColor.blue.cgColor)
            context.fill(CGRect(x: 0, y: 30, width: 40, height: 30))
        }
        let average = await CoverTint.average(of: halves)
        let tint = try XCTUnwrap(average)
        XCTAssertEqual(tint.red, 0.5, accuracy: 0.05)
        XCTAssertEqual(tint.green, 0, accuracy: 0.02)
        XCTAssertEqual(tint.blue, 0.5, accuracy: 0.05)
    }

    func testAnImageWithNoPixelsHasNoTint() async {
        let empty = await CoverTint.average(of: UIImage())
        XCTAssertNil(empty)
    }
}
