import SwiftUI

/// The web app's icons, Lucide, drawn the same way: a 24 point grid with a
/// stroke of 2 that scales with the icon. Add a case named for the Lucide
/// icon (the raw value when Swift can't spell it), then run `mise ios:icons`,
/// which writes it into the asset catalog from the web app's lucide-react. A
/// raw value ending in "-fill" is the icon filled.
enum Lucide: String, CaseIterable {
    case arrowDown = "arrow-down"
    case arrowDownRight = "arrow-down-right"
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
    case chevronLeft = "chevron-left"
    case chevronRight = "chevron-right"
    case chevronUp = "chevron-up"
    case chevronsUpDown = "chevrons-up-down"
    case circleAlert = "circle-alert"
    case clock
    case clockArrowUp = "clock-arrow-up"
    case download
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
    case redo2 = "redo-2"
    case rotateCcw = "rotate-ccw"
    case rotateCw = "rotate-cw"
    case search
    case server
    case settings
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
    case undo2 = "undo-2"
    case upload
    case user
    case userX = "user-x"
    case users
    case wifi
    case wifiOff = "wifi-off"
    case x

    /// The icon as an image, for a Label, a menu item, or a swipe action.
    var image: Image {
        Image("Lucide/\(rawValue)").renderingMode(.template)
    }
}

/// A Lucide icon, size points across at the default text size, growing with
/// Dynamic Type as text of style does, in the foreground style. With no
/// style it stays size points, for one in a fixed-size frame, such as a
/// cover's ribbon or a control that caps its text size. It's decorative: the
/// control it's in carries the label.
struct Icon: View {
    let name: Lucide
    private let fixed: CGFloat?
    @ScaledMetric private var scaled: CGFloat

    init(_ name: Lucide, size: CGFloat = 20, relativeTo style: Font.TextStyle? = .body) {
        self.name = name
        fixed = style == nil ? size : nil
        _scaled = ScaledMetric(wrappedValue: size, relativeTo: style ?? .body)
    }

    var body: some View {
        let size = fixed ?? scaled
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
