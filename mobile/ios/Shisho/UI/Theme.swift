import SwiftUI

/// The web app's design language: its colors (in the asset catalog, with
/// light and dark variants) and its typeface, Geist, scaled with Dynamic
/// Type.
enum Theme {
    static let primary = Color("AccentColor")
    static let card = Color("Card")
    static let border = Color("Border")
    static let mutedForeground = Color("MutedForeground")
    static let destructive = Color("Destructive")

    /// The corner radius of covers and cards, as the web app rounds them.
    static let radius: CGFloat = 8
}

extension Font {
    /// Geist at style's size, scaling with Dynamic Type.
    static func geist(_ style: Font.TextStyle, weight: Font.Weight = .regular) -> Font {
        let size: CGFloat
        switch style {
        case .largeTitle: size = 34
        case .title: size = 28
        case .title2: size = 22
        case .title3: size = 20
        case .headline: size = 17
        case .subheadline: size = 15
        case .callout: size = 16
        case .footnote: size = 13
        case .caption: size = 12
        case .caption2: size = 11
        default: size = 17
        }
        return .custom("Geist", size: size, relativeTo: style).weight(weight)
    }
}
