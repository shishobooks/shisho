import XCTest
@testable import Shisho

/// A footnote's popover takes the font the book sets around its reference,
/// as the page draws it.
final class NoteFontTests: XCTestCase {
    private let installed: Set<String> = ["Georgia", "Palatino", "Times New Roman", "Helvetica", "Courier"]

    func testTheFirstInstalledFamilyIsUsed() {
        XCTAssertEqual(NoteFont.resolve(#""Book Face", 'georgia', serif"#, installed: installed), .family("Georgia"))
        XCTAssertEqual(NoteFont.resolve("Palatino Linotype, Palatino, serif", installed: installed), .family("Palatino"))
    }

    /// A font the book carries itself isn't installed: the generic family
    /// after it stands in, as WebKit draws it.
    func testAGenericFamilyIsWhatWebKitDrawsForIt() {
        XCTAssertEqual(NoteFont.resolve(#""Embedded Serif", serif"#, installed: installed), .family("Times New Roman"))
        XCTAssertEqual(NoteFont.resolve("sans-serif", installed: installed), .family("Helvetica"))
        XCTAssertEqual(NoteFont.resolve("monospace", installed: installed), .family("Courier"))
        XCTAssertEqual(NoteFont.resolve("-apple-system, sans-serif", installed: installed), .system)
    }

    /// No font set, or nothing usable: the page's default, Times.
    func testTheDefaultIsWebKitsStandardFont() {
        XCTAssertEqual(NoteFont.resolve("-webkit-standard", installed: installed), .family("Times New Roman"))
        XCTAssertEqual(NoteFont.resolve("", installed: installed), .family("Times New Roman"))
        XCTAssertEqual(NoteFont.resolve(#""Embedded Only""#, installed: installed), .family("Times New Roman"))
    }

    /// A note's lines are as far apart as the page's: at the reader's 1.45,
    /// a few points more than the font's own spacing; none when the book
    /// gives no line height.
    func testANotesLineSpacingFollowsThePage() {
        let font = NoteFont.family("Times New Roman")
        let spacing = font.lineSpacing(for: 1.45, relativeTo: .callout, base: 16)
        XCTAssertGreaterThan(spacing, 3)
        XCTAssertLessThan(spacing, 9)
        XCTAssertEqual(font.lineSpacing(for: nil, relativeTo: .callout, base: 16), 0)
    }
}
