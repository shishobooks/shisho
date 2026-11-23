import UIKit
import XCTest
@testable import Shisho

/// Every Lucide icon the app names is in its asset catalog. A name that
/// isn't draws nothing, without a word, until `mise ios:icons` writes it.
final class LucideIconsTests: XCTestCase {
    func testEveryIconIsInTheAssetCatalog() {
        for icon in Lucide.allCases {
            XCTAssertNotNil(UIImage(named: "Lucide/\(icon.rawValue)"), icon.rawValue)
        }
    }
}
