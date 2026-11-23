import SwiftUI

/// The web app's icons, Lucide, drawn the same way: a 24 point grid with a
/// stroke of 2 that scales with the icon. `mobile/ios/scripts/lucide-icons.mjs`
/// writes them into the asset catalog from the web app's lucide-react; add a
/// name there, run `mise ios:icons`, then add a case here. A filled variant
/// ends in "-fill".
enum Lucide: String, CaseIterable {
    case arrowDown = "arrow-down"
    case arrowDownUp = "arrow-down-up"
    case arrowUp = "arrow-up"
    case arrowUpRight = "arrow-up-right"
    case book
    case bookOpen = "book-open"
    case bookOpenCheck = "book-open-check"
    case bookmark
    case bookmarkPlus = "bookmark-plus"
    case check
    case chevronDown = "chevron-down"
    case chevronRight = "chevron-right"
    case chevronUp = "chevron-up"
    case chevronsUpDown = "chevrons-up-down"
    case circleAlert = "circle-alert"
    case clock
    case clockArrowUp = "clock-arrow-up"
    case ellipsis
    case file
    case fileSearch = "file-search"
    case fileX = "file-x"
    case headphones
    case languages
    case layers
    case library
    case list
    case listFilter = "list-filter"
    case listMusic = "list-music"
    case listX = "list-x"
    case lock
    case logOut = "log-out"
    case messageCircleWarning = "message-circle-warning"
    case minus
    case moon
    case moonFill = "moon-fill"
    case pause
    case pauseFill = "pause-fill"
    case pencil
    case play
    case playFill = "play-fill"
    case plus
    case refresh = "refresh-cw"
    case rotateCcw = "rotate-ccw"
    case rotateCw = "rotate-cw"
    case search
    case server
    case share = "share-2"
    case skipBack = "skip-back"
    case skipForward = "skip-forward"
    case square
    case squareCheck = "square-check-big"
    case star
    case starFill = "star-fill"
    case starHalfFill = "star-half-fill"
    case tags
    case trash = "trash-2"
    case triangleAlert = "triangle-alert"
    case user
    case userX = "user-x"
    case users
    case wifiOff = "wifi-off"
    case x

    /// The icon as an image, for a Label, a menu item, or a swipe action.
    var image: Image {
        Image("Lucide/\(rawValue)").renderingMode(.template)
    }
}

/// A Lucide icon, size points across at the default text size, growing with
/// Dynamic Type as text of style does, in the foreground style. It's
/// decorative: the control it's in carries the label.
struct Icon: View {
    let name: Lucide
    @ScaledMetric private var size: CGFloat

    init(_ name: Lucide, size: CGFloat = 20, relativeTo style: Font.TextStyle = .body) {
        self.name = name
        _size = ScaledMetric(wrappedValue: size, relativeTo: style)
    }

    var body: some View {
        name.image
            .resizable()
            .frame(width: size, height: size)
            .accessibilityHidden(true)
    }
}

/// `Icon`, by a name a `Label` extension can use: there, `Icon` is the
/// label's own icon type.
typealias LucideIcon = Icon

extension Label where Title == Text, Icon == LucideIcon {
    /// A label with a Lucide icon sized beside its text, as buttons and rows
    /// show it: size points at the default text size, growing as text of
    /// style does.
    @MainActor
    init(_ title: LocalizedStringKey, icon: Lucide, size: CGFloat = 16, relativeTo style: Font.TextStyle = .subheadline) {
        self.init { Text(title) } icon: { LucideIcon(icon, size: size, relativeTo: style) }
    }

    /// The same, with a title that's already localized.
    @MainActor
    init(verbatim title: String, icon: Lucide, size: CGFloat = 16, relativeTo style: Font.TextStyle = .subheadline) {
        self.init { Text(verbatim: title) } icon: { LucideIcon(icon, size: size, relativeTo: style) }
    }
}

extension Label where Title == Text, Icon == Image {
    /// A label with a Lucide icon for what iOS draws itself, such as menus,
    /// swipe actions, and the large content viewer, which size the image.
    init(_ title: LocalizedStringKey, menuIcon icon: Lucide) {
        self.init { Text(title) } icon: { icon.image }
    }

    init(verbatim title: String, menuIcon icon: Lucide) {
        self.init { Text(verbatim: title) } icon: { icon.image }
    }
}

/// "Nothing here" with a Lucide icon, as the system's empty and unavailable
/// screens show.
struct EmptyState<Actions: View>: View {
    let title: Text
    let icon: Lucide
    var description: Text?
    @ViewBuilder var actions: () -> Actions

    var body: some View {
        ContentUnavailableView {
            Label { title } icon: { Icon(icon, size: 44, relativeTo: .largeTitle) }
        } description: {
            if let description { description }
        } actions: {
            actions()
        }
    }
}

extension EmptyState where Actions == EmptyView {
    init(_ title: LocalizedStringKey, icon: Lucide, description: Text? = nil) {
        self.init(title: Text(title), icon: icon, description: description) { EmptyView() }
    }

    init(verbatim title: String, icon: Lucide, description: Text? = nil) {
        self.init(title: Text(verbatim: title), icon: icon, description: description) { EmptyView() }
    }
}

/// The skip buttons' icon, as the web player draws it: Lucide's rotate arrow,
/// back or forward, around the seconds it skips, the listener's setting.
struct SkipIcon: View {
    let forward: Bool
    let seconds: Double
    var size: CGFloat = 30

    var body: some View {
        ZStack {
            Icon(forward ? .rotateCw : .rotateCcw, size: size)
            Text(verbatim: "\(Int(seconds))")
                .font(.geist(size: size * 0.3, relativeTo: .body, weight: .bold))
                .monospacedDigit()
                .minimumScaleFactor(0.7)
                .frame(width: size * 0.5)
                .offset(y: size * 0.02)
        }
        .accessibilityHidden(true)
    }
}
