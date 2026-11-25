import SwiftUI

/// Home, the first tab: what to pick up next, each as a shelf that scrolls
/// sideways and opens its own page from its heading. The Books being read
/// come first, then the Reference Books opened lately, which have no
/// Read-through to keep them anywhere else, each as a tile that goes
/// straight back into the player or reader; then To Read, as covers that
/// open the Book's page. Reading History and Reading Stats are below. All of it reads the device's copy, with
/// changes made offline on top.
struct HomeView: View {
    @Environment(AppModel.self) private var model
    @State private var current: [CatalogStore.ContinueItem]?
    @State private var reference: [CatalogStore.ContinueItem]?
    @State private var toRead: [BookSummary]?
    @State private var downloaded: Set<Int>?

    var body: some View {
        @Bindable var model = model
        NavigationStack(path: $model.homePath) {
            ScrollViewReader { proxy in
                ScrollView {
                    VStack(alignment: .leading, spacing: 0) {
                        Color.clear.frame(height: 0).id(ScrollTop.anchor)
                        if let current, let toRead, let reference, current.isEmpty, toRead.isEmpty, reference.isEmpty {
                            EmptyNotice(title: String(localized: "Nothing to pick up yet"),
                                        message: String(localized: "Books you're reading and Books on To Read appear here."),
                                        icon: .bookOpen)
                                .accessibilityIdentifier("home.empty")
                        }
                        if let current, !current.isEmpty {
                            ShelfHeading(title: String(localized: "Currently Reading"), count: current.count, route: .currentlyReading,
                                         identifier: "home.current")
                            Shelf(identifier: "home.current.shelf") {
                                ForEach(current) { item in
                                    ResumeTile(item: item, identifier: "home.current.tile")
                                }
                            }
                        }
                        if let reference, !reference.isEmpty {
                            ShelfHeading(title: String(localized: "Recent Reference"), count: reference.count, route: .recentReference,
                                         identifier: "home.reference")
                            Shelf(identifier: "home.reference.shelf") {
                                ForEach(reference.prefix(20)) { item in
                                    ResumeTile(item: item, identifier: "home.reference.tile") {
                                        try? model.session?.store.clearRecent(bookIDs: [item.book.id])
                                    }
                                }
                            }
                        }
                        if let toRead, !toRead.isEmpty {
                            ShelfHeading(title: String(localized: "To Read"), count: toRead.count, route: .toRead, identifier: "home.toRead")
                            Shelf(identifier: "home.toRead.shelf") {
                                ForEach(toRead.prefix(20)) { book in
                                    ShelfBook(book: book, downloaded: downloaded?.contains(book.id) == true)
                                }
                            }
                        }
                        VStack(spacing: 8) {
                            HomeLink(title: String(localized: "Reading History"), icon: .history, route: .history, identifier: "reading.history")
                            if model.offers(.readingStats) {
                                HomeLink(title: String(localized: "Reading Stats"), icon: .chartColumn, route: .stats, identifier: "reading.stats")
                            }
                        }
                        .padding(.horizontal, GridLayout.margin)
                        .padding(.top, 32)
                    }
                    .padding(.bottom, 32)
                    .frame(maxWidth: 900)
                    .frame(maxWidth: .infinity)
                }
                .miniPlayerClearance()
                .accessibilityIdentifier("home.root")
                .onChange(of: model.scrollToTop[.home]) { _, _ in
                    withAnimation { proxy.scrollTo(ScrollTop.anchor, anchor: .top) }
                }
            }
            .background(Theme.background)
            .refreshable { await model.syncNow() }
            .navigationTitle("Home")
            .navigationBarTitleDisplayMode(.large)
            .toolbar { RootToolbar() }
            .routeDestinations()
        }
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $current) { db in
            try CatalogStore.continueItems(db)
        }
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $reference) { db in
            try CatalogStore.recentReferenceItems(db)
        }
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $toRead) { db in
            try CatalogStore.toReadBooks(db)
        }
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $downloaded) { db in
            try CatalogStore.downloadedBookIDs(db)
        }
    }
}

/// A shelf's heading, which opens the shelf's own page.
private struct ShelfHeading: View {
    let title: String
    let count: Int
    let route: Route
    let identifier: String

    var body: some View {
        NavigationLink(value: route) {
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                Text(title)
                    .font(.geist(.title3, weight: .semibold))
                    .foregroundStyle(Theme.foreground)
                Text(verbatim: "\(count)")
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
                Icon(.chevronRight, size: 16, relativeTo: .title3)
                    .foregroundStyle(Theme.mutedForeground)
                    .accessibilityHidden(true)
                Spacer(minLength: 0)
            }
            .frame(minHeight: 44)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(.horizontal, GridLayout.margin)
        .padding(.top, 24)
        .padding(.bottom, 4)
        .accessibilityLabel(String(localized: "\(title), \(count)"))
        .accessibilityHint(String(localized: "Opens every one"))
        .accessibilityAddTraits(.isHeader)
        .accessibilityIdentifier("\(identifier).header")
    }
}

/// A row of a shelf, scrolling sideways.
private struct Shelf<Content: View>: View {
    let identifier: String
    @ViewBuilder let content: Content

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            LazyHStack(alignment: .top, spacing: GridLayout.spacing) {
                content
            }
            .padding(.horizontal, GridLayout.margin)
            .padding(.vertical, 6)
        }
        .accessibilityIdentifier(identifier)
    }
}

/// A Book on a shelf, as the Library's grid shows it.
private struct ShelfBook: View {
    @ScaledMetric(relativeTo: .body) private var width: CGFloat = 112
    let book: BookSummary
    let downloaded: Bool

    var body: some View {
        NavigationLink(value: Route.book(book)) {
            BookCell(book: book, width: width, downloaded: downloaded)
                .frame(width: width)
        }
        .buttonStyle(PressableStyle())
        .accessibilityValue(downloaded ? String(localized: "Downloaded") : "")
    }
}

/// A Book that opens straight into the player or a reader, where it was
/// left: its cover with a play or read disc on it, so it doesn't look like
/// the covers that open a Book's page, its title, and how far through it
/// is. Pressing and holding it offers the Book's page, and remove offers
/// taking it off the shelf it's on.
struct ResumeTile: View {
    @Environment(AppModel.self) private var model
    @ScaledMetric(relativeTo: .body) private var scaledWidth: CGFloat = 112
    let item: CatalogStore.ContinueItem
    let identifier: String
    /// The tile's width, or nil for the shelf's.
    var width: CGFloat?
    var remove: (() -> Void)?

    private var tileWidth: CGFloat { width ?? scaledWidth }
    private var playing: Bool { model.player?.file?.fileID == item.fileID && model.player?.isPlaying == true }

    private var resumeLabel: String {
        item.listens ? String(localized: "Listen to \(item.book.title)") : String(localized: "Read \(item.book.title)")
    }

    var body: some View {
        Button(action: resume) {
            VStack(alignment: .leading, spacing: 6) {
                CoverView(book: item.book, width: tileWidth)
                    .overlay(alignment: .bottomTrailing) {
                        Group {
                            if playing {
                                PlayingIndicator(playing: true, height: 12)
                                    .foregroundStyle(Theme.primaryForeground)
                            } else {
                                Icon(item.listens ? .headphones : .bookOpen, size: 16, relativeTo: nil)
                                    .foregroundStyle(Theme.primaryForeground)
                            }
                        }
                        .frame(width: 34, height: 34)
                        .background(Theme.primary, in: Circle())
                        .shadow(color: .black.opacity(0.25), radius: 4, y: 2)
                        .padding(6)
                    }
                Text(item.book.title)
                    .font(.geist(.subheadline, weight: .semibold))
                    .foregroundStyle(Theme.foreground)
                    .lineLimit(1)
                Text(detail)
                    .font(.geist(.caption))
                    .foregroundStyle(Theme.mutedForeground)
                    .monospacedDigit()
                    .lineLimit(1)
            }
            .frame(width: tileWidth, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(PressableStyle())
        .modifier(NeedsConnection(available: model.canOpen(fileID: item.fileID)))
        .contextMenu {
            Button { model.push(.book(item.book)) } label: {
                Label("Open Book Page", menuIcon: .book)
            }
            .accessibilityIdentifier("\(identifier).book")
            if let remove {
                Button(action: remove) {
                    Label("Remove from Recent", menuIcon: .x)
                }
                .accessibilityIdentifier("\(identifier).remove")
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(resumeLabel)
        .accessibilityValue(detail)
        .accessibilityAddTraits(.isButton)
        .accessibilityActions {
            Button(String(localized: "Open Book Page")) { model.push(.book(item.book)) }
            if let remove {
                Button(String(localized: "Remove from Recent"), action: remove)
            }
        }
        .accessibilityIdentifier(identifier)
    }

    /// How far through it is, or what the tile does with none yet.
    private var detail: String {
        if let progress = item.progress, progress > 0 {
            return String(localized: "\(Int((progress * 100).rounded()))%")
        }
        return item.listens ? String(localized: "Listen") : String(localized: "Read")
    }

    private func resume() {
        if item.listens { model.listen(fileID: item.fileID) } else { model.read(fileID: item.fileID) }
    }
}

/// Resume tiles in a grid, for a shelf's own page.
struct ResumeGrid: View {
    @Environment(\.horizontalSizeClass) private var sizeClass
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let items: [CatalogStore.ContinueItem]
    let width: CGFloat
    let identifier: String
    var remove: ((CatalogStore.ContinueItem) -> Void)?

    var body: some View {
        let layout = GridLayout(width: width, regular: sizeClass == .regular, accessibility: dynamicTypeSize.isAccessibilitySize)
        let column = GridItem(.fixed(layout.cellWidth), spacing: GridLayout.spacing, alignment: .top)
        LazyVGrid(columns: Array(repeating: column, count: layout.columns), spacing: 22) {
            ForEach(items) { item in
                ResumeTile(item: item, identifier: identifier, width: layout.cellWidth, remove: remove.map { remove in { remove(item) } })
            }
        }
        .padding(.horizontal, GridLayout.margin)
    }
}

/// A row that opens one of Home's other pages.
private struct HomeLink: View {
    let title: String
    let icon: Lucide
    let route: Route
    let identifier: String

    var body: some View {
        NavigationLink(value: route) {
            HStack(spacing: 12) {
                Icon(icon, size: 18, relativeTo: .body)
                    .foregroundStyle(Theme.primary)
                    .frame(width: 24)
                Text(title)
                    .font(.geist(.callout, weight: .medium))
                    .foregroundStyle(Theme.foreground)
                Spacer()
                Icon(.chevronRight, size: 14, relativeTo: .body)
                    .foregroundStyle(Theme.mutedForeground)
                    .accessibilityHidden(true)
            }
            .padding(14)
            .frame(minHeight: 44)
            .cardStyle(radius: Theme.radius + 2)
            .contentShape(Rectangle())
        }
        .buttonStyle(PressableStyle())
        .accessibilityIdentifier(identifier)
    }
}

/// The Reference Books opened on this device lately, each opening straight
/// into the player or reader. Nothing about them is saved anywhere else,
/// so taking one off only tidies the list: opening it again brings it back.
struct RecentReferenceView: View {
    @Environment(AppModel.self) private var model
    @State private var items: [CatalogStore.ContinueItem]?
    @State private var confirmingClear = false

    var body: some View {
        GeometryReader { geometry in
            ScrollView {
                if let items {
                    if items.isEmpty {
                        EmptyNotice(title: String(localized: "No Reference Books opened lately"),
                                    message: String(localized: "Reference Books you open in the reader or player appear here, so you can jump back in."),
                                    icon: .library)
                    } else {
                        ResumeGrid(items: items, width: geometry.size.width, identifier: "recentReference.tile") { item in
                            clear([item.book.id])
                        }
                        .padding(.top, 8)
                    }
                }
            }
            .miniPlayerClearance()
        }
        .background(Theme.background)
        .navigationTitle("Recent Reference")
        .navigationBarTitleDisplayMode(.large)
        .toolbar {
            if !(items ?? []).isEmpty {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(String(localized: "Clear")) { confirmingClear = true }
                        .font(.geist(.body, weight: .medium))
                        .accessibilityIdentifier("recentReference.clear")
                }
            }
        }
        .confirmationDialog(String(localized: "Clear Recent Reference?"), isPresented: $confirmingClear, titleVisibility: .visible) {
            Button(String(localized: "Clear"), role: .destructive) { clear(nil) }
                .accessibilityIdentifier("recentReference.clear.confirm")
        } message: {
            Text("The Books stay Reference. Opening one again brings it back here.")
        }
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $items) { db in
            try CatalogStore.recentReferenceItems(db)
        }
    }

    private func clear(_ bookIDs: [Int]?) {
        try? model.session?.store.clearRecent(bookIDs: bookIDs)
    }
}
