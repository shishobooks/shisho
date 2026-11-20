import SwiftUI
import UIKit

/// The web app's design language (`app/index.css`): its colors, in the asset
/// catalog with light and dark variants, its typeface, Geist, scaled with
/// Dynamic Type, and its shapes. Navigation stays native; every visual comes
/// from here.
enum Theme {
    static let primary = Color("AccentColor")
    static let primaryForeground = Color("PrimaryForeground")
    static let background = Color("Background")
    static let foreground = Color("Foreground")
    static let card = Color("Card")
    static let secondary = Color("Secondary")
    static let muted = Color("Muted")
    static let mutedForeground = Color("MutedForeground")
    static let border = Color("Border")
    static let input = Color("Input")
    static let destructive = Color("Destructive")

    /// Covers and inputs, as the web app rounds them.
    static let radius: CGFloat = 8
    /// Cards and sheets.
    static let cardRadius: CGFloat = 14
}

extension Font {
    /// Geist at style's size, scaling with Dynamic Type.
    static func geist(_ style: Font.TextStyle, weight: Font.Weight = .regular) -> Font {
        .custom("Geist", size: geistSize(style), relativeTo: style).weight(weight)
    }

    /// Geist at an exact size, scaling with Dynamic Type relative to style.
    static func geist(size: CGFloat, relativeTo style: Font.TextStyle, weight: Font.Weight = .regular) -> Font {
        .custom("Geist", size: size, relativeTo: style).weight(weight)
    }

    private static func geistSize(_ style: Font.TextStyle) -> CGFloat {
        switch style {
        case .largeTitle: 34
        case .title: 28
        case .title2: 22
        case .title3: 20
        case .headline: 17
        case .body: 17
        case .callout: 16
        case .subheadline: 15
        case .footnote: 13
        case .caption: 12
        case .caption2: 11
        default: 17
        }
    }
}

// MARK: - Brand

/// "SHISHO 司書" beside the shelf mark, as the web app's logo draws it.
struct Wordmark: View {
    var size: CGFloat = 28
    /// Off where the app's icon already stands beside it.
    var showsMark = true

    var body: some View {
        HStack(alignment: .center, spacing: size * 0.22) {
            if showsMark {
                Image("LogoMark")
                    .resizable()
                    .renderingMode(.template)
                    .foregroundStyle(Theme.primary)
                    .frame(width: size * 1.05, height: size * 1.05)
            }
            // 司書 sits raised beside the name, as the web app's superscript does.
            Text(verbatim: "SHISHO")
                .font(.custom("ShishoWordmark-Bold", fixedSize: size))
                .tracking(size * 0.05)
                .foregroundStyle(Theme.foreground)
                + Text(verbatim: "司書")
                .font(.system(size: size * 0.5))
                .foregroundStyle(Theme.primary)
                .baselineOffset(size * 0.36)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Shisho")
    }
}

// MARK: - Small pieces

/// A File Type, as the web app's secondary badge draws it: a rounded
/// rectangle, not a pill.
struct FileTypeChip: View {
    let fileType: String
    var outlined = false
    private let shape = RoundedRectangle(cornerRadius: 5, style: .continuous)

    var body: some View {
        Text(FileTypes.label(fileType))
            .font(.geist(.caption, weight: .medium))
            .padding(.horizontal, 7)
            .padding(.vertical, 2)
            .background(outlined ? Color.clear : Theme.secondary, in: shape)
            .overlay {
                if outlined { shape.strokeBorder(Theme.border) }
            }
            .foregroundStyle(Theme.foreground)
    }
}

/// A Series Number in the primary color's square, as the web app badges it.
struct SeriesNumberBadge: View {
    let number: Double
    @ScaledMetric(relativeTo: .caption) private var height: CGFloat = 18

    var body: some View {
        Text(SeriesNumbers.format(number))
            .font(.geist(.caption, weight: .heavy))
            .monospacedDigit()
            .padding(.horizontal, 5)
            .frame(minWidth: height, minHeight: height)
            .background(Theme.primary, in: RoundedRectangle(cornerRadius: 4, style: .continuous))
            .foregroundStyle(Theme.primaryForeground)
            .accessibilityLabel("Number \(SeriesNumbers.format(number))")
    }
}

extension View {
    /// The web app's card: its background, a hairline border, and a soft
    /// shadow.
    func cardStyle(radius: CGFloat = Theme.cardRadius) -> some View {
        background(Theme.card, in: RoundedRectangle(cornerRadius: radius, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: radius, style: .continuous).strokeBorder(Theme.border))
            .shadow(color: .black.opacity(0.06), radius: 3, y: 1)
    }
}

/// The primary button: full width, the primary color, and a little give
/// when pressed.
struct PrimaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.geist(.headline, weight: .semibold))
            .frame(maxWidth: .infinity, minHeight: 50)
            .background(Theme.primary.opacity(isEnabled ? 1 : 0.45), in: RoundedRectangle(cornerRadius: 12, style: .continuous))
            .foregroundStyle(Theme.primaryForeground)
            .scaleEffect(configuration.isPressed ? 0.98 : 1)
            .animation(.spring(response: 0.25, dampingFraction: 0.7), value: configuration.isPressed)
    }
}

/// The outlined button, for secondary actions.
struct OutlineButtonStyle: ButtonStyle {
    var role: ButtonRole?

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.geist(.headline, weight: .medium))
            .frame(maxWidth: .infinity, minHeight: 48)
            .background(Theme.card.opacity(configuration.isPressed ? 0.6 : 1), in: RoundedRectangle(cornerRadius: 12, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).strokeBorder(Theme.border))
            .foregroundStyle(role == .destructive ? Theme.destructive : Theme.foreground)
            .scaleEffect(configuration.isPressed ? 0.98 : 1)
            .animation(.spring(response: 0.25, dampingFraction: 0.7), value: configuration.isPressed)
    }
}

/// A labeled text field in the web app's input style, with an icon.
struct LabeledInput<Field: View>: View {
    let label: LocalizedStringKey
    let systemImage: String
    var focused = false
    @ViewBuilder let field: () -> Field

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(label)
                .font(.geist(.subheadline, weight: .medium))
                .foregroundStyle(Theme.foreground)
            HStack(spacing: 10) {
                Image(systemName: systemImage)
                    .foregroundStyle(focused ? Theme.primary : Theme.mutedForeground)
                    .frame(width: 20)
                    .accessibilityHidden(true)
                field()
                    .font(.geist(.body))
            }
            .padding(.horizontal, 14)
            .frame(minHeight: 50)
            .background(Theme.card, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .strokeBorder(focused ? Theme.primary : Theme.input, lineWidth: focused ? 1.5 : 1)
            )
            .animation(.easeOut(duration: 0.15), value: focused)
        }
    }
}

/// A notice above content: an icon, text, and an optional action.
struct NoticeCard<Action: View>: View {
    let text: String
    let systemImage: String
    var tint: Color = Theme.mutedForeground
    @ViewBuilder var action: () -> Action

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: systemImage)
                .font(.geist(.body, weight: .semibold))
                .foregroundStyle(tint)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 8) {
                Text(text)
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.foreground)
                    .fixedSize(horizontal: false, vertical: true)
                action()
            }
            Spacer(minLength: 0)
        }
        .padding(14)
        .cardStyle()
        .accessibilityElement(children: .combine)
    }
}

/// Opens the app's page in Settings, where its permissions are.
struct SettingsLink: View {
    var body: some View {
        Link(destination: URL(string: UIApplication.openSettingsURLString)!) {
            Label("Open Settings", systemImage: "arrow.up.forward")
                .font(.geist(.subheadline, weight: .semibold))
        }
        .accessibilityIdentifier("settings.link")
    }
}

extension NoticeCard where Action == EmptyView {
    init(text: String, systemImage: String, tint: Color = Theme.mutedForeground) {
        self.init(text: text, systemImage: systemImage, tint: tint) { EmptyView() }
    }
}

// MARK: - Formatting

enum FileTypes {
    /// File Types this version knows; any other is shown as not supported
    /// (ADR 0021).
    static let known: [String: String] = [
        "epub": "EPUB", "kepub": "KePub", "cbz": "CBZ", "pdf": "PDF", "m4b": "M4B", "mobi": "MOBI", "azw3": "AZW3",
    ]

    static func label(_ fileType: String) -> String {
        known[fileType] ?? fileType.uppercased()
    }

    static func isKnown(_ fileType: String) -> Bool {
        known[fileType] != nil
    }
}

enum SeriesNumbers {
    static func format(_ number: Double) -> String {
        number.formatted(.number.precision(.fractionLength(0...2)))
    }
}
