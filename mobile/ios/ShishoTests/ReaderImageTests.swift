@testable import Shisho
import UIKit
import XCTest

final class ReaderImageTests: XCTestCase {
    private func dataURL(width: Int, height: Int) -> String {
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        let image = UIGraphicsImageRenderer(size: CGSize(width: width, height: height), format: format).image { context in
            UIColor.red.setFill()
            context.fill(CGRect(x: 0, y: 0, width: width, height: height))
        }
        return "data:image/png;base64," + (image.pngData() ?? Data()).base64EncodedString()
    }

    /// A huge map or comic page opens scaled down, so it can't take the
    /// memory its every pixel would.
    func testAHugeImageOpensScaledDown() throws {
        let image = try XCTUnwrap(ReaderModel.decodeImage(dataURL(width: 6000, height: 1500)))
        let pixels = try XCTUnwrap(image.cgImage)
        XCTAssertEqual(pixels.width, ReaderModel.largestImageSide)
        XCTAssertEqual(pixels.height, 750)
    }

    func testASmallImageOpensAsItIs() throws {
        let image = try XCTUnwrap(ReaderModel.decodeImage(dataURL(width: 300, height: 200)))
        let pixels = try XCTUnwrap(image.cgImage)
        XCTAssertEqual(pixels.width, 300)
        XCTAssertEqual(pixels.height, 200)
    }

    func testSomethingThatIsntAnImageOpensNothing() {
        XCTAssertNil(ReaderModel.decodeImage("data:image/svg+xml;base64," + Data("<svg/>".utf8).base64EncodedString()))
        XCTAssertNil(ReaderModel.decodeImage("not a data URL"))
    }
}
