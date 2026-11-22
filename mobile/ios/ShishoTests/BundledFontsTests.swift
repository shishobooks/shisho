import CoreText
import UIKit
import XCTest
@testable import Shisho

/// iOS draws text in the system font when a font name is unknown, so a font
/// missing from `UIAppFonts` or a misspelled name would go unnoticed.
final class BundledFontsTests: XCTestCase {
    func testTheTypefacesAreRegistered() {
        for family in [Theme.Fonts.geist, Theme.Fonts.geistMono] {
            XCTAssertFalse(UIFont.fontNames(forFamilyName: family).isEmpty, "\(family) isn't registered")
        }
    }

    func testTheWordmarkFontsHoldTheirCharacters() throws {
        for (name, text) in [(Theme.Fonts.wordmark, "SHISHO"), (Theme.Fonts.wordmarkJP, "司書")] {
            let font = try XCTUnwrap(UIFont(name: name, size: 20), "\(name) isn't registered")
            let characters = Array(text.utf16)
            var glyphs = [CGGlyph](repeating: 0, count: characters.count)
            XCTAssertTrue(
                CTFontGetGlyphsForCharacters(font as CTFont, characters, &glyphs, characters.count),
                "\(name) is missing characters of \(text)"
            )
        }
    }
}
