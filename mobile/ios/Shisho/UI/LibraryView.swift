import SwiftUI

/// Every Book in the user's Libraries, read from the device's copy of the
/// catalog, so it works offline.
struct LibraryView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.horizontalSizeClass) private var sizeClass
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var libraryID: Int?
    @State private var books: [BookSummary] = []
    @State private var libraries: [LibrarySummary] = []

    private var cellWidth: CGFloat {
        let base: CGFloat = sizeClass == .regular ? 150 : 104
        return dynamicTypeSize.isAccessibilitySize ? base * 1.5 : base
    }

    private var title: String {
        libraries.first { $0.id == libraryID }?.name ?? String(localized: "Library")
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                SyncBanner()
                if books.isEmpty {
                    emptyState
                } else {
                    LazyVGrid(columns: [GridItem(.adaptive(minimum: cellWidth), spacing: 16, alignment: .top)], spacing: 20) {
                        ForEach(books) { book in
                            NavigationLink(value: book.id) {
                                BookCell(book: book, width: cellWidth)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.vertical, 8)
                    .accessibilityIdentifier("library.grid")
                }
            }
            .navigationTitle(title)
            .navigationDestination(for: Int.self) { id in
                BookDetailView(bookID: id)
            }
            .toolbar {
                if libraries.count > 1 {
                    ToolbarItem(placement: .topBarTrailing) {
                        Menu {
                            Picker("Library", selection: $libraryID) {
                                Text("All Libraries").tag(Int?.none)
                                ForEach(libraries) { library in
                                    Text(library.name).tag(Int?.some(library.id))
                                }
                            }
                        } label: {
                            Label("Choose Library", systemImage: "line.3.horizontal.decrease.circle")
                        }
                        .accessibilityIdentifier("library.picker")
                    }
                }
            }
            .refreshable { await model.syncNow() }
        }
        .task(id: "\(model.catalogRevision)-\(libraryID ?? -1)") { reload() }
    }

    private var emptyState: some View {
        VStack(spacing: 8) {
            if model.syncStatus == .syncing {
                ProgressView()
                Text("Downloading your library…")
            } else {
                Image(systemName: "books.vertical")
                    .font(.system(size: 40))
                    .foregroundStyle(Theme.mutedForeground)
                Text("No Books yet")
                    .font(.geist(.headline, weight: .semibold))
                Text("Books in the Libraries you can access appear here.")
                    .foregroundStyle(Theme.mutedForeground)
                    .multilineTextAlignment(.center)
            }
        }
        .font(.geist(.subheadline))
        .padding(.top, 80)
        .padding(.horizontal, 32)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("library.empty")
    }

    private func reload() {
        guard let store = model.session?.store else { return }
        libraries = (try? store.libraries()) ?? []
        if let libraryID, !libraries.contains(where: { $0.id == libraryID }) {
            self.libraryID = nil
        }
        books = (try? store.books(libraryID: libraryID)) ?? []
    }
}

struct BookCell: View {
    let book: BookSummary
    let width: CGFloat

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            CoverView(book: book, width: width)
            Text(book.title)
                .font(.geist(.footnote, weight: .semibold))
                .lineLimit(2)
                .accessibilityIdentifier("book.title")
            if !book.authors.isEmpty {
                Text(book.authors)
                    .font(.geist(.caption))
                    .foregroundStyle(Theme.mutedForeground)
                    .lineLimit(1)
            }
        }
        .frame(width: width, alignment: .leading)
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
        .accessibilityLabel(book.authors.isEmpty ? book.title : String(localized: "\(book.title), by \(book.authors)"))
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("book.cell")
    }
}

/// Says when the device's copy can't be brought up to date, and why.
struct SyncBanner: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        switch model.syncStatus {
        case .idle, .syncing:
            EmptyView()
        case .offline:
            banner(String(localized: "Offline. Showing what's on this device."), systemImage: "wifi.slash")
        case .versionGap(let message, let appTooOld):
            banner(message, systemImage: "arrow.triangle.2.circlepath", link: appTooOld)
        case .failed(let message):
            banner(message, systemImage: "exclamationmark.triangle")
        }
    }

    private func banner(_ text: String, systemImage: String, link: Bool = false) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Label(text, systemImage: systemImage)
            if link {
                Link("Update the app", destination: Installation.updateURL)
                    .font(.geist(.footnote, weight: .semibold))
            }
        }
        .font(.geist(.footnote))
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(12)
        .background(Theme.card, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).stroke(Theme.border))
        .padding(.horizontal, 16)
        .padding(.top, 4)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("sync.banner")
    }
}
