import SwiftUI

extension ResourceKind {
    /// The kind's name as a screen's title.
    var title: String {
        switch self {
        case .series: String(localized: "Series")
        case .person: String(localized: "People")
        case .genre: String(localized: "Genres")
        case .tag: String(localized: "Tags")
        }
    }

    /// The web's navigation icon for it.
    var icon: Lucide {
        switch self {
        case .series: .layers
        case .person: .users
        case .genre: .bookmark
        case .tag: .tags
        }
    }

    /// The accessibility identifier's stem, shared with the Android app.
    var identifier: String { rawValue }
}

/// The ways into the Library besides its grid: its Series, People, Genres,
/// and Tags, as the web app's navigation lists them, in its small outlined
/// buttons.
struct BrowseButtons: View {
    var body: some View {
        ForEach(ResourceKind.allCases, id: \.self) { kind in
            NavigationLink(value: Route.browse(kind)) {
                Label(verbatim: kind.title, icon: kind.icon, size: 15, relativeTo: .footnote)
            }
            .buttonStyle(SmallOutlineStyle())
            .accessibilityIdentifier("browse.\(kind.identifier)")
        }
    }
}

/// Every Series, Person, Genre, or Tag in the Library picked, by name,
/// with how many Books name each, filtered as the user types.
struct ResourceListView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.horizontalSizeClass) private var sizeClass
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let kind: ResourceKind

    @State private var items: [ResourceSummary]?
    @State private var filter = ""

    private var shown: [ResourceSummary] {
        let all = items ?? []
        let text = filter.trimmingCharacters(in: .whitespaces)
        guard !text.isEmpty else { return all }
        return all.filter { $0.name.localizedStandardContains(text) || $0.detail.localizedStandardContains(text) }
    }

    /// The web app's line under each list's title.
    private var subtitle: String {
        switch kind {
        case .series: String(localized: "Browse book series in your library")
        case .person: String(localized: "Authors, narrators, and other contributors")
        case .genre: String(localized: "Browse genres in your library")
        case .tag: String(localized: "Browse tags in your library")
        }
    }

    var body: some View {
        GeometryReader { geometry in
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    Text(subtitle)
                        .font(.geist(.subheadline))
                        .foregroundStyle(Theme.mutedForeground)
                        .padding(.horizontal, GridLayout.margin)
                        .padding(.bottom, 16)
                    if let items, items.isEmpty {
                        EmptyNotice(title: String(localized: "Nothing here yet"),
                                    message: String(localized: "\(kind.title) in your Libraries appear here."),
                                    icon: kind.icon)
                    } else if kind == .series {
                        seriesGrid(width: geometry.size.width)
                    } else {
                        LazyVStack(spacing: 8) {
                            ForEach(shown) { item in
                                NavigationLink(value: Route.resource(kind, id: item.id, name: item.name)) {
                                    ResourceRow(item: item, kind: kind)
                                }
                                .buttonStyle(PressableStyle())
                                .accessibilityIdentifier("browse.item")
                            }
                        }
                        .padding(.horizontal, GridLayout.margin)
                    }
                }
                .padding(.bottom, 32)
            }
            .miniPlayerClearance()
        }
        .background(Theme.background)
        .searchable(text: $filter, prompt: Text("Search \(kind.title.lowercased())"))
        .navigationTitle(kind.title)
        .navigationBarTitleDisplayMode(.large)
        .refreshable { await model.syncNow() }
        .observing(model.session?.store, id: "\(model.session?.account.id ?? "")-\(kind)-\(model.libraryID)", into: $items) { [kind, libraryID = model.libraryID == 0 ? nil : model.libraryID] db in
            try CatalogStore.resources(db, kind: kind, libraryID: libraryID)
        }
    }

    /// Series as the web app shows them: a grid of their first covers.
    private func seriesGrid(width: CGFloat) -> some View {
        let layout = GridLayout(width: width, regular: sizeClass == .regular,
                                accessibility: dynamicTypeSize.isAccessibilitySize)
        let column = GridItem(.fixed(layout.cellWidth), spacing: GridLayout.spacing, alignment: .top)
        return LazyVGrid(columns: Array(repeating: column, count: layout.columns), spacing: 22) {
            ForEach(shown) { item in
                NavigationLink(value: Route.resource(.series, id: item.id, name: item.name)) {
                    SeriesCard(item: item, width: layout.cellWidth)
                }
                .buttonStyle(PressableStyle())
                .accessibilityIdentifier("browse.item")
            }
        }
        .padding(.horizontal, GridLayout.margin)
    }
}

/// One Series, Person, Genre, or Tag: its name, description, and other
/// names, and its Books. A Series' are in its order with their numbers; a
/// Person's are the Books they wrote and the ones they narrated.
struct ResourceView: View {
    @Environment(AppModel.self) private var model
    let kind: ResourceKind
    let id: Int
    let initialName: String

    @State private var detail: ResourceDetail??
    @State private var books: [BookSummary]?
    @State private var narrated: [BookSummary]?
    @State private var numbers: SeriesNumberMap?

    private var name: String { (detail ?? nil)?.name ?? initialName }

    var body: some View {
        GeometryReader { geometry in
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    if case .some(.none) = detail {
                        EmptyNotice(title: String(localized: "This isn't in your Libraries anymore"), icon: kind.icon)
                            .padding(.top, 80)
                    } else {
                        header
                        if let books {
                            if kind == .person {
                                if !books.isEmpty {
                                    SectionHeading(title: String(localized: "Books Authored"))
                                    BookGrid(books: books, width: geometry.size.width, identifier: "resource.grid")
                                }
                                if let narrated, !narrated.isEmpty {
                                    SectionHeading(title: String(localized: "Books Narrated"))
                                    BookGrid(books: narrated, width: geometry.size.width, identifier: "resource.narrated")
                                }
                            } else {
                                SectionHeading(title: kind == .series ? String(localized: "Books in Series") : String(localized: "Books"))
                                BookGrid(books: books, width: geometry.size.width, badges: numbers?.values ?? [:], identifier: "resource.grid")
                            }
                        }
                    }
                }
                .padding(.bottom, 32)
            }
            .miniPlayerClearance()
        }
        .background(Theme.background)
        .refreshable { await model.syncNow() }
        .navigationTitle(name)
        .navigationBarTitleDisplayMode(.large)
        .observing(model.session?.store, id: "\(kind)-\(id)", into: $detail) { [kind, id] db in
            try CatalogStore.resource(db, kind: kind, id: id)
        }
        .observing(model.session?.store, id: "\(kind)-\(id)", into: $books) { [kind, id] db in
            try CatalogStore.books(db, kind: kind, id: id)
        }
        .observing(model.session?.store, id: "\(kind)-\(id)", into: $narrated) { [kind, id] db in
            kind == .person ? try CatalogStore.books(db, kind: .person, id: id, narrated: true) : []
        }
        .observing(model.session?.store, id: "\(kind)-\(id)", into: $numbers) { [kind, id] db in
            kind == .series ? SeriesNumberMap(values: try CatalogStore.seriesNumbers(db, seriesID: id)) : SeriesNumberMap(values: [:])
        }
    }

    @ViewBuilder
    private var header: some View {
        if let detail = detail ?? nil {
            VStack(alignment: .leading, spacing: 10) {
                // Its counts, as the web app badges them under the name.
                HStack(spacing: 6) {
                    ForEach(counts, id: \.self) { CountBadge(text: $0) }
                }
                .accessibilityElement(children: .combine)
                if let description = detail.description, !description.isEmpty {
                    ExpandableText(text: PlainText.from(description), lineLimit: 4, identifier: "resource.description")
                        .font(.geist(.subheadline))
                }
                if !detail.aliases.isEmpty {
                    Text("Also known as \(detail.aliases.joined(separator: ", "))")
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                }
            }
            .padding(.horizontal, GridLayout.margin)
            .padding(.top, 4)
        }
    }

    private var counts: [String] {
        let authored = books?.count ?? 0
        guard kind == .person else { return [Counts.books(authored)] }
        var out: [String] = []
        if authored > 0 {
            out.append(String(AttributedString(localized: "^[\(authored) book](inflect: true) authored").characters))
        }
        // Files, as the web and the People list count a narrator.
        if let files = (detail ?? nil)?.narratedFiles, files > 0 {
            out.append(String(AttributedString(localized: "^[\(files) file](inflect: true) narrated").characters))
        }
        return out
    }
}

/// Series Numbers by Book, comparable so a screen can observe them.
struct SeriesNumberMap: Equatable, Sendable {
    let values: [Int: (Double, Double?)]

    static func == (a: SeriesNumberMap, b: SeriesNumberMap) -> Bool {
        a.values.count == b.values.count && a.values.allSatisfy { key, value in
            guard let other = b.values[key] else { return false }
            return other.0 == value.0 && other.1 == value.1
        }
    }
}

/// Text a description holds, which may be HTML, as the reader sees it.
enum PlainText {
    static func from(_ text: String) -> String {
        guard text.contains("<") else { return text }
        var plain = text
        for (tag, replacement) in [("<br>", "\n"), ("<br/>", "\n"), ("<br />", "\n"), ("</p>", "\n\n"), ("</li>", "\n")] {
            plain = plain.replacingOccurrences(of: tag, with: replacement, options: .caseInsensitive)
        }
        plain = plain.replacingOccurrences(of: "<[^>]+>", with: "", options: .regularExpression)
        for (entity, character) in [("&amp;", "&"), ("&lt;", "<"), ("&gt;", ">"), ("&quot;", "\""), ("&#39;", "'"), ("&nbsp;", " ")] {
            plain = plain.replacingOccurrences(of: entity, with: character)
        }
        return plain.trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

/// Expands or collapses a long description.
struct ShowMoreButton: View {
    @Binding var expanded: Bool
    let identifier: String

    var body: some View {
        Button(expanded ? String(localized: "Show less") : String(localized: "Show more")) {
            withAnimation(.easeInOut(duration: 0.25)) { expanded.toggle() }
        }
        .font(.geist(.subheadline, weight: .semibold))
        .foregroundStyle(Theme.primary)
        .accessibilityIdentifier(identifier)
    }
}
