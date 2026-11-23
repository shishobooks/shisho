import CoreText
import Foundation
import Observation

/// The font a book is read in: its own, a bundled one, or one imported
/// from Files. Kept as the web reader keeps it (app/libraries/
/// readerSettings.ts): "book", a bundled font's id, or "imported:" and the
/// imported font's id.
enum ReaderFont: Hashable, Sendable, Codable, RawRepresentable {
    case book
    case bundled(BundledFont)
    case imported(Int)

    private static let importedPrefix = "imported:"

    init?(rawValue: String) {
        if rawValue == "book" {
            self = .book
        } else if let font = BundledFont(rawValue: rawValue) {
            self = .bundled(font)
        } else if rawValue.hasPrefix(ReaderFont.importedPrefix),
                  let id = Int(rawValue.dropFirst(ReaderFont.importedPrefix.count)) {
            self = .imported(id)
        } else {
            return nil
        }
    }

    var rawValue: String {
        switch self {
        case .book: "book"
        case .bundled(let font): font.rawValue
        case .imported(let id): "\(ReaderFont.importedPrefix)\(id)"
        }
    }
}

/// The fonts the app carries for the reader, the web reader's own files
/// (app/assets/fonts), in its order. All are under the SIL Open Font
/// License, bundled unmodified under their own names with their licenses
/// beside them.
enum BundledFont: String, CaseIterable, Sendable {
    case libron
    case literata
    case atkinsonHyperlegible = "atkinson-hyperlegible"
    case notoSerif = "noto-serif"
    case notoSans = "noto-sans"
    case openDyslexic = "opendyslexic"

    /// The font's own name, never translated.
    var name: String {
        switch self {
        case .libron: "Libron"
        case .literata: "Literata"
        case .atkinsonHyperlegible: "Atkinson Hyperlegible"
        case .notoSerif: "Noto Serif"
        case .notoSans: "Noto Sans"
        case .openDyslexic: "OpenDyslexic"
        }
    }

    var fallback: String {
        switch self {
        case .libron, .literata, .notoSerif: "serif"
        case .atkinsonHyperlegible, .notoSans, .openDyslexic: "sans-serif"
        }
    }

    struct Face: Sendable {
        /// The file in the app's bundle.
        let path: String
        let format: String
        /// A range for a variable font.
        let weight: String
        let style: String
    }

    var faces: [Face] {
        func fourStyles(_ files: [String]) -> [Face] {
            zip(files, [("normal", "normal"), ("normal", "italic"), ("bold", "normal"), ("bold", "italic")]).map { file, style in
                Face(path: "fonts/\(rawValue)/\(file)", format: "woff2", weight: style.0, style: style.1)
            }
        }
        func variable(_ format: String, _ weight: String, _ files: [String]) -> [Face] {
            zip(files, ["normal", "italic"]).map { file, style in
                Face(path: "fonts/\(rawValue)/\(file)", format: format, weight: weight, style: style)
            }
        }
        return switch self {
        case .libron:
            fourStyles(["Libron-Regular.woff2", "Libron-Italic.woff2", "Libron-Bold.woff2", "Libron-BoldItalic.woff2"])
        case .literata:
            variable("woff2", "200 900", ["Literata-Variable.woff2", "Literata-Italic-Variable.woff2"])
        case .atkinsonHyperlegible:
            fourStyles(["AtkinsonHyperlegible-Regular.woff2", "AtkinsonHyperlegible-Italic.woff2",
                        "AtkinsonHyperlegible-Bold.woff2", "AtkinsonHyperlegible-BoldItalic.woff2"])
        case .notoSerif:
            variable("truetype", "100 900", ["NotoSerif-Variable.ttf", "NotoSerif-Italic-Variable.ttf"])
        case .notoSans:
            variable("truetype", "100 900", ["NotoSans-Variable.ttf", "NotoSans-Italic-Variable.ttf"])
        case .openDyslexic:
            fourStyles(["OpenDyslexic-Regular.woff2", "OpenDyslexic-Italic.woff2", "OpenDyslexic-Bold.woff2",
                        "OpenDyslexic-Bold-Italic.woff2"])
        }
    }
}

/// A font other than the book's, as the page declares it: the family to
/// set, what stands in while it loads, and its faces at the reader's own
/// addresses.
struct ReaderFontFace: Equatable, Sendable {
    struct Face: Equatable, Sendable {
        let src: String
        let format: String
        let weight: String
        let style: String
    }

    let family: String
    let fallback: String
    let faces: [Face]
}

/// A font imported from Files, kept on this device.
struct ImportedFont: Codable, Equatable, Identifiable, Sendable {
    enum Format: String, Codable, Sendable {
        case truetype, opentype, woff2

        init?(extension ext: String) {
            switch ext.lowercased() {
            case "ttf": self = .truetype
            case "otf": self = .opentype
            case "woff2": self = .woff2
            default: return nil
            }
        }
    }

    /// Grows with each import, so fonts list in the order they were added.
    let id: Int
    /// The file's name without its extension.
    let name: String
    let format: Format
}

/// A font file the reader refused, with a message for the reader.
struct FontImportError: LocalizedError, Equatable {
    let message: String
    var errorDescription: String? { message }
}

enum ReaderFonts {
    /// The font's name as the settings show it. An imported font that's
    /// gone shows as the book's, which is what the page uses.
    static func name(of font: ReaderFont, imported: [ImportedFont]) -> String {
        switch font {
        case .book:
            String(localized: "Book's font")
        case .bundled(let bundled):
            bundled.name
        case .imported(let id):
            imported.first { $0.id == id }?.name ?? String(localized: "Book's font")
        }
    }

    /// The faces the page declares for a font setting, or nil to keep the
    /// book's own fonts, as when an imported font chosen earlier is gone.
    static func face(for font: ReaderFont, imported: [ImportedFont]) -> ReaderFontFace? {
        switch font {
        case .book:
            return nil
        case .bundled(let bundled):
            return ReaderFontFace(family: bundled.name, fallback: bundled.fallback, faces: bundled.faces.map {
                .init(src: "\(ReaderSchemeHandler.appURL)/\($0.path)", format: $0.format, weight: $0.weight, style: $0.style)
            })
        case .imported(let id):
            guard let match = imported.first(where: { $0.id == id }) else { return nil }
            // Named by id, so a file named like a bundled font can't take
            // its place.
            return ReaderFontFace(family: "Imported Font \(id)", fallback: "serif", faces: [
                .init(src: ImportedFontStore.url(for: match.id), format: match.format.rawValue, weight: "normal", style: "normal"),
            ])
        }
    }

    /// The chosen font for the footnote card, read from its own file: it
    /// isn't installed outside the page. nil keeps the book's.
    @MainActor
    static func noteFont(for font: ReaderFont, store: ImportedFontStore) -> NoteFont? {
        // Imported ids are never reused, so a font read once stays right.
        if let cached = noteFonts[font] { return cached }
        let found = readNoteFont(for: font, store: store)
        if let found { noteFonts[font] = found }
        return found
    }

    @MainActor private static var noteFonts: [ReaderFont: NoteFont] = [:]

    @MainActor
    private static func readNoteFont(for font: ReaderFont, store: ImportedFontStore) -> NoteFont? {
        let data: Data? = switch font {
        case .book:
            nil
        case .bundled(let bundled):
            bundled.faces.first.flatMap { face in
                Bundle.main.resourceURL.flatMap { try? Data(contentsOf: $0.appendingPathComponent(face.path), options: .mappedIfSafe) }
            }
        case .imported(let id):
            store.data(for: id)
        }
        guard let data,
              let descriptors = CTFontManagerCreateFontDescriptorsFromData(data as CFData) as? [CTFontDescriptor],
              // A variable font lists each named instance; the regular one
              // is what the page sets body text in.
              let descriptor = descriptors.first(where: {
                  CTFontDescriptorCopyAttribute($0, kCTFontStyleNameAttribute) as? String == "Regular"
              }) ?? descriptors.first
        else { return nil }
        return .descriptor(descriptor)
    }
}

/// The fonts imported from Files, kept in the app's own storage on this
/// device, never uploaded, for every account on it.
@MainActor
@Observable
final class ImportedFontStore {
    static let shared = ImportedFontStore(folder: URL.applicationSupportDirectory.appendingPathComponent("ReaderFonts", isDirectory: true))

    /// The largest font file it keeps: a font for Chinese, Japanese, or
    /// Korean runs to tens of megabytes.
    nonisolated static let largestFont = 50 * 1024 * 1024

    private(set) var fonts: [ImportedFont] = []
    @ObservationIgnored private let folder: URL
    @ObservationIgnored private var nextID = 1

    private struct Index: Codable {
        var nextID: Int
        var fonts: [ImportedFont]
    }

    init(folder: URL) {
        self.folder = folder
        if let data = try? Data(contentsOf: indexURL), let index = try? JSONDecoder().decode(Index.self, from: data) {
            fonts = index.fonts
            nextID = index.nextID
        }
    }

    /// Where the page fetches an imported font from.
    nonisolated static func url(for id: Int) -> String {
        "\(ReaderSchemeHandler.appURL)/imported-fonts/\(id)"
    }

    private var indexURL: URL { folder.appendingPathComponent("fonts.json") }

    private func fileURL(for id: Int) -> URL {
        folder.appendingPathComponent("\(id).font")
    }

    /// The font's bytes, or nil once it's removed.
    func data(for id: Int) -> Data? {
        guard fonts.contains(where: { $0.id == id }) else { return nil }
        return try? Data(contentsOf: fileURL(for: id), options: .mappedIfSafe)
    }

    /// Keeps a font file picked in Files, after checking it's a font the
    /// reader can use.
    @discardableResult
    func importFont(from url: URL) throws -> ImportedFont {
        let fileName = url.lastPathComponent
        guard let format = ImportedFont.Format(extension: url.pathExtension) else {
            throw FontImportError(message: String(localized: "Choose a TTF, OTF, or WOFF2 font file."))
        }
        // A file from another app's storage is lent for the moment.
        let scoped = url.startAccessingSecurityScopedResource()
        defer { if scoped { url.stopAccessingSecurityScopedResource() } }
        let data: Data
        do {
            data = try Data(contentsOf: url)
        } catch {
            throw FontImportError(message: String(localized: "\(fileName) couldn't be read."))
        }
        // The page and the footnote card each load the whole file.
        guard data.count <= ImportedFontStore.largestFont else {
            throw FontImportError(message: String(localized: "\(fileName) is too large. Choose a font file under 50 MB."))
        }
        let descriptors = CTFontManagerCreateFontDescriptorsFromData(data as CFData) as? [CTFontDescriptor] ?? []
        guard !descriptors.isEmpty else {
            throw FontImportError(message: String(localized: "\(fileName) isn't a font Shisho can read."))
        }
        let font = ImportedFont(id: nextID, name: url.deletingPathExtension().lastPathComponent, format: format)
        do {
            try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
            try data.write(to: fileURL(for: font.id), options: .atomic)
            try save(Index(nextID: nextID + 1, fonts: fonts + [font]))
        } catch {
            try? FileManager.default.removeItem(at: fileURL(for: font.id))
            throw FontImportError(message: String(localized: "\(fileName) couldn't be saved."))
        }
        nextID += 1
        fonts.append(font)
        return font
    }

    func remove(_ id: Int) {
        let remaining = fonts.filter { $0.id != id }
        guard remaining.count != fonts.count else { return }
        // Kept when the list can't be saved, or it would list a font whose
        // file is gone.
        guard (try? save(Index(nextID: nextID, fonts: remaining))) != nil else { return }
        try? FileManager.default.removeItem(at: fileURL(for: id))
        fonts = remaining
    }

    private func save(_ index: Index) throws {
        try JSONEncoder().encode(index).write(to: indexURL, options: .atomic)
    }
}
