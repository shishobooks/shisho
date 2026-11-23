import SwiftUI
import UIKit

/// The ebook reader's display settings, with the web reader's list, names,
/// ranges, defaults, and colors (app/libraries/readerSettings.ts). They're
/// kept per installation, so a phone and a tablet each keep their own, and
/// never reach the server.
///
/// The theme follows the app's appearance, Light or Dark, until the user
/// picks one. The reader's bars and sheets always take the app's
/// appearance, whatever the page's theme.
struct ReaderSettings: Codable, Equatable, Sendable {
    enum Theme: String, Codable, CaseIterable, Sendable {
        case light, sepia, dark, black
    }

    /// The theme the user picked, or System to follow the app.
    enum ThemeChoice: String, Codable, CaseIterable, Sendable {
        case system, light, sepia, dark, black

        var theme: Theme? { Theme(rawValue: rawValue) }
    }

    enum Layout: String, Codable, CaseIterable, Sendable {
        case paginated, scrolled
    }

    /// How many pages a paginated book shows at once. Auto shows two side
    /// by side when the reader is wider than tall and has room for both;
    /// One never does.
    enum Pages: String, Codable, CaseIterable, Sendable {
        case auto, one
    }

    /// The settings set by number, each with its range and step.
    enum Numeric: CaseIterable, Sendable {
        /// The text's base size in CSS pixels; the book's own sizes scale
        /// from it.
        case size
        /// A multiple of the font size.
        case lineSpacing
        /// The space on each side of the text, as a percent of the page's
        /// width. At least 1: foliate leaves the same space between the two
        /// pages of a spread, which would otherwise run together.
        case margins

        var range: ClosedRange<Double> {
            switch self {
            case .size: 10...40
            case .lineSpacing: 1...2.5
            case .margins: 1...15
            }
        }

        var step: Double {
            switch self {
            case .size: 0.5
            case .lineSpacing: 0.05
            case .margins: 1
            }
        }

        /// The nearest step in range, without the float noise of 0.05 steps
        /// (1.4500000001).
        func snapped(_ value: Double) -> Double {
            let clamped = min(range.upperBound, max(range.lowerBound, value))
            return ((clamped / step).rounded() * step * 100).rounded() / 100
        }

        /// One step up (1) or down (-1), held to the range.
        func stepped(_ value: Double, by direction: Int) -> Double {
            snapped(value + Double(direction) * step)
        }

        /// The value as the web's panel shows it.
        func text(_ value: Double) -> String {
            switch self {
            case .size: "\(value.formatted(.number.precision(.fractionLength(0...1)))) px"
            case .lineSpacing: value.formatted(.number.precision(.fractionLength(2)))
            case .margins: "\(Int(value.rounded()))%"
            }
        }

        var defaultValue: Double {
            switch self {
            case .size: 16
            case .lineSpacing: 1.45
            case .margins: 7
            }
        }

        fileprivate var key: CodingKeys {
            switch self {
            case .size: .size
            case .lineSpacing: .lineSpacing
            case .margins: .margins
            }
        }
    }

    var font = ReaderFont.book
    var size = Numeric.size.defaultValue
    /// Sets body text at size even where the book sets its own sizes.
    var forceFontSize = false
    var lineSpacing = Numeric.lineSpacing.defaultValue
    var margins = Numeric.margins.defaultValue
    var justify = true
    var themeChoice = ThemeChoice.system
    var layout = Layout.paginated
    var pages = Pages.auto
    /// The app is in Dark Mode. Not a setting: the reader sets it from the
    /// screen, and it isn't kept.
    var appDark = false

    /// The theme the page is drawn in.
    var theme: Theme {
        themeChoice.theme ?? (appDark ? .dark : .light)
    }

    subscript(numeric: Numeric) -> Double {
        get {
            switch numeric {
            case .size: size
            case .lineSpacing: lineSpacing
            case .margins: margins
            }
        }
        set {
            switch numeric {
            case .size: size = newValue
            case .lineSpacing: lineSpacing = newValue
            case .margins: margins = newValue
            }
        }
    }

    private static let key = "reader.settings"

    fileprivate enum CodingKeys: String, CodingKey {
        case font, size, forceFontSize, lineSpacing, margins, justify, themeChoice, layout, pages
    }

    init() {}

    /// Each setting on its own, so one a kept copy lacks, as from an earlier
    /// build, or holds a value this build can't use, takes its default
    /// without losing the others.
    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        func value<T: Decodable>(_ key: CodingKeys) -> T? {
            (try? container.decodeIfPresent(T.self, forKey: key)) ?? nil
        }
        font = value(.font) ?? .book
        for numeric in Numeric.allCases {
            // Out of range falls back rather than clamping, as the web's
            // does: a value from nowhere isn't a reader's choice.
            if let stored: Double = value(numeric.key), stored.isFinite, numeric.range.contains(stored) {
                self[numeric] = numeric.snapped(stored)
            }
        }
        forceFontSize = value(.forceFontSize) ?? false
        justify = value(.justify) ?? true
        themeChoice = value(.themeChoice) ?? .system
        layout = value(.layout) ?? .paginated
        pages = value(.pages) ?? .auto
    }

    static func load(_ defaults: UserDefaults = .standard) -> ReaderSettings {
        guard let data = defaults.data(forKey: key),
              let settings = try? JSONDecoder().decode(ReaderSettings.self, from: data)
        else { return ReaderSettings() }
        return settings
    }

    func save(_ defaults: UserDefaults = .standard) {
        defaults.set(try? JSONEncoder().encode(self), forKey: ReaderSettings.key)
    }

    /// What the page's script styles the book with, as JSON: every setting,
    /// the theme's colors, and the chosen font's faces, or null for the
    /// book's own fonts.
    func script(font face: ReaderFontFace?) -> String {
        let colors = theme.colors
        var object: [String: Any] = [
            "theme": theme.rawValue, "layout": layout.rawValue, "pages": pages.rawValue,
            "fg": colors.fg, "bg": colors.bg, "link": colors.link ?? NSNull(),
            "size": size, "forceFontSize": forceFontSize, "lineSpacing": lineSpacing,
            "margins": margins, "justify": justify, "font": NSNull(),
        ]
        if let face {
            object["font"] = [
                "family": face.family, "fallback": face.fallback,
                "faces": face.faces.map { ["src": $0.src, "format": $0.format, "weight": $0.weight, "style": $0.style] },
            ]
        }
        return (try? JSONSerialization.data(withJSONObject: object, options: .sortedKeys))
            .flatMap { String(data: $0, encoding: .utf8) } ?? "{}"
    }
}

extension ReaderSettings {
    /// What the page styles the book with, with the chosen font's faces as
    /// they are on this device.
    @MainActor
    var pageScript: String {
        script(font: ReaderFonts.face(for: font, imported: ImportedFontStore.shared.fonts))
    }
}

extension ReaderSettings.Theme {
    /// The web reader's colors. link replaces the book's link colors (Light
    /// keeps them); noteLink is a link in a footnote.
    var colors: (fg: String, bg: String, link: String?, noteLink: String) {
        switch self {
        case .light: ("#111111", "#ffffff", nil, "#0645ad")
        case .sepia: ("#5b4636", "#f4ecd8", "#8a4b08", "#8a4b08")
        case .dark: ("#e8e8e8", "#1a1a1a", "#8ab4f8", "#8ab4f8")
        case .black: ("#c8c8c8", "#000000", "#8ab4f8", "#8ab4f8")
        }
    }

    var background: UIColor { UIColor(hex: colors.bg) }
    var foreground: UIColor { UIColor(hex: colors.fg) }
}

extension ReaderSettings.ThemeChoice {
    var label: LocalizedStringResource {
        switch self {
        case .system: "System"
        case .light: "Light"
        case .sepia: "Sepia"
        case .dark: "Dark"
        case .black: "Black"
        }
    }
}

extension ReaderSettings.Layout {
    var label: LocalizedStringResource {
        switch self {
        case .paginated: "Paginated"
        case .scrolled: "Scrolled"
        }
    }
}

extension ReaderSettings.Pages {
    var label: LocalizedStringResource {
        switch self {
        case .auto: "Auto"
        case .one: "One"
        }
    }
}

extension ReaderSettings.Numeric {
    var label: LocalizedStringResource {
        switch self {
        case .size: "Font size"
        case .lineSpacing: "Line spacing"
        case .margins: "Margins"
        }
    }

    /// Words for the slider's two ends, which say what the number means.
    var ends: (LocalizedStringResource, LocalizedStringResource)? {
        switch self {
        case .size: nil
        case .lineSpacing: ("Compact", "Relaxed")
        case .margins: ("Narrow", "Wide")
        }
    }

    var decreaseLabel: LocalizedStringResource {
        switch self {
        case .size: "Decrease font size"
        case .lineSpacing: "Decrease line spacing"
        case .margins: "Decrease margins"
        }
    }

    var increaseLabel: LocalizedStringResource {
        switch self {
        case .size: "Increase font size"
        case .lineSpacing: "Increase line spacing"
        case .margins: "Increase margins"
        }
    }

    /// The setting's name in its identifiers, the same on every platform.
    var identifier: String {
        switch self {
        case .size: "size"
        case .lineSpacing: "lineSpacing"
        case .margins: "margins"
        }
    }
}

extension UIColor {
    /// A color from "#rrggbb".
    convenience init(hex: String) {
        var value: UInt64 = 0
        Scanner(string: hex.replacingOccurrences(of: "#", with: "")).scanHexInt64(&value)
        self.init(red: CGFloat((value >> 16) & 0xff) / 255, green: CGFloat((value >> 8) & 0xff) / 255,
                  blue: CGFloat(value & 0xff) / 255, alpha: 1)
    }
}
