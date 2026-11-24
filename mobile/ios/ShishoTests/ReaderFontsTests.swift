import XCTest
@testable import Shisho

/// The reader's fonts: the bundled ones, unmodified under their own names,
/// and fonts imported from Files, kept per installation and removable
/// (#920).
@MainActor
final class ReaderFontsTests: XCTestCase {
    private var folder: URL!

    override func setUp() async throws {
        folder = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
    }

    override func tearDown() async throws {
        try? FileManager.default.removeItem(at: folder)
    }

    /// A font file to import, copied from the app's own bundled fonts under
    /// another name, as a reader's would be.
    private func fontFile(named name: String, from bundled: String = "fonts/libron/Libron-Regular.woff2") throws -> URL {
        let source = try XCTUnwrap(Bundle(for: ImportedFontStore.self).resourceURL?.appendingPathComponent(bundled))
        let picked = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        try FileManager.default.createDirectory(at: picked, withIntermediateDirectories: true)
        let file = picked.appendingPathComponent(name)
        try FileManager.default.copyItem(at: source, to: file)
        return file
    }

    /// The web reader's list, in its order, and every face is in the app.
    func testTheBundledFontsAreTheWebReadersAndAllThere() throws {
        XCTAssertEqual(BundledFont.allCases.map(\.name),
                       ["Libron", "Literata", "Atkinson Hyperlegible", "Noto Serif", "Noto Sans", "OpenDyslexic"])
        let resources = try XCTUnwrap(Bundle(for: ImportedFontStore.self).resourceURL)
        for font in BundledFont.allCases {
            for face in font.faces {
                XCTAssertTrue(FileManager.default.fileExists(atPath: resources.appendingPathComponent(face.path).path), face.path)
            }
            // Its license ships beside it.
            let license = resources.appendingPathComponent("fonts/\(font.rawValue)/OFL.txt")
            XCTAssertTrue(FileManager.default.fileExists(atPath: license.path), font.rawValue)
        }
    }

    func testAnImportedFontIsKeptAndListedByItsFilesName() throws {
        let store = ImportedFontStore(folder: folder)
        let font = try store.importFont(from: fontFile(named: "My Serif.woff2"))
        XCTAssertEqual(font.name, "My Serif")
        XCTAssertEqual(font.format, .woff2)
        XCTAssertEqual(store.fonts, [font])
        // Another launch finds it.
        XCTAssertEqual(ImportedFontStore(folder: folder).fonts, [font])
        XCTAssertNotNil(store.data(for: font.id))
    }

    /// Ids only grow, so a font removed and another imported never take
    /// each other's place in a setting.
    func testIdsAreNeverReused() throws {
        let store = ImportedFontStore(folder: folder)
        let first = try store.importFont(from: fontFile(named: "One.ttf", from: "fonts/noto-sans/NotoSans-Variable.ttf"))
        store.remove(first.id)
        let second = try store.importFont(from: fontFile(named: "Two.otf", from: "fonts/noto-sans/NotoSans-Variable.ttf"))
        XCTAssertGreaterThan(second.id, first.id)
        XCTAssertEqual(second.format, .opentype)
        XCTAssertEqual(store.fonts.map(\.name), ["Two"])
    }

    func testARemovedFontIsGoneWithItsFile() throws {
        let store = ImportedFontStore(folder: folder)
        let font = try store.importFont(from: fontFile(named: "Gone.woff2"))
        store.remove(font.id)
        XCTAssertEqual(store.fonts, [])
        XCTAssertNil(store.data(for: font.id))
        XCTAssertEqual(ImportedFontStore(folder: folder).fonts, [])
    }

    /// Only TTF, OTF, and WOFF2, and only files that really are fonts, each
    /// refusal saying what was wrong.
    func testOnlyFontsAreImported() throws {
        let store = ImportedFontStore(folder: folder)
        XCTAssertThrowsError(try store.importFont(from: fontFile(named: "Notes.txt"))) { error in
            XCTAssertEqual((error as? FontImportError)?.message, "Choose a TTF, OTF, or WOFF2 font file.")
        }
        XCTAssertThrowsError(try store.importFont(from: fontFile(named: "Fake.ttf", from: "fonts/libron/OFL.txt"))) { error in
            XCTAssertEqual((error as? FontImportError)?.message, "Fake.ttf isn't a font Shisho can read.")
        }
        XCTAssertEqual(store.fonts, [])
    }

    /// The page gets an imported font's faces at its own address, named by
    /// id, so a file named like a bundled font can't take its place; a
    /// removed one leaves the book's own fonts.
    func testAnImportedFontsFaceIsNamedByItsId() throws {
        let store = ImportedFontStore(folder: folder)
        let font = try store.importFont(from: fontFile(named: "Libron.woff2"))
        let face = try XCTUnwrap(ReaderFonts.face(for: .imported(font.id), imported: store.fonts))
        XCTAssertEqual(face.family, "Imported Font \(font.id)")
        XCTAssertEqual(face.faces, [ReaderFontFace.Face(src: "shisho-reader://app/imported-fonts/\(font.id)", format: "woff2",
                                                        weight: "normal", style: "normal")])
        XCTAssertNil(ReaderFonts.face(for: .imported(font.id + 1), imported: store.fonts))
        XCTAssertNil(ReaderFonts.face(for: .book, imported: store.fonts))
    }

    /// The footnote card takes the chosen font, which isn't installed: it's
    /// read from the font's own file.
    func testTheNoteCardReadsTheChosenFontsFile() throws {
        let store = ImportedFontStore(folder: folder)
        let font = try store.importFont(from: fontFile(named: "Mine.woff2"))
        XCTAssertEqual(ReaderFonts.noteFont(for: .bundled(.openDyslexic), store: store)?.familyName, "OpenDyslexic")
        XCTAssertEqual(ReaderFonts.noteFont(for: .imported(font.id), store: store)?.familyName, "Libron")
        XCTAssertNil(ReaderFonts.noteFont(for: .book, store: store))
    }
}
