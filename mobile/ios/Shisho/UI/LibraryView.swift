import SwiftUI

/// Every Book in the user's Libraries, read from the device's copy of the
/// catalog, so it works offline. It keeps itself current as syncs land.
struct LibraryView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.horizontalSizeClass) private var sizeClass
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    @State private var books: [BookSummary]?
    @State private var libraries: [LibrarySummary]?
    /// How far the content has scrolled up, which shrinks the logo.
    @State private var scrolledBy: CGFloat = 0
    @State private var notes: [JournalNote]?

    private var libraryID: Int? {
        model.libraryID == 0 ? nil : model.libraryID
    }


    private var libraryName: String {
        (libraries ?? []).first { $0.id == libraryID }?.name ?? String(localized: "All Libraries")
    }

    var body: some View {
        @Bindable var model = model
        NavigationStack(path: $model.libraryPath) {
            // The width comes from outside the scroll view, so a grid laid
            // out for a wider screen can't hold it wide after a rotation.
            GeometryReader { geometry in
                ScrollView {
                    VStack(alignment: .leading, spacing: 0) {
                        ScrollTopMarker(space: "library")
                        // The Library picked, as the prototype has it under
                        // the title, and the ways into it besides the grid.
                        FlowLayout(spacing: 8) {
                            if let libraries, libraries.count > 1 {
                                LibraryPicker(libraries: libraries, selection: $model.libraryID, name: libraryName)
                            }
                            BrowseButtons()
                        }
                        .padding(.horizontal, GridLayout.margin)
                        .padding(.top, 8)
                        SyncBanner()
                            .padding(.horizontal, GridLayout.margin)
                            .padding(.top, 14)
                        if let notes, !notes.isEmpty {
                            JournalNotesBanner(notes: notes) { model.dismissJournalNotes() }
                                .padding(.horizontal, GridLayout.margin)
                                .padding(.top, 10)
                        }
                        if let books {
                            sectionHeader(count: books.count)
                            if books.isEmpty {
                                emptyState
                            } else {
                                BookGrid(books: books, width: geometry.size.width)
                            }
                        }
                    }
                    .padding(.bottom, 32)
                }
                .onScrolledDistanceChange(space: "library") { scrolledBy = $0 }
                .miniPlayerClearance()
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
        .tint(Theme.primary)
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $notes) { db in
            try CatalogStore.journalNotes(db)
        }
        .observing(model.session?.store, id: "\(model.session?.account.id ?? "")-\(model.libraryID)", into: $books) { [libraryID] db in
            try CatalogStore.books(db, libraryID: libraryID)
        }
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $libraries) { db in
            try CatalogStore.libraries(db)
        }
        .onChange(of: libraries) { _, libraries in
            if let libraries, libraryID != nil, !libraries.contains(where: { $0.id == libraryID }) {
                model.libraryID = 0
            }
        }
    }

    private func sectionHeader(count: Int) -> some View {
        HStack(alignment: .firstTextBaseline) {
            Text("Library")
                .font(.geist(.title3, weight: .semibold))
                .accessibilityAddTraits(.isHeader)
            Spacer()
            Text("^[\(count) book](inflect: true)")
                .font(.geist(.subheadline))
                .foregroundStyle(Theme.mutedForeground)
                .contentTransition(.numericText())
        }
        .padding(.horizontal, GridLayout.margin)
        .padding(.top, 20)
        .padding(.bottom, 14)
    }

    private var emptyState: some View {
        VStack(spacing: 10) {
            if model.isSyncing {
                ProgressView()
                    .controlSize(.large)
                Text("Downloading your library…")
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
            } else {
                Image(systemName: "books.vertical")
                    .font(.system(size: 40, weight: .light))
                    .foregroundStyle(Theme.primary)
                    .padding(.bottom, 4)
                Text("No Books yet")
                    .font(.geist(.headline, weight: .semibold))
                Text("Books in the Libraries you can access appear here.")
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
                    .multilineTextAlignment(.center)
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 56)
        .padding(.horizontal, 32)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("library.empty")
    }
}

/// The grid's columns for a width: as many as fit at about 110 points each
/// on a phone, or 140 on an iPad, and half again at accessibility text
/// sizes; never fewer than three, or two at those sizes. A phone shows three
/// upright and about six on its side.
struct GridLayout {
    static let margin: CGFloat = 16
    static let spacing: CGFloat = 10

    let columns: Int
    let cellWidth: CGFloat

    init(width: CGFloat, regular: Bool, accessibility: Bool) {
        let target: CGFloat = (regular ? 140 : 110) * (accessibility ? 1.5 : 1)
        let available = width - 2 * Self.margin
        let fits = Int((available + Self.spacing) / (target + Self.spacing))
        columns = max(accessibility ? 2 : 3, fits)
        let n = CGFloat(columns)
        cellWidth = max(0, ((available - Self.spacing * (n - 1)) / n).rounded(.down))
    }
}

/// Which Library the grid shows, as the web app's library picker does.
private struct LibraryPicker: View {
    let libraries: [LibrarySummary]
    @Binding var selection: Int
    let name: String

    var body: some View {
        Menu {
            Picker("Library", selection: $selection) {
                Label("All Libraries", systemImage: "square.stack").tag(0)
                ForEach(libraries) { library in
                    Text(library.name).tag(library.id)
                }
            }
        } label: {
            HStack(spacing: 6) {
                Image(systemName: "books.vertical")
                    .accessibilityHidden(true)
                Text(name)
                    .lineLimit(1)
                Image(systemName: "chevron.down")
                    .font(.geist(.caption2, weight: .semibold))
                    .foregroundStyle(Theme.mutedForeground)
            }
        }
        .buttonStyle(SmallOutlineStyle())
        .accessibilityLabel("Library: \(name)")
        .accessibilityIdentifier("library.picker")
    }
}

/// The logo at the top left, shrinking a little as the Library scrolls.
private struct LogoToolbarItem: ToolbarContent {
    let scrolledBy: CGFloat

    private var scale: CGFloat {
        1 - 0.18 * min(1, scrolledBy / 48)
    }

    var body: some ToolbarContent {
        if #available(iOS 26, *) {
            ToolbarItem(placement: .topBarLeading) { logo }
                .sharedBackgroundVisibility(.hidden)
        } else {
            ToolbarItem(placement: .topBarLeading) { logo }
        }
    }

    private var logo: some View {
        Wordmark(size: 26)
            .fixedSize()
            .scaleEffect(scale, anchor: .leading)
            .accessibilityAddTraits(.isHeader)
    }
}

/// The signed-in account, top right, as the user's initial.
struct AccountButton: View {
    @Environment(AppModel.self) private var model
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            if let account = model.session?.account {
                AccountAvatar(account: account, size: 34, inToolbar: true)
            }
        }
        .accessibilityLabel("Account")
        .accessibilityIdentifier("account.button")
    }
}

struct BookCell: View {
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let book: BookSummary
    let width: CGFloat
    /// The Book's Series Number, on a Series' page, inline with the title so
    /// the title wraps under it, as the web gallery shows it.
    var seriesNumber: (Double, Double?)?

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            CoverView(book: book, width: width)
                .padding(.bottom, 4)
            titleText
                .font(.geist(.subheadline, weight: .semibold))
                .foregroundStyle(Theme.foreground)
                // Larger text fits fewer words on a line, so it gets more lines.
                .lineLimit(dynamicTypeSize.isAccessibilitySize ? 5 : 2)
                .multilineTextAlignment(.leading)
                .accessibilityIdentifier("book.title")
            if !book.authors.isEmpty {
                Text(book.authors)
                    .font(.geist(.footnote))
                    .foregroundStyle(Theme.mutedForeground)
                    .lineLimit(1)
            }
            if !book.fileTypes.isEmpty {
                HStack(spacing: 4) {
                    ForEach(book.fileTypes.prefix(3), id: \.self) { FileTypeChip(fileType: $0) }
                }
                .padding(.top, 2)
                .accessibilityHidden(true)
            }
        }
        .frame(width: width, alignment: .leading)
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
        .accessibilityLabel(accessibilityText)
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("book.cell")
    }

    @Environment(\.displayScale) private var displayScale

    /// The title, after its Series Number's badge on a Series' page. The
    /// badge is drawn into the text as an image, so the title wraps under
    /// it rather than beside it.
    private var titleText: Text {
        guard let (number, end) = seriesNumber,
              let badge = BadgeImages.image(number: number, end: end, scale: displayScale, size: dynamicTypeSize)
        else { return Text(book.title) }
        return Text(Image(uiImage: badge)).baselineOffset(-3) + Text(verbatim: " ") + Text(book.title)
    }

    private var accessibilityText: String {
        var text = book.title
        if let (number, _) = seriesNumber {
            text = String(localized: "Number \(SeriesNumbers.format(number)), \(text)")
        }
        if !book.authors.isEmpty {
            text += ", " + String(localized: "by \(book.authors)")
        }
        return text
    }
}

/// A tap target that gives under the finger, as native grids do.
struct PressableStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .scaleEffect(configuration.isPressed ? 0.96 : 1)
            .opacity(configuration.isPressed ? 0.85 : 1)
            .animation(.spring(response: 0.25, dampingFraction: 0.7), value: configuration.isPressed)
    }
}

/// Says when the device's copy can't be brought up to date, and why.
struct SyncBanner: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        Group {
            switch model.syncStatus {
            case .idle:
                EmptyView()
            case .offline:
                NoticeCard(text: String(localized: "Offline. Showing what's on this device."), systemImage: "wifi.slash")
            case .localNetworkDenied:
                NoticeCard(text: Messages.localNetworkDenied, systemImage: "exclamationmark.triangle.fill", tint: Theme.destructive) {
                    SettingsLink()
                }
            case .versionGap(let message, let appTooOld):
                NoticeCard(text: message, systemImage: "exclamationmark.triangle.fill", tint: Theme.destructive) {
                    if appTooOld {
                        UpdateAppLink()
                    }
                }
            case .failed(let message):
                NoticeCard(text: message, systemImage: "exclamationmark.triangle.fill", tint: Theme.destructive)
            }
        }
        .transition(.move(edge: .top).combined(with: .opacity))
        .animation(.spring(response: 0.35, dampingFraction: 0.85), value: model.syncStatus)
        .accessibilityIdentifier("sync.banner")
    }
}

/// Changes made on this device that the server refused, such as listening
/// to a Book that has since been deleted. They're dropped rather than sent
/// forever (ADR 0024); this says so until the user dismisses it.
struct JournalNotesBanner: View {
    let notes: [JournalNote]
    let dismiss: () -> Void

    var body: some View {
        NoticeCard(
            text: notes.count == 1
                ? String(localized: "A change made on this device couldn't be saved on the server: \(notes[0].reason)")
                : String(localized: "\(notes.count) changes made on this device couldn't be saved on the server. The last: \(notes[notes.count - 1].reason)"),
            systemImage: "exclamationmark.bubble",
            tint: Theme.destructive
        ) {
            Button("Dismiss", action: dismiss)
                .font(.geist(.subheadline, weight: .semibold))
                .accessibilityIdentifier("journal.notes.dismiss")
        }
        .accessibilityIdentifier("journal.notes")
    }
}

/// Series Number badges drawn as images, for setting inline in text.
@MainActor
enum BadgeImages {
    private static var cache: [String: UIImage] = [:]

    static func image(number: Double, end: Double?, scale: CGFloat, size: DynamicTypeSize) -> UIImage? {
        let key = "\(number)-\(end.map { "\($0)" } ?? "")-\(scale)-\(size)"
        if let image = cache[key] {
            return image
        }
        let renderer = ImageRenderer(content: SeriesNumberBadge(number: number, end: end).environment(\.dynamicTypeSize, size))
        renderer.scale = scale
        let image = renderer.uiImage
        cache[key] = image
        return image
    }
}
