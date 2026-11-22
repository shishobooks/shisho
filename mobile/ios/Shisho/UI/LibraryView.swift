import SwiftUI

/// Every Book in the user's Libraries, read from the device's copy of the
/// catalog, so it works offline. It keeps itself current as syncs land.
struct LibraryView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.horizontalSizeClass) private var sizeClass
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    @AppStorage("libraryID") private var storedLibraryID = 0
    @State private var books: [BookSummary]?
    @State private var libraries: [LibrarySummary]?
    /// How far the content has scrolled up, which shrinks the logo.
    @State private var scrolledBy: CGFloat = 0
    @State private var showingAccount = false
    @State private var notes: [JournalNote]?

    private var libraryID: Int? {
        storedLibraryID == 0 ? nil : storedLibraryID
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
                        if let libraries, libraries.count > 1 {
                            LibraryPicker(libraries: libraries, selection: $storedLibraryID, name: libraryName)
                                .padding(.horizontal, GridLayout.margin)
                                .padding(.top, 8)
                        }
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
                                grid(books, width: geometry.size.width)
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
                ToolbarItem(placement: .topBarTrailing) {
                    AccountButton { showingAccount = true }
                }
            }
            .navigationDestination(for: BookSummary.self) { book in
                BookDetailView(bookID: book.id, initial: book)
            }
            .sheet(isPresented: $showingAccount) {
                AccountSheet()
            }
        }
        // Outside the stack, so the mini player stays on every pushed screen.
        // Each screen's scroll view leaves room for it (miniPlayerClearance).
        .overlay(alignment: .bottom) {
            MiniPlayer()
                .frame(maxWidth: 680)
                .animation(.spring(response: 0.4, dampingFraction: 0.85), value: model.player?.file?.fileID)
        }
        .tint(Theme.primary)
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $notes) { db in
            try CatalogStore.journalNotes(db)
        }
        .observing(model.session?.store, id: "\(model.session?.account.id ?? "")-\(storedLibraryID)", into: $books) { [libraryID] db in
            try CatalogStore.books(db, libraryID: libraryID)
        }
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $libraries) { db in
            try CatalogStore.libraries(db)
        }
        .onChange(of: libraries) { _, libraries in
            if let libraries, libraryID != nil, !libraries.contains(where: { $0.id == libraryID }) {
                storedLibraryID = 0
            }
        }
    }

    private func sectionHeader(count: Int) -> some View {
        HStack(alignment: .firstTextBaseline) {
            Text("Library")
                .font(.geist(.title2, weight: .semibold))
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

    private func grid(_ books: [BookSummary], width: CGFloat) -> some View {
        let layout = GridLayout(width: width, regular: sizeClass == .regular,
                                accessibility: dynamicTypeSize.isAccessibilitySize)
        let cellWidth = layout.cellWidth
        let column = GridItem(.fixed(cellWidth), spacing: GridLayout.spacing, alignment: .top)
        return LazyVGrid(columns: Array(repeating: column, count: layout.columns), spacing: 22) {
            ForEach(books) { book in
                NavigationLink(value: book) {
                    BookCell(book: book, width: cellWidth)
                }
                .buttonStyle(PressableStyle())
            }
        }
        .padding(.horizontal, GridLayout.margin)
        .accessibilityIdentifier("library.grid")
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
            HStack(spacing: 8) {
                Image(systemName: "books.vertical")
                    .foregroundStyle(Theme.mutedForeground)
                Text(name)
                    .foregroundStyle(Theme.foreground)
                    .lineLimit(1)
                Image(systemName: "chevron.down")
                    .font(.geist(.caption, weight: .semibold))
                    .foregroundStyle(Theme.mutedForeground)
            }
            .font(.geist(.subheadline, weight: .medium))
            .padding(.horizontal, 14)
            .frame(minHeight: 40)
            .cardStyle(radius: 10)
        }
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
private struct AccountButton: View {
    @Environment(AppModel.self) private var model
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            let initial = Text(String(model.session?.account.username.prefix(1).uppercased() ?? "?"))
                .font(.geist(.subheadline, weight: .semibold))
                .foregroundStyle(Theme.primary)
            if #available(iOS 26, *) {
                // The toolbar already sets the button in a glass circle.
                initial
            } else {
                initial
                    .frame(width: 34, height: 34)
                    .background(Theme.primary.opacity(0.12), in: Circle())
                    .overlay(Circle().strokeBorder(Theme.primary.opacity(0.25)))
            }
        }
        .accessibilityLabel("Account")
        .accessibilityIdentifier("account.button")
    }
}

struct BookCell: View {
    let book: BookSummary
    let width: CGFloat

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            CoverView(book: book, width: width)
                .padding(.bottom, 4)
            Text(book.title)
                .font(.geist(.subheadline, weight: .semibold))
                .foregroundStyle(Theme.foreground)
                .lineLimit(2)
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

    private var accessibilityText: String {
        var text = book.title
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
