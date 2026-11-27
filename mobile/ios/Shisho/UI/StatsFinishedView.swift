import SwiftUI

/// The finished Books a Reading Stats breakdown row counts, as the web's
/// Finished section lists them for a filter chip: each finish in the range,
/// rereads apart, the latest first, from the device's own copy.
struct StatsFinishedView: View {
    @Environment(AppModel.self) private var model
    let filter: StatsFinishedFilter
    /// The range Reading Stats showed, or nil for all time.
    let year: Int?
    @State private var entries: [HistoryEntry]?

    var body: some View {
        Group {
            if let entries, entries.isEmpty {
                // In a scroll view, so it can still be pulled to refresh.
                ScrollView {
                    header
                        .padding(.horizontal, GridLayout.margin)
                    EmptyNotice(title: String(localized: "No finished books"),
                                message: String(localized: "Finished books show here once they're on this device."), icon: .history)
                }
                .accessibilityIdentifier("stats.finishedPage")
            } else {
                List {
                    header
                        .listRowBackground(Theme.background)
                        .listRowSeparator(.hidden)
                    ForEach(entries ?? []) { entry in
                        Button {
                            if let book = entry.book { model.push(.book(book)) }
                        } label: {
                            HistoryRow(entry: entry)
                        }
                        .buttonStyle(.plain)
                        .disabled(entry.book == nil)
                        .listRowBackground(Theme.background)
                        .accessibilityIdentifier("stats.finishedPage.row")
                    }
                }
                .listStyle(.plain)
                .scrollContentBackground(.hidden)
                .miniPlayerClearance()
                .accessibilityIdentifier("stats.finishedPage")
            }
        }
        .background(Theme.background)
        .refreshable { await model.syncNow() }
        .navigationTitle(StatsFinishedText.title(filter))
        .navigationBarTitleDisplayMode(.inline)
        .observing(model.session?.store, id: "\(filter)-\(year ?? 0)", into: $entries) { [filter, year] db in
            try CatalogStore.statsFinished(db, year: year, filter: filter)
        }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(StatsFinishedText.range(year))
                .font(.geist(.subheadline))
                .foregroundStyle(Theme.mutedForeground)
            if let entries {
                Text(StatsFinishedText.count(entries))
                    .font(.geist(.footnote, weight: .medium))
                    .monospacedDigit()
                    .foregroundStyle(Theme.foreground)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 4)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("stats.finishedPage.summary")
    }
}

/// How the Finished page names its filter and range.
enum StatsFinishedText {
    /// How many Books the page lists, as the breakdown row counts them, each
    /// once, with its finishes too when some Books were finished more than
    /// once: "2 books · 3 finishes".
    static func count(_ entries: [HistoryEntry]) -> String {
        // A Book that isn't on the device counts by its Read-through.
        let books = Set(entries.map { $0.book.map { "book-\($0.id)" } ?? "read-through-\($0.id)" }).count
        guard entries.count > books else { return Counts.books(books) }
        let finishes = String(AttributedString(localized: "^[\(entries.count) finish](inflect: true)").characters)
        return "\(Counts.books(books)) · \(finishes)"
    }

    static func title(_ filter: StatsFinishedFilter) -> String {
        switch filter {
        case .genre(let name), .tag(let name), .author(let name): name
        case .rating(let stars): StatsText.stars(stars)
        }
    }

    static func range(_ year: Int?) -> String {
        guard let year else { return String(localized: "Books you finished, all time") }
        return String(localized: "Books you finished in \(String(year))")
    }
}
