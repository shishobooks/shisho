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
    @State private var scrolled = false
    @State private var showingAccount = false

    private var libraryID: Int? {
        storedLibraryID == 0 ? nil : storedLibraryID
    }

    private var cellWidth: CGFloat {
        let base: CGFloat = sizeClass == .regular ? 156 : 104
        return dynamicTypeSize.isAccessibilitySize ? base * 1.45 : base
    }

    private var libraryName: String {
        (libraries ?? []).first { $0.id == libraryID }?.name ?? String(localized: "All Libraries")
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    header
                    SyncBanner()
                        .padding(.horizontal, 16)
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
                let collapsed = offset < -36
                if collapsed != scrolled {
                    withAnimation(.easeOut(duration: 0.2)) { scrolled = collapsed }
                }
            }
            .background(Theme.background)
            .refreshable { await model.syncNow() }
            .navigationBarTitleDisplayMode(.inline)
            .toolbarBackground(scrolled ? .visible : .hidden, for: .navigationBar)
            .toolbar {
                ToolbarItem(placement: .principal) {
                    Wordmark(size: 17)
                        .opacity(scrolled ? 1 : 0)
                        .accessibilityHidden(!scrolled)
                }
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

    private var header: some View {
        VStack(alignment: .leading, spacing: 18) {
            Wordmark(size: 32)
                .accessibilityAddTraits(.isHeader)
            if let libraries, libraries.count > 1 {
                LibraryPicker(libraries: libraries, selection: $storedLibraryID, name: libraryName)
            }
        }
        .padding(.horizontal, 16)
        .padding(.top, 4)
        .readsScrollOffset(in: "library")
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
        .padding(.horizontal, 16)
        .padding(.top, 26)
        .padding(.bottom, 14)
    }

    private func grid(_ books: [BookSummary]) -> some View {
        LazyVGrid(columns: [GridItem(.adaptive(minimum: cellWidth, maximum: cellWidth * 1.4), spacing: 14, alignment: .top)], spacing: 26) {
            ForEach(books) { book in
                NavigationLink(value: book) {
                    BookCell(book: book, width: cellWidth, zoom: zoom)
                }
                .buttonStyle(PressableStyle())
            }
        }
        .padding(.horizontal, 16)
        .accessibilityIdentifier("library.grid")
    }

    private var emptyState: some View {
        VStack(spacing: 10) {
            if model.syncStatus == .syncing {
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
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                if let number = book.seriesNumber {
                    SeriesNumberBadge(number: number)
                }
                Text(book.title)
                    .font(.geist(.subheadline, weight: .semibold))
                    .foregroundStyle(Theme.foreground)
                    .lineLimit(2)
                    .multilineTextAlignment(.leading)
                    .accessibilityIdentifier("book.title")
            }
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
        if let series = book.seriesName {
            if let number = book.seriesNumber {
                text += ", " + String(localized: "\(series) number \(SeriesNumbers.format(number))")
            } else {
                text += ", \(series)"
            }
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
            case .idle, .syncing:
                EmptyView()
            case .offline:
                NoticeCard(text: String(localized: "Offline. Showing what's on this device."), systemImage: "wifi.slash")
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
