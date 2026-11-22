import SwiftUI

/// The signed-in app: the Library and Lists tabs of the prototype's native
/// structure (variant E), each with its own navigation, the mini player
/// above the tab bar on every one, and the Account sheet any tab opens.
struct MainTabs: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        @Bindable var model = model
        TabView(selection: $model.tab) {
            LibraryView()
                .withMiniPlayer()
                .tabItem { Label("Library", systemImage: "books.vertical").accessibilityIdentifier("tab.library") }
                .tag(AppTab.library)
            ListsView()
                .withMiniPlayer()
                .tabItem { Label("Lists", systemImage: "list.bullet").accessibilityIdentifier("tab.lists") }
                .tag(AppTab.lists)
        }
        .tint(Theme.primary)
        .sheet(isPresented: $model.showingAccount) {
            AccountSheet()
        }
    }
}

/// The top right of each tab's first screen, as the prototype has it:
/// search, which opens on top of the tab, and the account.
struct RootToolbar: ToolbarContent {
    @Environment(AppModel.self) private var model

    var body: some ToolbarContent {
        ToolbarItemGroup(placement: .topBarTrailing) {
            NavigationLink(value: Route.search) {
                Image(systemName: "magnifyingglass")
                    .font(.system(size: 17, weight: .medium))
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
    /// Person, Genre, or Tag, a List, and search.
    func routeDestinations() -> some View {
        navigationDestination(for: Route.self) { route in
            switch route {
            case .book(let book):
                BookDetailView(bookID: book.id, initial: book)
            case .browse(let kind):
                ResourceListView(kind: kind)
            case .resource(let kind, let id, let name):
                ResourceView(kind: kind, id: id, initialName: name)
            case .list(let id, let name):
                ListDetailView(listID: id, initialName: name)
            case .search:
                SearchView()
            }
        }
    }
}

private struct MiniPlayerOverlay: ViewModifier {
    @Environment(AppModel.self) private var model

    func body(content: Content) -> some View {
        content.overlay(alignment: .bottom) {
            MiniPlayer()
                .frame(maxWidth: 680)
                .animation(.spring(response: 0.4, dampingFraction: 0.85), value: model.player?.file?.fileID)
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
    let message: String
    let systemImage: String

    var body: some View {
        VStack(spacing: 10) {
            Image(systemName: systemImage)
                .font(.system(size: 36, weight: .light))
                .foregroundStyle(Theme.primary)
                .padding(.bottom, 4)
                .accessibilityHidden(true)
            Text(title)
                .font(.geist(.headline, weight: .semibold))
            Text(message)
                .font(.geist(.subheadline))
                .foregroundStyle(Theme.mutedForeground)
                .multilineTextAlignment(.center)
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
            .font(.geist(.caption, weight: .medium))
            .foregroundStyle(Theme.foreground)
            .padding(.horizontal, 8)
            .padding(.vertical, 3)
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
