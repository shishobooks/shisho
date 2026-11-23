import SwiftUI
import UIKit

/// The ebook reader's display settings, with the web reader's names and
/// colors (app/libraries/readerSettings.ts). They're kept per installation,
/// so a phone and a tablet each keep their own, and never reach the server.
struct ReaderSettings: Codable, Equatable, Sendable {
    enum Theme: String, Codable, CaseIterable, Sendable {
        case light, sepia, dark, black
    }

    enum Layout: String, Codable, CaseIterable, Sendable {
        case paginated, scrolled
    }

    var theme = Theme.light
    var layout = Layout.paginated

    private static let key = "reader.settings"

    static func load(_ defaults: UserDefaults = .standard) -> ReaderSettings {
        guard let data = defaults.data(forKey: key),
              let settings = try? JSONDecoder().decode(ReaderSettings.self, from: data)
        else { return ReaderSettings() }
        return settings
    }

    func save(_ defaults: UserDefaults = .standard) {
        defaults.set(try? JSONEncoder().encode(self), forKey: ReaderSettings.key)
    }

    /// What the page's script styles the book with, as JSON.
    var script: String {
        let colors = theme.colors
        let object: [String: Any] = [
            "theme": theme.rawValue, "layout": layout.rawValue,
            "fg": colors.fg, "bg": colors.bg, "link": colors.link ?? NSNull(),
        ]
        return (try? JSONSerialization.data(withJSONObject: object)).flatMap { String(data: $0, encoding: .utf8) } ?? "{}"
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

    var isDark: Bool { self == .dark || self == .black }

    var background: UIColor { UIColor(hex: colors.bg) }
    var foreground: UIColor { UIColor(hex: colors.fg) }

    var label: LocalizedStringResource {
        switch self {
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

extension UIColor {
    /// A color from "#rrggbb".
    convenience init(hex: String) {
        var value: UInt64 = 0
        Scanner(string: hex.replacingOccurrences(of: "#", with: "")).scanHexInt64(&value)
        self.init(red: CGFloat((value >> 16) & 0xff) / 255, green: CGFloat((value >> 8) & 0xff) / 255,
                  blue: CGFloat(value & 0xff) / 255, alpha: 1)
    }
}
