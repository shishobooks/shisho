import SwiftUI

/// The signed-in app: the Library, Lists, and Downloads tabs of the prototype's native
/// structure (variant E), each with its own navigation, the mini player
/// above the tab bar on every one, and the Account sheet any tab opens.
struct MainTabs: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        // A TabView with its own bar hidden keeps both tabs alive, each with
        // its navigation, and hides the one not showing from VoiceOver, which
        // accessibilityHidden can't do inside a NavigationStack. The
        // prototype's tab bar sits over it instead of the system's floating
        // one; every screen hides the system's (routeDestinations).
        TabView(selection: Binding(get: { model.tab }, set: { model.tab = $0 })) {
            LibraryView()
                .withMiniPlayer()
                .tag(AppTab.library)
            ListsView()
                .withMiniPlayer()
                .tag(AppTab.lists)
            DownloadsView()
                .withMiniPlayer()
                .tag(AppTab.downloads)
        }
        // Over the screens, which leave room for it at the end of their
        // scrolling (miniPlayerClearance), as the prototype's bar is.
        .overlay(alignment: .bottom) {
            AppTabBar()
        }
        // The bar stays at the bottom under the keyboard, as the system's tab
        // bar does, rather than riding up over search results. Each tab's
        // screens still make room for the keyboard themselves.
        .ignoresSafeArea(.keyboard)
        .tint(Theme.primary)
        .sheet(isPresented: Binding(get: { model.showingAccount }, set: { model.showingAccount = $0 })) {
            AccountSheet()
        }
    }

}

/// The tab bar of the prototype's native structure (variant E): across the
/// bottom, the page's color blurred behind a hairline, each tab's icon over
/// its name, the one showing in the primary color. Tapping the one showing
/// goes back to its first screen, as a native tab bar does.
struct AppTabBar: View {
    @Environment(AppModel.self) private var model
    /// Its height above the bottom safe area, which screens leave clear.
    static let height: CGFloat = 53

    /// The strip above the tabs that says the server can't be reached.
    static let connectionHeight: CGFloat = 30

    /// The room screens leave for the bar, with the strip when it shows.
    static func room(_ connection: Connection) -> CGFloat {
        height + (connection == .online ? 0 : connectionHeight)
    }

    var body: some View {
        VStack(spacing: 0) {
            if model.connection != .online {
                ConnectionStrip(connection: model.connection)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
            tabs
        }
        .animation(.easeOut(duration: 0.25), value: model.connection)
    }

    private var tabs: some View {
        HStack(spacing: 0) {
            item(.library, String(localized: "Library"), icon: .library, identifier: "tab.library")
            item(.lists, String(localized: "Lists"), icon: .list, identifier: "tab.lists")
            item(.downloads, String(localized: "Downloads"), icon: .download, identifier: "tab.downloads")
        }
        .padding(.top, 7)
        .padding(.bottom, 2)
        .frame(maxWidth: .infinity)
        // The bar and its hairline run edge to edge, under the home
        // indicator and, in landscape, past the side safe areas, while the
        // tabs stay inside them.
        .background {
            Theme.background.opacity(0.85)
                .background(.ultraThinMaterial)
                .overlay(alignment: .top) {
                    Rectangle().fill(Theme.border).frame(height: 1)
                }
                .ignoresSafeArea(edges: [.bottom, .horizontal])
        }
        // A fixed size, as the system's tab bar keeps, so it always fits the
        // room screens leave for it; at accessibility sizes a long press
        // shows each tab large instead.
        .dynamicTypeSize(.large)
    }

    private func item(_ tab: AppTab, _ title: String, icon: Lucide, identifier: String) -> some View {
        let selected = model.tab == tab
        return Button {
            if selected {
                // Back to the first screen, or, on it, back to its top.
                model.reselect(tab)
            } else {
                model.tab = tab
            }
        } label: {
            VStack(spacing: 3) {
                Icon(icon, size: 24, relativeTo: nil)
                    .frame(height: 26)
                Text(title)
                    .font(.geist(size: 10.5, relativeTo: .caption2, weight: .medium))
            }
            .foregroundStyle(selected ? Theme.primary : Theme.mutedForeground)
            .frame(maxWidth: .infinity, minHeight: 44)
            .contentShape(Rectangle())
        }
        .buttonStyle(TabPressStyle())
        .accessibilityShowsLargeContentViewer {
            Label(verbatim: title, menuIcon: icon)
        }
        .accessibilityLabel(title)
        .accessibilityAddTraits(selected ? [.isSelected] : [])
        .accessibilityIdentifier(identifier)
    }
}

/// Says, across the app, that the account's server can't be reached, and
/// why, while what's on the device keeps working.
private struct ConnectionStrip: View {
    let connection: Connection

    var body: some View {
        HStack(spacing: 6) {
            Icon(.wifiOff, size: 13, relativeTo: nil)
            Text(connection == .offline ? String(localized: "Offline") : String(localized: "Can't reach your server"))
                .font(.geist(size: 12, relativeTo: .caption, weight: .semibold))
                .lineLimit(1)
        }
        .foregroundStyle(Theme.mutedForeground)
        .frame(maxWidth: .infinity, minHeight: AppTabBar.connectionHeight)
        .background(Theme.muted)
        .overlay(alignment: .top) {
            Rectangle().fill(Theme.border).frame(height: 1)
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(connection == .offline
            ? String(localized: "Offline. What's downloaded still works.")
            : String(localized: "Your server can't be reached. What's downloaded still works."))
        .accessibilityIdentifier("connection.bar")
    }
}

/// The prototype's tab press: the icon gives a little under the finger.
private struct TabPressStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .scaleEffect(configuration.isPressed ? 0.92 : 1)
            .animation(.spring(response: 0.3, dampingFraction: 0.6), value: configuration.isPressed)
    }
}

/// The top right of each tab's first screen, as the prototype has it:
/// search, which opens on top of the tab, and the account.
struct RootToolbar: ToolbarContent {
    @Environment(AppModel.self) private var model
    /// The Downloads tab's: its settings, rarely changed, behind a gear.
    var downloadSettings = false

    var body: some ToolbarContent {
        ToolbarItemGroup(placement: .topBarTrailing) {
            if downloadSettings {
                NavigationLink(value: Route.downloadSettings) {
                    Icon(.settings, size: 20, relativeTo: nil)
                        .foregroundStyle(Theme.foreground)
                }
                .accessibilityLabel("Download Settings")
                .accessibilityIdentifier("downloads.settings")
            }
            NavigationLink(value: Route.search) {
                Icon(.search, size: 20, relativeTo: nil)
                    .foregroundStyle(Theme.foreground)
            }
            .accessibilityLabel("Search")
            .accessibilityIdentifier("search.button")
            AccountButton { model.showingAccount = true }
        }
    }
}

extension View {
    /// The mini player over the bottom of a tab, outside its navigation so
    /// it stays on every pushed screen. Each screen's scroll view leaves
    /// room for it (miniPlayerClearance).
    func withMiniPlayer() -> some View {
        modifier(MiniPlayerOverlay())
    }

    /// The pages any tab can push: a Book, a browsing list, a Series,
    /// Person, Genre, or Tag, a List, and search. Each, and the tab's first
    /// screen this is on, hides the system tab bar, since the prototype's
    /// takes its place (MainTabs).
    func routeDestinations() -> some View {
        toolbar(.hidden, for: .tabBar)
            .navigationDestination(for: Route.self) { route in
                RouteDestination(route: route)
                    .toolbar(.hidden, for: .tabBar)
            }
    }
}

/// The screen a Route opens.
private struct RouteDestination: View {
    let route: Route

    var body: some View {
        switch route {
        case .book(let book):
            BookDetailView(bookID: book.id, initial: book)
        case .browse(let kind):
            ResourceListView(kind: kind)
        case .resource(let kind, let id, let name):
            ResourceView(kind: kind, id: id, initialName: name)
        case .list(let id, let name):
            ListDetailView(listID: id, initialName: name)
        case .file(let bookID, let fileID):
            FileView(bookID: bookID, fileID: fileID)
        case .search:
            SearchView()
        case .downloadSettings:
            DownloadSettingsView()
        }
    }
}

private struct MiniPlayerOverlay: ViewModifier {
    @Environment(AppModel.self) private var model

    func body(content: Content) -> some View {
        // Above the tab bar, and like it, staying put under the keyboard.
        content.overlay {
            Color.clear
                .ignoresSafeArea(.keyboard)
                .overlay(alignment: .bottom) {
                    MiniPlayer()
                        .frame(maxWidth: 680)
                        .padding(.bottom, AppTabBar.room(model.connection))
                        .animation(.spring(response: 0.4, dampingFraction: 0.85), value: model.player?.file?.fileID)
                }
        }
    }
}

/// Books in a grid, as the Library shows them: as many columns as fit, each
/// opening its Book. badges are Series Numbers to show inline with titles.
struct BookGrid: View {
    @Environment(\.horizontalSizeClass) private var sizeClass
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let books: [BookSummary]
    let width: CGFloat
    var badges: [Int: (Double, Double?)] = [:]
    var identifier = "library.grid"

    var body: some View {
        let layout = GridLayout(width: width, regular: sizeClass == .regular,
                                accessibility: dynamicTypeSize.isAccessibilitySize)
        let column = GridItem(.fixed(layout.cellWidth), spacing: GridLayout.spacing, alignment: .top)
        LazyVGrid(columns: Array(repeating: column, count: layout.columns), spacing: 22) {
            ForEach(books) { book in
                NavigationLink(value: Route.book(book)) {
                    BookCell(book: book, width: layout.cellWidth, seriesNumber: badges[book.id])
                }
                .buttonStyle(PressableStyle())
            }
        }
        .padding(.horizontal, GridLayout.margin)
        .accessibilityIdentifier(identifier)
    }
}

/// A screen's heading over its content, with a count beside it.
struct SectionHeading: View {
    let title: String
    var count: String?

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(title)
                .font(.geist(.title3, weight: .semibold))
                .accessibilityAddTraits(.isHeader)
            Spacer()
            if let count {
                Text(count)
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
            }
        }
        .padding(.horizontal, GridLayout.margin)
        .padding(.top, 20)
        .padding(.bottom, 12)
    }
}

/// Says a screen has nothing to show yet.
struct EmptyNotice: View {
    let title: String
    var message: String?
    let icon: Lucide

    var body: some View {
        VStack(spacing: 10) {
            Icon(icon, size: 40, relativeTo: nil)
                .foregroundStyle(Theme.primary)
                .padding(.bottom, 4)
            Text(title)
                .font(.geist(.headline, weight: .semibold))
            if let message {
                Text(message)
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
                    .multilineTextAlignment(.center)
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 56)
        .padding(.horizontal, 32)
        .accessibilityElement(children: .combine)
    }
}

// MARK: - The web app's pieces

/// The web app's small outlined button (`Button variant="outline"
/// size="sm"`), for choosing and browsing.
struct SmallOutlineStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.geist(.footnote, weight: .medium))
            .foregroundStyle(Theme.foreground)
            .padding(.horizontal, 12)
            .frame(minHeight: 34)
            .background(Theme.card.opacity(configuration.isPressed ? 0.6 : 1), in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(Theme.border))
            .shadow(color: .black.opacity(0.04), radius: 1, y: 1)
            .hitArea(height: 34)
            .scaleEffect(configuration.isPressed ? 0.97 : 1)
            .animation(.spring(response: 0.25, dampingFraction: 0.7), value: configuration.isPressed)
    }
}

/// The web app's secondary badge: a count or label in a muted rounded
/// rectangle, such as "3 books".
struct CountBadge: View {
    let text: String

    var body: some View {
        Text(text)
            .font(.geist(.footnote, weight: .medium))
            .foregroundStyle(Theme.foreground)
            .padding(.horizontal, 9)
            .padding(.vertical, 4)
            .background(Theme.secondary, in: RoundedRectangle(cornerRadius: 6, style: .continuous))
    }
}

/// A resource's first covers, fanned, as the web app's lists show them.
struct PreviewStack: View {
    let books: [BookSummary]
    var height: CGFloat = 54

    var body: some View {
        let width = height * 2 / 3
        // Room for three, whatever the count, so the names beside the
        // stacks line up, as on the web.
        ZStack(alignment: .leading) {
            if books.isEmpty {
                RoundedRectangle(cornerRadius: 4, style: .continuous)
                    .fill(Theme.muted)
                    .frame(width: width, height: height)
            }
            ForEach(Array(books.prefix(3).enumerated().reversed()), id: \.element.id) { index, book in
                CoverView(book: book, width: width)
                    .frame(height: height, alignment: .top)
                    .clipped()
                    .offset(x: CGFloat(index) * width * 0.22)
            }
        }
        .frame(width: width * 1.44, height: height, alignment: .leading)
        .accessibilityHidden(true)
    }
}

/// A Person, Genre, or Tag in a list, as the web app's resource lists show
/// one: its covers, its name over its sort name and other names, and its
/// counts.
struct ResourceRow: View {
    let item: ResourceSummary
    let kind: ResourceKind

    private var badges: [String] {
        guard kind == .person else { return [Counts.books(item.bookCount)] }
        var out: [String] = []
        if item.authoredCount > 0 {
            out.append(String(AttributedString(localized: "^[\(item.authoredCount) book](inflect: true) authored").characters))
        }
        if item.narratedCount > 0 {
            out.append(String(AttributedString(localized: "^[\(item.narratedCount) file](inflect: true) narrated").characters))
        }
        return out
    }

    var body: some View {
        HStack(spacing: 12) {
            PreviewStack(books: item.previews)
            VStack(alignment: .leading, spacing: 2) {
                Text(item.name)
                    .font(.geist(.headline, weight: .semibold))
                    .foregroundStyle(Theme.foreground)
                    .lineLimit(2)
                if !item.detail.isEmpty {
                    Text(item.detail)
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                        .lineLimit(1)
                }
            }
            Spacer(minLength: 8)
            VStack(alignment: .trailing, spacing: 4) {
                ForEach(badges, id: \.self) { CountBadge(text: $0) }
            }
        }
        .padding(12)
        .cardStyle(radius: Theme.radius + 2)
        .contentShape(Rectangle())
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(([item.name] + badges).joined(separator: ", "))
        .accessibilityAddTraits(.isButton)
    }
}

/// A Series in a grid, as the web app shows one: its first Book's cover,
/// its name, and how many Books it has.
struct SeriesCard: View {
    let item: ResourceSummary
    let width: CGFloat

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let first = item.previews.first {
                CoverView(book: first, width: width)
                    .padding(.bottom, 4)
            }
            Text(item.name)
                .font(.geist(.subheadline, weight: .semibold))
                .foregroundStyle(Theme.foreground)
                .lineLimit(2)
                .multilineTextAlignment(.leading)
            CountBadge(text: Counts.books(item.bookCount))
        }
        .frame(width: width, alignment: .leading)
        .contentShape(Rectangle())
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(item.name + ", " + Counts.books(item.bookCount))
        .accessibilityAddTraits(.isButton)
    }
}

/// The id of the view a tab's first screen scrolls back to when its tab is
/// tapped again.
enum ScrollTop {
    static let anchor = "scroll.top"
}
