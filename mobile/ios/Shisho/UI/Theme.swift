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
    /// The web app's Rating stars, Tailwind's amber-400, the same in light
    /// and dark.
    static let rating = Color(red: 251 / 255, green: 191 / 255, blue: 36 / 255)
    /// Tailwind's amber-500, which the web's Rating filter marks with.
    static let ratingStrong = Color(red: 245 / 255, green: 158 / 255, blue: 11 / 255)
    /// The web's chart colors (`--chart-1`, `--chart-2`, `--chart-5`), which
    /// its filter sections and chips are marked with.
    static let chart1 = Color("Chart1")
    static let chart2 = Color("Chart2")
    static let chart5 = Color("Chart5")

    /// Covers and inputs, as the web app rounds them.
    static let radius: CGFloat = 8
    /// Cards and sheets.
    static let cardRadius: CGFloat = 14

    /// The bundled fonts, listed under `UIAppFonts` in `project.yml`. A name
    /// iOS doesn't know falls back to the system font without a word, so
    /// `BundledFontsTests` checks each one.
    enum Fonts {
        static let geist = "Geist"
        static let geistMono = "Geist Mono"
        /// Cuts of the web logo's Noto Sans Bold and Noto Sans JP holding
        /// only "SHISHO" and "司書".
        static let wordmark = "ShishoWordmark-Bold"
        static let wordmarkJP = "ShishoWordmarkJP-Regular"
    }
}

extension Theme {
    /// Geist for what SwiftUI's font doesn't reach: navigation bar titles,
    /// bar buttons such as Cancel and Done, and the search field. System
    /// menus, alerts, and the keyboard stay as iOS draws them.
    @MainActor static func applyUIKitFonts() {
        func geist(_ size: CGFloat, _ weight: UIFont.Weight, _ style: UIFont.TextStyle) -> UIFont {
            let descriptor = UIFontDescriptor(fontAttributes: [
                .family: Theme.Fonts.geist,
                .traits: [UIFontDescriptor.TraitKey.weight: weight],
            ])
            return UIFontMetrics(forTextStyle: style).scaledFont(for: UIFont(descriptor: descriptor, size: size))
        }
        let bar = UINavigationBar.appearance()
        bar.titleTextAttributes = [.font: geist(17, .semibold, .headline)]
        bar.largeTitleTextAttributes = [.font: geist(34, .bold, .largeTitle)]
        for state in [UIControl.State.normal, .highlighted, .disabled] {
            UIBarButtonItem.appearance().setTitleTextAttributes([.font: geist(17, .regular, .body)], for: state)
        }
        UISearchTextField.appearance().font = geist(17, .regular, .body)
    }
}

extension View {
    /// Takes taps a little past its edges, so a control drawn smaller than
    /// Apple's 44 points still has a 44-point target.
    func hitArea(height: CGFloat) -> some View {
        contentShape(Rectangle().inset(by: -max(0, (44 - height) / 2)))
    }
}

extension Font {
    /// Geist at style's size, scaling with Dynamic Type.
    static func geist(_ style: Font.TextStyle, weight: Font.Weight = .regular) -> Font {
        .custom(Theme.Fonts.geist, size: textStyleSize(style), relativeTo: style).weight(weight)
    }

    /// Geist at an exact size, scaling with Dynamic Type relative to style.
    static func geist(size: CGFloat, relativeTo style: Font.TextStyle, weight: Font.Weight = .regular) -> Font {
        .custom(Theme.Fonts.geist, size: size, relativeTo: style).weight(weight)
    }

    /// Geist Mono, the web app's monospace, at style's size, scaling with
    /// Dynamic Type. For text read character by character, like ids and
    /// diagnostics; `.monospacedDigit()` on Geist suits numbers in prose.
    static func geistMono(_ style: Font.TextStyle, weight: Font.Weight = .regular) -> Font {
        .custom(Theme.Fonts.geistMono, size: textStyleSize(style), relativeTo: style).weight(weight)
    }

    /// A text style's size at the default text size, which Dynamic Type
    /// scales from.
    static func textStyleSize(_ style: Font.TextStyle) -> CGFloat {
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
                // The shelf in the primary color, with the web logo's spine
                // highlights, title bands, and shading over it.
                ZStack {
                    Image("LogoMark")
                        .resizable()
                        .renderingMode(.template)
                        .foregroundStyle(Theme.primary)
                    Image("LogoMarkDetails")
                        .resizable()
                }
                .frame(width: size * 1.05, height: size * 1.05)
            }
            // 司書 sits raised beside the name, as the web app's superscript does.
            Text(verbatim: "SHISHO")
                .font(.custom(Theme.Fonts.wordmark, fixedSize: size))
                .tracking(size * 0.05)
                .foregroundStyle(Theme.foreground)
                + Text(verbatim: "司書")
                .font(.custom(Theme.Fonts.wordmarkJP, fixedSize: size * 0.5))
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

/// A Series Number in the primary color's square, as the web app badges it:
/// "3", or a range such as "1-6".
struct SeriesNumberBadge: View {
    let number: Double
    var end: Double?
    @ScaledMetric(relativeTo: .caption) private var height: CGFloat = 18

    private var label: String {
        guard let end else { return SeriesNumbers.format(number) }
        return "\(SeriesNumbers.format(number))-\(SeriesNumbers.format(end))"
    }

    var body: some View {
        Text(label)
            .font(.geist(.caption, weight: .heavy))
            .monospacedDigit()
            .padding(.horizontal, 5)
            .frame(minWidth: height, minHeight: height)
            .background(Theme.primary, in: RoundedRectangle(cornerRadius: 4, style: .continuous))
            .foregroundStyle(Theme.primaryForeground)
            .accessibilityLabel(end.map { "Numbers \(SeriesNumbers.format(number)) to \(SeriesNumbers.format($0))" }
                ?? "Number \(SeriesNumbers.format(number))")
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
    let icon: Lucide
    var focused = false
    @ViewBuilder let field: () -> Field

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(label)
                .font(.geist(.subheadline, weight: .medium))
                .foregroundStyle(Theme.foreground)
            HStack(spacing: 10) {
                Icon(icon, size: 18)
                    .foregroundStyle(focused ? Theme.primary : Theme.mutedForeground)
                    .frame(width: 20)
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
    let icon: Lucide
    var tint: Color = Theme.mutedForeground
    @ViewBuilder var action: () -> Action

    var body: some View {
        let card = HStack(alignment: .top, spacing: 12) {
            Icon(icon, size: 19)
                .foregroundStyle(tint)
                .padding(.top, 1)
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
        // Combined, an action would be a button inside a button, which
        // VoiceOver and the harness can't press reliably, so a card with one
        // keeps its text and its action apart. It goes by the action's type,
        // so one built with an `if` stays apart even when nothing shows,
        // which VoiceOver reads the same way.
        if Action.self == EmptyView.self {
            card.accessibilityElement(children: .combine)
        } else {
            card.accessibilityElement(children: .contain)
        }
    }
}

/// Opens where the app is updated, for a server too new for this version.
struct UpdateAppLink: View {
    var body: some View {
        Link(destination: Installation.updateURL) {
            Label("Update the app", icon: .arrowUpRight)
                .font(.geist(.subheadline, weight: .semibold))
        }
        .accessibilityIdentifier("update.link")
    }
}

/// Opens the app's page in Settings, where its permissions are.
struct SettingsLink: View {
    var body: some View {
        Link(destination: URL(string: UIApplication.openSettingsURLString)!) {
            Label("Open Settings", icon: .arrowUpRight)
                .font(.geist(.subheadline, weight: .semibold))
        }
        .accessibilityIdentifier("settings.link")
    }
}

extension NoticeCard where Action == EmptyView {
    init(text: String, icon: Lucide, tint: Color = Theme.mutedForeground) {
        self.init(text: text, icon: icon, tint: tint) { EmptyView() }
    }
}

// MARK: - Formatting

enum FileTypes {
    /// The order the web's filters list them in (`FILE_TYPE_OPTIONS`).
    static let order = ["epub", "kepub", "azw3", "mobi", "m4b", "cbz", "pdf"]

    /// Sorted as the web lists them, any it doesn't know last.
    static func sorted(_ types: some Sequence<String>) -> [String] {
        types.sorted { (order.firstIndex(of: $0) ?? order.count, $0) < (order.firstIndex(of: $1) ?? order.count, $1) }
    }

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

