import XCTest
@testable import Shisho

/// The reader's theme follows the app's appearance until the user picks
/// one (#919).
final class ReaderSettingsTests: XCTestCase {
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

    /// The appearance isn't a setting: it isn't kept, and a kept copy
    /// without a theme choice, as from an earlier build, keeps its layout
    /// and follows the app.
    func testOnlyTheChoicesAreKept() throws {
        let defaults = try XCTUnwrap(UserDefaults(suiteName: UUID().uuidString))
        var settings = ReaderSettings()
        settings.themeChoice = .black
        settings.layout = .scrolled
        settings.appDark = true
        settings.save(defaults)
        let loaded = ReaderSettings.load(defaults)
        XCTAssertEqual(loaded.themeChoice, .black)
        XCTAssertEqual(loaded.layout, .scrolled)
        XCTAssertFalse(loaded.appDark)

        defaults.set(Data(#"{"theme":"sepia","layout":"scrolled"}"#.utf8), forKey: "reader.settings")
        let older = ReaderSettings.load(defaults)
        XCTAssertEqual(older.themeChoice, .system)
        XCTAssertEqual(older.layout, .scrolled)
    }
}
