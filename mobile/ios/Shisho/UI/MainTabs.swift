import SwiftUI

/// The signed-in app: the Library, Lists, and Search tabs, each with its own
/// navigation, and the mini player above the tab bar on every one.
struct MainTabs: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        @Bindable var model = model
        TabView(selection: $model.tab) {
            LibraryView()
                .withMiniPlayer()
                .tabItem { Label("Library", systemImage: "books.vertical") }
                .tag(AppTab.library)
            ListsView()
                .withMiniPlayer()
                .tabItem { Label("Lists", systemImage: "list.bullet.rectangle") }
                .tag(AppTab.lists)
            SearchView()
                .withMiniPlayer()
                .tabItem { Label("Search", systemImage: "magnifyingglass") }
                .tag(AppTab.search)
        }
        .tint(Theme.primary)
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
    /// Person, Genre, or Tag, and a List.
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
