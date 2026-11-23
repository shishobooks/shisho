import CoreText
import SwiftUI
import UIKit

/// The font a footnote's card is set in: the one the book sets around the
/// note reference, as the page draws it, as the web reader's popover takes
/// the book's font. A font the book carries itself isn't installed outside
/// the page, so the next family in the list stands in, and a generic one is
/// the font WebKit draws for it. A font chosen in the reader's settings is
/// read from its own file instead.
enum NoteFont: Equatable {
    case family(String)
    case system
    /// A font that isn't installed, read from its file.
    case descriptor(CTFontDescriptor)

    /// The font's family, for tests and logs.
    var familyName: String? {
        switch self {
        case .family(let name): name
        case .system: nil
        case .descriptor(let descriptor): CTFontDescriptorCopyAttribute(descriptor, kCTFontFamilyNameAttribute) as? String
        }
    }

    /// WebKit's font on iOS for each generic family, and for a page that
    /// sets none.
    private static let standard = "Times New Roman"
    private static func generic(_ family: String) -> NoteFont? {
        switch family {
        case "serif", "-webkit-standard": .family(standard)
        case "sans-serif": .family("Helvetica")
        case "monospace", "-webkit-monospace": .family("Courier")
        case "system-ui", "-apple-system": .system
        default: nil
        }
    }

    /// css is a computed font-family list, as getComputedStyle gives it.
    static func resolve(_ css: String, installed: Set<String>) -> NoteFont {
        let byLowercase = Dictionary(installed.map { ($0.lowercased(), $0) }, uniquingKeysWith: { first, _ in first })
        for name in css.split(separator: ",") {
            let family = name.trimmingCharacters(in: .whitespaces).trimmingCharacters(in: CharacterSet(charactersIn: "\"'"))
            let key = family.lowercased()
            if let generic = generic(key) { return generic }
            if let installedName = byLowercase[key] { return .family(installedName) }
        }
        return .family(standard)
    }

    static func resolve(_ css: String) -> NoteFont {
        resolve(css, installed: installedFamilies)
    }

    private static let installedFamilies = Set(UIFont.familyNames)

    private func uiFont(size: CGFloat) -> UIFont {
        switch self {
        case .family(let name):
            UIFont(descriptor: UIFontDescriptor(fontAttributes: [.family: name]), size: size)
        case .system:
            .systemFont(ofSize: size)
        case .descriptor(let descriptor):
            CTFontCreateWithFontDescriptor(descriptor, size, nil) as UIFont
        }
    }

    /// The space to add between lines so they're lineHeight times the text's
    /// size apart, as the page sets them, at size.
    func lineSpacing(for lineHeight: Double?, size: CGFloat) -> CGFloat {
        guard let lineHeight else { return 0 }
        return max(0, size * lineHeight - uiFont(size: size).lineHeight)
    }

    /// The font at size, which is the reader's own text size rather than
    /// Dynamic Type's: the note is part of the book.
    func font(size: CGFloat) -> Font {
        switch self {
        case .family(let name): .custom(name, fixedSize: size)
        case .system: .system(size: size)
        case .descriptor(let descriptor): Font(CTFontCreateWithFontDescriptor(descriptor, size, nil))
        }
    }

    /// A note's text size: 90% of the reader's font size, to the half point,
    /// as the web reader's popover sets it (footnoteStyle in
    /// app/libraries/readerSettings.ts).
    static func size(forReaderSize size: Double) -> CGFloat {
        CGFloat((size * 0.9 * 2).rounded() / 2)
    }
}
