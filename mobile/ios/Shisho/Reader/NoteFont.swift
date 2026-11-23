import SwiftUI
import UIKit

/// The font a footnote's popover is set in: the one the book sets around the
/// note reference, as the page draws it, as the web reader's popover takes
/// the book's font. A font the book carries itself isn't installed outside
/// the page, so the next family in the list stands in, and a generic one is
/// the font WebKit draws for it.
enum NoteFont: Equatable {
    case family(String)
    case system

    /// WebKit's font on iOS for each generic family, and for a page that
    /// sets none.
    private static let standard = "Times New Roman"
    private static let generics: [String: NoteFont] = [
        "serif": .family(standard),
        "-webkit-standard": .family(standard),
        "sans-serif": .family("Helvetica"),
        "monospace": .family("Courier"),
        "-webkit-monospace": .family("Courier"),
        "system-ui": .system,
        "-apple-system": .system,
    ]

    /// css is a computed font-family list, as getComputedStyle gives it.
    static func resolve(_ css: String, installed: Set<String>) -> NoteFont {
        let byLowercase = Dictionary(installed.map { ($0.lowercased(), $0) }, uniquingKeysWith: { first, _ in first })
        for name in css.split(separator: ",") {
            let family = name.trimmingCharacters(in: .whitespaces).trimmingCharacters(in: CharacterSet(charactersIn: "\"'"))
            let key = family.lowercased()
            if let generic = generics[key] { return generic }
            if let installedName = byLowercase[key] { return .family(installedName) }
        }
        return .family(standard)
    }

    static func resolve(_ css: String) -> NoteFont {
        resolve(css, installed: installedFamilies)
    }

    private static let installedFamilies = Set(UIFont.familyNames)

    /// The space to add between lines so they're lineHeight times the text's
    /// size apart, as the page sets them, at style's size.
    func lineSpacing(for lineHeight: Double?, relativeTo style: UIFont.TextStyle, base: CGFloat) -> CGFloat {
        guard let lineHeight else { return 0 }
        let size = UIFontMetrics(forTextStyle: style).scaledValue(for: base)
        let font: UIFont = switch self {
        case .family(let name):
            UIFont(descriptor: UIFontDescriptor(fontAttributes: [.family: name]), size: size)
        case .system:
            .systemFont(ofSize: size)
        }
        return max(0, size * lineHeight - font.lineHeight)
    }

    /// The font at style's size, following Dynamic Type.
    func font(relativeTo style: Font.TextStyle) -> Font {
        switch self {
        case .family(let name): .custom(name, size: Font.textStyleSize(style), relativeTo: style)
        case .system: .system(style)
        }
    }
}
