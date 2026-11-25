import SwiftUI

/// Home, the first tab: what to pick up next, each as a shelf that scrolls
/// sideways and opens its own page from its heading. The Books being read
/// come first, as the prototype's Continue cards with a button that goes
/// straight back in; then the Reference Books opened lately, which have no
/// Read-through to keep them anywhere else; then To Read. Reading History
/// and Reading Stats are below. All of it reads the device's copy, with
/// changes made offline on top.
struct HomeView: View {
    @Environment(AppModel.self) private var model
    @State private var current: [CatalogStore.ContinueItem]?
    @State private var reference: [BookSummary]?
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
                                    ContinueCard(item: item)
                                }
                            }
                        }
                        if let reference, !reference.isEmpty {
                            ShelfHeading(title: String(localized: "Recent Reference"), count: reference.count, route: .recentReference,
                                         identifier: "home.reference")
                            Shelf(identifier: "home.reference.shelf") {
                                ForEach(reference.prefix(20)) { book in
                                    ShelfBook(book: book, downloaded: downloaded?.contains(book.id) == true)
                                        .contextMenu {
                                            Button { try? model.session?.store.clearRecent(bookIDs: [book.id]) } label: {
                                                Label("Remove from Recent", menuIcon: .x)
                                            }
                                            .accessibilityIdentifier("home.reference.remove")
                                        }
                                        .accessibilityAction(named: Text("Remove from Recent")) {
                                            try? model.session?.store.clearRecent(bookIDs: [book.id])
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
            try CatalogStore.recentReference(db)
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

/// A Book being read, as the prototype's Continue card: its cover, title,
/// authors, and how far through it is, which open its page, and a button
/// that goes straight back to listening or reading where it was left.
private struct ContinueCard: View {
    @Environment(AppModel.self) private var model
    @ScaledMetric(relativeTo: .body) private var width: CGFloat = 290
    let item: CatalogStore.ContinueItem

    private var playing: Bool { model.player?.file?.fileID == item.fileID && model.player?.isPlaying == true }

    var body: some View {
        HStack(spacing: 12) {
            Button { model.push(.book(item.book)) } label: {
                HStack(spacing: 12) {
                    CoverView(book: item.book, width: item.listens ? 56 : 40)
                        .frame(width: 56)
                    VStack(alignment: .leading, spacing: 3) {
                        Text(item.book.title)
                            .font(.geist(.subheadline, weight: .semibold))
                            .foregroundStyle(Theme.foreground)
                            .lineLimit(1)
                        if !item.book.authors.isEmpty {
                            Text(item.book.authors)
                                .font(.geist(.caption))
                                .foregroundStyle(Theme.mutedForeground)
                                .lineLimit(1)
                        }
                        if let progress = item.book.progress {
                            ProgressBar(value: progress)
                                .padding(.top, 5)
                        }
                    }
                    Spacer(minLength: 0)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityElement(children: .combine)
            .accessibilityHint(String(localized: "Opens the Book's page"))
            .accessibilityIdentifier("home.current.card")

            Button {
                if item.listens { model.listen(fileID: item.fileID) } else { model.read(fileID: item.fileID) }
            } label: {
                Group {
                    if playing {
                        PlayingIndicator(playing: true, height: 14)
                    } else {
                        Icon(item.listens ? .headphones : .bookOpen, size: 18, relativeTo: nil)
                    }
                }
                .foregroundStyle(Theme.primary)
                .frame(width: 44, height: 44)
                .background(Theme.primary.opacity(0.12), in: Circle())
            }
            .buttonStyle(PlayerControlStyle())
            .accessibilityLabel(item.listens ? String(localized: "Resume listening to \(item.book.title)")
                                             : String(localized: "Resume reading \(item.book.title)"))
            .accessibilityIdentifier("home.current.resume")
            .modifier(NeedsConnection(available: model.canOpen(fileID: item.fileID)))
        }
        .padding(10)
        .frame(width: width)
        .cardStyle(radius: 14)
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

/// The Reference Books opened on this device lately. Nothing about them is
/// saved anywhere else, so taking one off only tidies the list: opening it
/// again brings it back.
struct RecentReferenceView: View {
    @Environment(AppModel.self) private var model
    @State private var books: [BookSummary]?
    @State private var confirmingClear = false

    var body: some View {
        Group {
            if let books, books.isEmpty {
                EmptyNotice(title: String(localized: "No Reference Books opened lately"),
                            message: String(localized: "Reference Books you open in the reader or player appear here, so you can jump back in."),
                            icon: .library)
                    .frame(maxHeight: .infinity, alignment: .top)
            } else {
                List {
                    ForEach(books ?? []) { book in
                        Button { model.push(.book(book)) } label: {
                            BookRow(book: book)
                        }
                        .buttonStyle(.plain)
                        .listRowBackground(Theme.background)
                        .accessibilityIdentifier("recentReference.row")
                        .swipeActions {
                            Button(role: .destructive) { clear([book.id]) } label: {
                                Label("Remove", menuIcon: .x)
                            }
                            .accessibilityIdentifier("recentReference.remove")
                        }
                        .accessibilityAction(named: Text("Remove from Recent")) { clear([book.id]) }
                    }
                }
                .listStyle(.plain)
                .scrollContentBackground(.hidden)
                .miniPlayerClearance()
            }
        }
        .background(Theme.background)
        .navigationTitle("Recent Reference")
        .navigationBarTitleDisplayMode(.large)
        .toolbar {
            if !(books ?? []).isEmpty {
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
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $books) { db in
            try CatalogStore.recentReference(db)
        }
    }

    private func clear(_ bookIDs: [Int]?) {
        try? model.session?.store.clearRecent(bookIDs: bookIDs)
    }
}
