import GRDB
import SwiftUI

/// Searches the device's copy of the catalog, so it works offline, with the
/// server's own way of matching and ranking (`CatalogStore.search`). It
/// opens from a tab's toolbar, as the prototype's search button does, with
/// its field ready to type in, and lists what it finds as the web app's
/// search does: Books, then Series, People, Genres, and Tags.
struct SearchView: View {
    @Environment(AppModel.self) private var model
    @State private var text = ""
    @State private var results = SearchResults()
    @State private var searching = true

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 0) {
                if !results.books.isEmpty {
                    SectionHeading(title: String(localized: "Books"))
                    VStack(spacing: 8) {
                        ForEach(results.books) { book in
                            NavigationLink(value: Route.book(book)) {
                                SearchBookRow(book: book)
                            }
                            .buttonStyle(PressableStyle())
                            .accessibilityIdentifier("search.book")
                        }
                    }
                    .padding(.horizontal, GridLayout.margin)
                }
                section(String(localized: "Series"), .series, results.series)
                section(String(localized: "People"), .person, results.persons)
                section(String(localized: "Genres"), .genre, results.genres)
                section(String(localized: "Tags"), .tag, results.tags)
            }
            .padding(.bottom, 32)
            .frame(maxWidth: 680)
            .frame(maxWidth: .infinity)
        }
        .background(Theme.background)
        .overlay {
            if text.trimmingCharacters(in: .whitespaces).isEmpty {
                EmptyNotice(title: String(localized: "Search your Libraries"),
                            message: String(localized: "Find Books by title, author, narrator, series, or ISBN, and find series, people, genres, and tags. It works offline."),
                            icon: .search)
            } else if results.isEmpty {
                EmptyState(verbatim: String(localized: "No Results for “\(text)”"), icon: .search,
                           description: Text("Check the spelling or try a new search."))
                    .accessibilityIdentifier("search.none")
            }
        }
        .miniPlayerClearance()
        .navigationTitle("Search")
        .navigationBarTitleDisplayMode(.inline)
        .searchable(text: $text, isPresented: $searching, placement: .navigationBarDrawer(displayMode: .always),
                    prompt: Text("Books, series, people"))
        .autocorrectionDisabled()
        .textInputAutocapitalization(.never)
        .task(id: "\(model.session?.account.id ?? "")|\(model.libraryID)|\(text)") {
            await search()
        }
    }

    @ViewBuilder
    private func section(_ title: String, _ kind: ResourceKind, _ items: [ResourceSummary]) -> some View {
        if !items.isEmpty {
            SectionHeading(title: title)
            VStack(spacing: 8) {
                ForEach(items) { item in
                    NavigationLink(value: Route.resource(kind, id: item.id, name: item.name)) {
                        ResourceRow(item: item, kind: kind)
                    }
                    .buttonStyle(PressableStyle())
                    .accessibilityIdentifier("search.\(kind.identifier)")
                }
            }
            .padding(.horizontal, GridLayout.margin)
        }
    }

    /// Runs the search a moment after typing stops, and again whenever a
    /// sync changes what it read. It observes the store itself rather than
    /// through observing(...), which has no pause to wait out typing.
    private func search() async {
        let query = text
        guard let store = model.session?.store, CatalogStore.searchQuery(query) != nil else {
            results = SearchResults()
            return
        }
        try? await Task.sleep(for: .milliseconds(150))
        guard !Task.isCancelled else { return }
        let libraryID = model.libraryID == 0 ? nil : model.libraryID
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

/// A Book the search found, as the web app's search lists one: its cover,
/// title, and authors.
private struct SearchBookRow: View {
    let book: BookSummary

    var body: some View {
        HStack(spacing: 12) {
            CoverView(book: book, width: 40)
            VStack(alignment: .leading, spacing: 3) {
                Text(book.title)
                    .font(.geist(.callout, weight: .semibold))
                    .foregroundStyle(Theme.foreground)
                    .lineLimit(2)
                    .multilineTextAlignment(.leading)
                if !book.authors.isEmpty {
                    Text(book.authors)
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                        .lineLimit(1)
                }
            }
            Spacer(minLength: 8)
            HStack(spacing: 4) {
                ForEach(book.fileTypes.prefix(2), id: \.self) { FileTypeChip(fileType: $0) }
            }
            .accessibilityHidden(true)
        }
        .padding(10)
        .cardStyle(radius: Theme.radius + 2)
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
    }
}
