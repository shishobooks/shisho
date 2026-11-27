import SwiftUI

/// Home, the first tab: what to pick up next, each as a shelf that scrolls
/// sideways and opens its own page from its heading. The Books being read
/// come first, then the Reference Books opened lately, which have no
/// Read-through to keep them anywhere else, each as a tile that goes
/// straight back into the player or reader; then To Read, as covers that
/// open the Book's page. Reading Stats and Reading History are below. All of
/// it reads the device's copy, with changes made offline on top.
struct HomeView: View {
    @Environment(AppModel.self) private var model
    @State private var current: [CatalogStore.ContinueItem]?
    @State private var reference: [CatalogStore.ContinueItem]?
    @State private var toRead: [BookSummary]?
    @State private var downloads: [Int: BookDownloads]?
    @State private var scrolledBy: CGFloat = 0

    var body: some View {
        @Bindable var model = model
        NavigationStack(path: $model.homePath) {
            ScrollViewReader { proxy in
                ScrollView {
                    VStack(alignment: .leading, spacing: 0) {
                        ScrollTopMarker(space: "home")
                            .id(ScrollTop.anchor)
                        if let current, let toRead, let reference, current.isEmpty, toRead.isEmpty, reference.isEmpty {
                            EmptyNotice(title: String(localized: "Nothing to pick up yet"),
                                        message: String(localized: "Books you're reading and Books on To Read appear here."),
                                        icon: .bookOpen)
                                .accessibilityIdentifier("home.empty")
                        }
                        if let current, !current.isEmpty {
                            ShelfHeading(title: String(localized: "Currently Reading"), count: current.count, route: .currentlyReading,
                                         identifier: "home.current")
                            Shelf(identifier: "home.current.shelf", equalHeights: true) {
                                ForEach(current.prefix(20)) { item in
                                    ResumeTile(item: item, identifier: "home.current.tile", downloads: downloads?[item.book.id])
                                }
                            }
                        }
                        if let reference, !reference.isEmpty {
                            ShelfHeading(title: String(localized: "Recent Reference"), count: reference.count, route: .recentReference,
                                         identifier: "home.reference")
                            Shelf(identifier: "home.reference.shelf", equalHeights: true) {
                                ForEach(reference.prefix(20)) { item in
                                    ResumeTile(item: item, identifier: "home.reference.tile", downloads: downloads?[item.book.id]) {
                                        try? model.session?.store.clearRecent(bookIDs: [item.book.id])
                                    }
                                }
                            }
                        }
                        if let toRead, !toRead.isEmpty {
                            ShelfHeading(title: String(localized: "To Read"), count: toRead.count, route: .toRead, identifier: "home.toRead")
                            // All laid out at once, so a later Book's longer
                            // title isn't cut to the first ones' height.
                            Shelf(identifier: "home.toRead.shelf", equalHeights: true) {
                                ForEach(toRead.prefix(20)) { book in
                                    ShelfBook(book: book, downloads: downloads?[book.id])
                                }
                            }
                        }
                        VStack(spacing: 8) {
                            // In the web's user menu's order.
                            if model.offers(.readingStats) {
                                HomeLink(title: String(localized: "Reading Stats"), icon: .chartColumn, route: .stats, identifier: "reading.stats")
                            }
                            HomeLink(title: String(localized: "Reading History"), icon: .history, route: .history, identifier: "reading.history")
                        }
                        .padding(.horizontal, GridLayout.margin)
                        .padding(.top, 32)
                    }
                    .padding(.bottom, 32)
                    .frame(maxWidth: 900)
                    .frame(maxWidth: .infinity)
                }
                .onScrolledDistanceChange(space: "home") { distance in
                    // Only as far as the bar's logo or title changes, so the
                    // page isn't redrawn on every frame of a long scroll.
                    let settled = min(LogoToolbarItem.settledAfter, distance)
                    if settled != scrolledBy { scrolledBy = settled }
                }
                .miniPlayerClearance()
                .accessibilityIdentifier("home.root")
                .onChange(of: model.scrollToTop[.home]) { _, _ in
                    withAnimation { proxy.scrollTo(ScrollTop.anchor, anchor: .top) }
                }
            }
            .background(Theme.background)
            .refreshable { await model.syncNow() }
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                LogoToolbarItem(scrolledBy: scrolledBy)
                RootToolbar()
            }
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
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $downloads) { db in
            try CatalogStore.booksDownloads(db)
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
    /// Every item as tall as the tallest, as the resume cards are. That
    /// needs them all laid out at once, so it's for short shelves.
    var equalHeights = false
    @ViewBuilder let content: Content

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            Group {
                if equalHeights {
                    HStack(alignment: .top, spacing: GridLayout.spacing) {
                        content
                    }
                    .fixedSize(horizontal: false, vertical: true)
                } else {
                    LazyHStack(alignment: .top, spacing: GridLayout.spacing) {
                        content
                    }
                }
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
    let downloads: BookDownloads?

    var body: some View {
        NavigationLink(value: Route.book(book)) {
            BookCell(book: book, width: width, downloaded: downloads?.hasDownloaded == true, downloads: downloads)
                .frame(width: width)
        }
        .buttonStyle(PressableStyle())
        .bookDownloadMenu(book: book, downloads: downloads)
    }
}

/// A Book that opens straight into the player or a reader, where it was
/// left, drawn as a card rather than a Library cell, since a tap here
/// doesn't open the Book's page: a small cover beside the title, and a
/// Listen or Read button that fills as far as the user is. Pressing and
/// holding offers the Book's page, and, for a Recent Reference Book, taking
/// it off the shelf it's on.
struct ResumeTile: View {
    @Environment(AppModel.self) private var model
    @ScaledMetric(relativeTo: .body) private var scaledWidth: CGFloat = 164
    /// The text beside the cover is this tall, whatever the cover's shape.
    @ScaledMetric(relativeTo: .body) private var rowHeight: CGFloat = 72
    @ScaledMetric(relativeTo: .body) private var squareCover: CGFloat = 60
    let item: CatalogStore.ContinueItem
    let identifier: String
    /// The Book's Downloads, for the line saying where they stand and the
    /// menu that starts or removes them.
    var downloads: BookDownloads?
    /// The card's width, or nil for the shelf's.
    var width: CGFloat?
    var remove: (() -> Void)?

    private var tileWidth: CGFloat { width ?? scaledWidth }
    private var coverWidth: CGFloat { item.listens ? squareCover : rowHeight * 2 / 3 }
    private var coverHeight: CGFloat { item.listens ? squareCover : rowHeight }
    private var playing: Bool { model.player?.file?.fileID == item.fileID && model.player?.isPlaying == true }

    /// How far through: the Read-through's progress, or for a Reference
    /// Book, which never has one underway, its place in the File. Nil until
    /// it reaches 1%, so a Book just started doesn't say "0%" beside its
    /// Download's percent.
    private var progress: Double? {
        let progress = item.book.progress ?? item.progress
        guard let progress, progress >= 0.005 else { return nil }
        return progress
    }

    private var actionTitle: String {
        if playing { return String(localized: "Listening") }
        return item.listens ? String(localized: "Listen") : String(localized: "Read")
    }

    private var resumeLabel: String {
        item.listens ? String(localized: "Listen to \(item.book.title)") : String(localized: "Read \(item.book.title)")
    }

    var body: some View {
        Button(action: resume) {
            VStack(alignment: .leading, spacing: 8) {
                HStack(alignment: .top, spacing: 8) {
                    // The cover of the File it opens: square for an audiobook.
                    CoverView(book: item.book, file: CoverFile(id: item.fileID, fileType: item.fileType, coverVersion: item.coverVersion),
                              width: coverWidth)
                        .frame(width: coverWidth, height: coverHeight)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(item.book.title)
                            .font(.geist(.footnote, weight: .semibold))
                            .foregroundStyle(Theme.foreground)
                            .lineLimit(3)
                            .multilineTextAlignment(.leading)
                        if !item.book.authors.isEmpty {
                            Text(item.book.authors)
                                .font(.geist(.caption))
                                .foregroundStyle(Theme.mutedForeground)
                                .lineLimit(1)
                        }
                        // Right under the title and author, not at the foot of
                        // the row, which an ebook's taller cover sets.
                        ResumeDownloadLine(downloads: downloads)
                            .padding(.top, 2)
                    }
                    .frame(maxWidth: .infinity, minHeight: rowHeight, alignment: .topLeading)
                }
                // A card made as tall as its shelf's tallest keeps the room
                // here, so every card's button sits along the bottom.
                Spacer(minLength: 0)
                actionBar
            }
            .padding(8)
            .frame(width: tileWidth, alignment: .leading)
            .frame(maxHeight: .infinity)
            .cardStyle(radius: 14)
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
            BookDownloadMenu(book: item.book, downloads: downloads)
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

    /// Listen or Read, filled as far as the user is, with the percent at
    /// its end: the button is the progress bar.
    private var actionBar: some View {
        HStack(spacing: 6) {
            Group {
                if playing {
                    PlayingIndicator(playing: true, height: 11)
                } else {
                    Icon(item.listens ? .headphones : .bookOpen, size: 13, relativeTo: .footnote)
                }
            }
            .foregroundStyle(Theme.primary)
            Text(actionTitle)
                .font(.geist(.footnote, weight: .bold))
                .foregroundStyle(Theme.foreground)
            Spacer(minLength: 4)
            if let progress {
                Text(verbatim: Percent.text(progress))
                    .font(.geist(.caption, weight: .semibold))
                    .monospacedDigit()
                    .foregroundStyle(Theme.mutedForeground)
            }
        }
        .padding(.horizontal, 10)
        .frame(maxWidth: .infinity, minHeight: 32)
        .background(ProgressFill(fraction: progress ?? 0, style: .subtle))
        .clipShape(RoundedRectangle(cornerRadius: 9, style: .continuous))
    }

    /// How far through it is, and where its Downloads stand.
    private var detail: String {
        var parts: [String] = []
        if let progress { parts.append(String(localized: "\(Int((progress * 100).rounded()))%")) }
        let progress = downloads?.isDownloading == true ? model.session?.downloads.progress ?? [:] : [:]
        if let words = downloads?.words(progress: progress) { parts.append(words) }
        return parts.joined(separator: ", ")
    }

    private func resume() {
        if item.listens { model.listen(fileID: item.fileID) } else { model.read(fileID: item.fileID) }
    }
}

/// The resume cards in columns, for the pages Home's headings open: two
/// across on a phone, more on an iPad, one at the largest text sizes.
struct ResumeGrid: View {
    @Environment(AppModel.self) private var model
    @Environment(\.horizontalSizeClass) private var sizeClass
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var downloads: [Int: BookDownloads]?
    let items: [CatalogStore.ContinueItem]
    let width: CGFloat
    let identifier: String
    var remove: ((CatalogStore.ContinueItem) -> Void)?

    var body: some View {
        let available = max(0, width - GridLayout.margin * 2)
        let columns = dynamicTypeSize.isAccessibilitySize ? 1
            : sizeClass == .regular ? max(2, Int((available + GridLayout.spacing) / 240)) : 2
        let cellWidth = ((available - GridLayout.spacing * CGFloat(columns - 1)) / CGFloat(columns)).rounded(.down)
        let rows = stride(from: 0, to: items.count, by: columns).map { Array(items[$0..<min($0 + columns, items.count)]) }
        // Lazy by row, for a long list, and each row laid out at once, so
        // its cards are as tall as its tallest, as on Home's shelf.
        LazyVStack(alignment: .leading, spacing: GridLayout.spacing) {
            ForEach(rows, id: \.first?.id) { row in
                HStack(alignment: .top, spacing: GridLayout.spacing) {
                    ForEach(row) { item in
                        ResumeTile(item: item, identifier: identifier, downloads: downloads?[item.book.id],
                                   width: cellWidth, remove: remove.map { remove in { remove(item) } })
                    }
                }
                .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(.horizontal, GridLayout.margin)
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $downloads) { db in
            try CatalogStore.booksDownloads(db)
        }
    }
}

/// Where a resume card's Downloads stand, under its title: a filling ring
/// while on the way, then Downloaded. Only a card downloading reads the
/// transfers' progress, so the rest aren't redrawn as bytes arrive.
private struct ResumeDownloadLine: View {
    @Environment(AppModel.self) private var model
    let downloads: BookDownloads?

    var body: some View {
        Group {
            if let downloads, downloads.isDownloading {
                let fraction = downloads.fraction(progress: model.session?.downloads.progress ?? [:])
                HStack(spacing: 4) {
                    CoverDownloadProgress(fraction: fraction)
                        .scaleEffect(0.7)
                        .frame(width: 13, height: 13)
                    // The word when it fits, else only the percent.
                    ViewThatFits(in: .horizontal) {
                        Text(fraction.map { String(localized: "Downloading \(Percent.text($0))") } ?? String(localized: "Downloading"))
                            .fixedSize()
                        Text(verbatim: fraction.map(Percent.text) ?? "")
                    }
                }
            } else if downloads?.hasDownloaded == true {
                // The word when it fits beside a narrow cover, else the mark.
                ViewThatFits(in: .horizontal) {
                    Label("Downloaded", icon: .arrowDownToLine, size: 10, relativeTo: .caption2)
                        .fixedSize()
                    Icon(.arrowDownToLine, size: 11, relativeTo: .caption2)
                }
            }
        }
        .font(.geist(.caption2))
        .foregroundStyle(Theme.mutedForeground)
        .lineLimit(1)
        .accessibilityHidden(true)
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
        .navigationBarTitleDisplayMode(.inline)
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
