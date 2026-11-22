import GRDB
import SwiftUI

/// Searches the device's copy of the catalog, so it works offline, with the
/// server's own way of matching and ranking (`CatalogStore.search`).
struct SearchView: View {
    @Environment(AppModel.self) private var model
    @AppStorage("libraryID") private var storedLibraryID = 0
    @State private var text = ""
    @State private var results = SearchResults()

    var body: some View {
        @Bindable var model = model
        NavigationStack(path: $model.searchPath) {
            List {
                if !results.books.isEmpty {
                    Section("Books") {
                        ForEach(results.books) { book in
                            NavigationLink(value: Route.book(book)) {
                                SearchBookRow(book: book)
                            }
                            .listRowBackground(Theme.card)
                            .accessibilityIdentifier("search.book")
                        }
                    }
                }
                section(String(localized: "Series"), .series, results.series)
                section(String(localized: "People"), .person, results.persons)
                section(String(localized: "Genres"), .genre, results.genres)
                section(String(localized: "Tags"), .tag, results.tags)
            }
            .listStyle(.insetGrouped)
            .scrollContentBackground(.hidden)
            .background(Theme.background)
            .overlay {
                if text.trimmingCharacters(in: .whitespaces).isEmpty {
                    EmptyNotice(title: String(localized: "Search your Libraries"),
                                message: String(localized: "Find Books by title, author, narrator, or series, and find series, people, genres, and tags. It works offline."),
                                systemImage: "magnifyingglass")
                } else if results.isEmpty {
                    ContentUnavailableView.search(text: text)
                        .accessibilityIdentifier("search.none")
                }
            }
            .miniPlayerClearance()
            .navigationTitle("Search")
            .navigationBarTitleDisplayMode(.large)
            .searchable(text: $text, placement: .navigationBarDrawer(displayMode: .always), prompt: Text("Books, series, people"))
            .autocorrectionDisabled()
            .textInputAutocapitalization(.never)
            .routeDestinations()
            .task(id: "\(model.session?.account.id ?? "")|\(storedLibraryID)|\(text)") {
                await search()
            }
        }
    }

    @ViewBuilder
    private func section(_ title: String, _ kind: ResourceKind, _ items: [ResourceSummary]) -> some View {
        if !items.isEmpty {
            Section(title) {
                ForEach(items) { item in
                    NavigationLink(value: Route.resource(kind, id: item.id, name: item.name)) {
                        HStack {
                            Label(item.name, systemImage: kind.systemImage)
                                .font(.geist(.body))
                            Spacer()
                            Text("\(item.bookCount)")
                                .font(.geist(.subheadline))
                                .foregroundStyle(Theme.mutedForeground)
                        }
                    }
                    .listRowBackground(Theme.card)
                    .accessibilityIdentifier("search.\(kind.identifier)")
                }
            }
        }
    }

    /// Runs the search a moment after typing stops, and again whenever a
    /// sync changes what it read.
    private func search() async {
        let query = text
        guard let store = model.session?.store, CatalogStore.searchQuery(query) != nil else {
            results = SearchResults()
            return
        }
        try? await Task.sleep(for: .milliseconds(150))
        guard !Task.isCancelled else { return }
        let libraryID = storedLibraryID == 0 ? nil : storedLibraryID
        let observation = ValueObservation.tracking { db in
            try CatalogStore.search(db, text: query, libraryID: libraryID)
        }.removeDuplicates()
        do {
            for try await found in observation.values(in: store.pool) {
                results = found
            }
        } catch {
            // A search that fails leaves what was shown.
        }
    }
}

private struct SearchBookRow: View {
    let book: BookSummary

    var body: some View {
        HStack(spacing: 12) {
            CoverView(book: book, width: 40)
            VStack(alignment: .leading, spacing: 3) {
                Text(book.title)
                    .font(.geist(.body, weight: .semibold))
                    .lineLimit(2)
                if !book.authors.isEmpty {
                    Text(book.authors)
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                        .lineLimit(1)
                }
            }
        }
        .accessibilityElement(children: .combine)
    }
}
