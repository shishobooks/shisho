import SwiftUI

/// Every Book in the user's Libraries, read from the device's copy of the
/// catalog, so it works offline. It keeps itself current as syncs land.
struct LibraryView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.horizontalSizeClass) private var sizeClass
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Namespace private var zoom

    @AppStorage("libraryID") private var storedLibraryID = 0
    @State private var books: [BookSummary]?
    @State private var libraries: [LibrarySummary]?
    /// How far the content has scrolled up, which shrinks the logo.
    @State private var scrolledBy: CGFloat = 0
    @State private var width: CGFloat = 0
    @State private var showingAccount = false

    private static let margin: CGFloat = 16
    private static let columnSpacing: CGFloat = 10

    private var libraryID: Int? {
        storedLibraryID == 0 ? nil : storedLibraryID
    }

    /// Three columns on a phone, two at accessibility text sizes, and as
    /// many as fit at about 150 points on an iPad.
    private var columns: Int {
        if dynamicTypeSize.isAccessibilitySize { return sizeClass == .regular ? 3 : 2 }
        guard sizeClass == .regular else { return 3 }
        return max(4, Int((width - 2 * Self.margin + Self.columnSpacing) / (150 + Self.columnSpacing)))
    }

    private var cellWidth: CGFloat {
        let n = CGFloat(columns)
        return max(0, ((width - 2 * Self.margin - Self.columnSpacing * (n - 1)) / n).rounded(.down))
    }

    private var libraryName: String {
        (libraries ?? []).first { $0.id == libraryID }?.name ?? String(localized: "All Libraries")
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    Color.clear.frame(height: 0)
                        .readsScrollOffset(in: "library")
                    if let libraries, libraries.count > 1 {
                        LibraryPicker(libraries: libraries, selection: $storedLibraryID, name: libraryName)
                            .padding(.horizontal, Self.margin)
                            .padding(.top, 8)
                    }
                    SyncBanner()
                        .padding(.horizontal, Self.margin)
                        .padding(.top, 14)
                    if let books {
                        sectionHeader(count: books.count)
                        if books.isEmpty {
                            emptyState
                        } else {
                            grid(books)
                        }
                    }
                }
                .padding(.bottom, 32)
            }
            .coordinateSpace(name: "library")
            .onPreferenceChange(ScrollOffsetKey.self) { offset in
                scrolledBy = max(0, -offset)
            }
            .onGeometryChange(for: CGFloat.self, of: \.size.width) { width = $0 }
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
                    .zoomDestination(id: book.id, in: zoom)
            }
            .sheet(isPresented: $showingAccount) {
                AccountSheet()
            }
        }
        .tint(Theme.primary)
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
        .padding(.horizontal, Self.margin)
        .padding(.top, 20)
        .padding(.bottom, 14)
    }

    private func grid(_ books: [BookSummary]) -> some View {
        let column = GridItem(.fixed(cellWidth), spacing: Self.columnSpacing, alignment: .top)
        return LazyVGrid(columns: Array(repeating: column, count: columns), spacing: 22) {
            ForEach(books) { book in
                NavigationLink(value: book) {
                    BookCell(book: book, width: cellWidth, zoom: zoom)
                }
                .buttonStyle(PressableStyle())
            }
        }
        .padding(.horizontal, Self.margin)
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
    let zoom: Namespace.ID

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            CoverView(book: book, width: width)
                .zoomSource(id: book.id, in: zoom)
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
                        Link(destination: Installation.updateURL) {
                            Label("Update the app", systemImage: "arrow.up.forward")
                                .font(.geist(.subheadline, weight: .semibold))
                        }
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
