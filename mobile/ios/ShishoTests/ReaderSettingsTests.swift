import XCTest
@testable import Shisho

/// The reader's display settings: the web reader's list, names, ranges, and
/// defaults (app/libraries/readerSettings.ts), kept per installation
/// (#919, #920).
final class ReaderSettingsTests: XCTestCase {
    private func defaults() throws -> UserDefaults {
        try XCTUnwrap(UserDefaults(suiteName: UUID().uuidString))
    }

    func testSystemMatchesTheAppsAppearance() {
        var settings = ReaderSettings()
        XCTAssertEqual(settings.themeChoice, .system)
        settings.appDark = false
        XCTAssertEqual(settings.theme, .light)
        settings.appDark = true
        XCTAssertEqual(settings.theme, .dark)
    }

    func testAChosenThemeStaysWhateverTheAppearance() {
        var settings = ReaderSettings()
        settings.themeChoice = .sepia
        settings.appDark = true
        XCTAssertEqual(settings.theme, .sepia)
        settings.themeChoice = .light
        XCTAssertEqual(settings.theme, .light)
    }

    /// The web reader's defaults.
    func testTheDefaultsAreTheWebReaders() {
        let settings = ReaderSettings()
        XCTAssertEqual(settings.font, .book)
        XCTAssertEqual(settings.size, 16)
        XCTAssertFalse(settings.forceFontSize)
        XCTAssertEqual(settings.lineSpacing, 1.45)
        XCTAssertEqual(settings.margins, 7)
        XCTAssertTrue(settings.justify)
        XCTAssertEqual(settings.layout, .paginated)
        XCTAssertEqual(settings.pages, .auto)
    }

    /// Each number has the web's range and step.
    func testTheRangesAreTheWebReaders() {
        XCTAssertEqual(ReaderSettings.Numeric.size.range, 10...40)
        XCTAssertEqual(ReaderSettings.Numeric.size.step, 0.5)
        XCTAssertEqual(ReaderSettings.Numeric.lineSpacing.range, 1...2.5)
        XCTAssertEqual(ReaderSettings.Numeric.lineSpacing.step, 0.05)
        XCTAssertEqual(ReaderSettings.Numeric.margins.range, 1...15)
        XCTAssertEqual(ReaderSettings.Numeric.margins.step, 1)
    }

    /// The minus and plus buttons move one step, held to the range, and land
    /// on the step exactly, without 0.05's float noise.
    func testASteppedValueStaysOnTheStepAndInRange() {
        XCTAssertEqual(ReaderSettings.Numeric.size.stepped(16, by: 1), 16.5)
        XCTAssertEqual(ReaderSettings.Numeric.size.stepped(10, by: -1), 10)
        XCTAssertEqual(ReaderSettings.Numeric.size.stepped(40, by: 1), 40)
        XCTAssertEqual(ReaderSettings.Numeric.lineSpacing.stepped(1.45, by: 1), 1.5)
        XCTAssertEqual(ReaderSettings.Numeric.lineSpacing.stepped(1.45, by: -1), 1.4)
        XCTAssertEqual(ReaderSettings.Numeric.margins.stepped(15, by: 1), 15)
        XCTAssertEqual(ReaderSettings.Numeric.margins.stepped(1, by: -1), 1)
    }

    /// A slider's value lands on the nearest step.
    func testASliderValueSnapsToTheNearestStep() {
        XCTAssertEqual(ReaderSettings.Numeric.lineSpacing.snapped(1.4999999), 1.5)
        XCTAssertEqual(ReaderSettings.Numeric.size.snapped(17.3), 17.5)
        XCTAssertEqual(ReaderSettings.Numeric.margins.snapped(0.2), 1)
    }

    /// How each number reads, as the web's panel shows it.
    func testTheValuesReadAsTheWebShowsThem() {
        XCTAssertEqual(ReaderSettings.Numeric.size.text(16.5), "16.5 px")
        XCTAssertEqual(ReaderSettings.Numeric.size.text(16), "16 px")
        XCTAssertEqual(ReaderSettings.Numeric.lineSpacing.text(1.5), "1.50")
        XCTAssertEqual(ReaderSettings.Numeric.margins.text(7), "7%")
    }

    /// Fonts are kept as the web keeps them: "book", a bundled font's id, or
    /// "imported:" and the imported font's id.
    func testFontsAreKeptAsTheWebKeepsThem() throws {
        let encoder = JSONEncoder()
        XCTAssertEqual(String(decoding: try encoder.encode(ReaderFont.book), as: UTF8.self), #""book""#)
        XCTAssertEqual(String(decoding: try encoder.encode(ReaderFont.bundled(.atkinsonHyperlegible)), as: UTF8.self),
                       #""atkinson-hyperlegible""#)
        XCTAssertEqual(String(decoding: try encoder.encode(ReaderFont.imported(3)), as: UTF8.self), #""imported:3""#)
        XCTAssertEqual(ReaderFont(rawValue: "opendyslexic"), .bundled(.openDyslexic))
        XCTAssertEqual(ReaderFont(rawValue: "imported:12"), .imported(12))
        XCTAssertNil(ReaderFont(rawValue: "imported:"))
        XCTAssertNil(ReaderFont(rawValue: "comic-sans"))
    }

    func testEverySettingIsKept() throws {
        let defaults = try defaults()
        var settings = ReaderSettings()
        settings.font = .imported(4)
        settings.size = 21.5
        settings.forceFontSize = true
        settings.lineSpacing = 1.8
        settings.margins = 3
        settings.justify = false
        settings.themeChoice = .black
        settings.layout = .scrolled
        settings.pages = .one
        settings.appDark = true
        settings.save(defaults)
        var loaded = ReaderSettings.load(defaults)
        // The app's appearance isn't a setting.
        XCTAssertFalse(loaded.appDark)
        loaded.appDark = true
        XCTAssertEqual(loaded, settings)
    }

    /// A kept copy lacking a setting, as from an earlier build, or holding
    /// one this build can't use, takes that setting's default and keeps the
    /// others, so a setting that later gains or loses an option never
    /// breaks the reader.
    func testASettingThatCantBeUsedTakesItsDefaultAlone() throws {
        let defaults = try defaults()
        defaults.set(Data(#"{"theme":"sepia","layout":"scrolled"}"#.utf8), forKey: "reader.settings")
        let older = ReaderSettings.load(defaults)
        XCTAssertEqual(older.themeChoice, .system)
        XCTAssertEqual(older.layout, .scrolled)
        XCTAssertEqual(older.size, 16)

        defaults.set(Data(#"""
        {"font":"comic-sans","size":80,"lineSpacing":"wide","margins":7.4,"justify":"yes",
         "themeChoice":"neon","layout":"scrolled","pages":"three","forceFontSize":true}
        """#.utf8), forKey: "reader.settings")
        let odd = ReaderSettings.load(defaults)
        XCTAssertEqual(odd.font, .book)
        XCTAssertEqual(odd.size, 16)
        XCTAssertEqual(odd.lineSpacing, 1.45)
        // In range, so it's kept, on the nearest step.
        XCTAssertEqual(odd.margins, 7)
        XCTAssertTrue(odd.justify)
        XCTAssertEqual(odd.themeChoice, .system)
        XCTAssertEqual(odd.layout, .scrolled)
        XCTAssertEqual(odd.pages, .auto)
        XCTAssertTrue(odd.forceFontSize)
    }

    /// What the page styles the book with: the web reader's settings, the
    /// theme's colors, and the chosen font's faces at the reader's own
    /// addresses.
    func testThePageGetsEverySettingAndTheFontsFaces() throws {
        var settings = ReaderSettings()
        settings.font = .bundled(.libron)
        settings.size = 18
        settings.forceFontSize = true
        settings.lineSpacing = 1.6
        settings.margins = 4
        settings.justify = false
        settings.themeChoice = .sepia
        settings.pages = .one
        let face = ReaderFonts.face(for: settings.font, imported: [])
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(settings.script(font: face).utf8)) as? [String: Any])
        XCTAssertEqual(json["theme"] as? String, "sepia")
        XCTAssertEqual(json["fg"] as? String, "#5b4636")
        XCTAssertEqual(json["size"] as? Double, 18)
        XCTAssertEqual(json["forceFontSize"] as? Bool, true)
        XCTAssertEqual(json["lineSpacing"] as? Double, 1.6)
        XCTAssertEqual(json["margins"] as? Double, 4)
        XCTAssertEqual(json["justify"] as? Bool, false)
        XCTAssertEqual(json["layout"] as? String, "paginated")
        XCTAssertEqual(json["pages"] as? String, "one")
        let font = try XCTUnwrap(json["font"] as? [String: Any])
        XCTAssertEqual(font["family"] as? String, "Libron")
        XCTAssertEqual(font["fallback"] as? String, "serif")
        let faces = try XCTUnwrap(font["faces"] as? [[String: String]])
        XCTAssertEqual(faces.count, 4)
        XCTAssertEqual(faces.first?["src"], "shisho-reader://app/fonts/libron/Libron-Regular.woff2")
        XCTAssertEqual(faces.first?["format"], "woff2")

        let bookFont = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(ReaderSettings().script(font: nil).utf8)) as? [String: Any])
        XCTAssertTrue(bookFont["font"] is NSNull)
    }

    /// A change restyles the page when the page would look different: not
    /// for the app's appearance under a chosen theme, but always for a new
    /// font, even an imported one removed as it's unchosen, which is gone
    /// from the fonts both before and after (#920).
    func testWhatRestylesThePage() {
        var dark = ReaderSettings()
        dark.themeChoice = .sepia
        var light = dark
        light.appDark = true
        XCTAssertFalse(light.restyles(from: dark, imported: []))

        var system = ReaderSettings()
        system.appDark = true
        XCTAssertTrue(system.restyles(from: ReaderSettings(), imported: []))

        var removed = ReaderSettings()
        removed.font = .imported(3)
        XCTAssertTrue(ReaderSettings().restyles(from: removed, imported: []))

        var bigger = ReaderSettings()
        bigger.size = 18
        XCTAssertTrue(bigger.restyles(from: ReaderSettings(), imported: []))
    }
}
