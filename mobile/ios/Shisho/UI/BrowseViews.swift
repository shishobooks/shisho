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

    var systemImage: String {
        switch self {
        case .series: "square.stack.3d.up"
        case .person: "person.2"
        case .genre: "theatermasks"
        case .tag: "tag"
        }
    }

    /// The accessibility identifier's stem, shared with the Android app.
    var identifier: String { rawValue }
}

/// The ways into the Library besides its grid: its Series, People, Genres,
/// and Tags, as the web app's sidebar lists them.
struct BrowseRow: View {
    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(ResourceKind.allCases, id: \.self) { kind in
                    NavigationLink(value: Route.browse(kind)) {
                        Label(kind.title, systemImage: kind.systemImage)
                            .font(.geist(.subheadline, weight: .medium))
                            .foregroundStyle(Theme.foreground)
                            .padding(.horizontal, 14)
                            .frame(minHeight: 38)
                            .cardStyle(radius: 19)
                    }
                    .buttonStyle(PressableStyle())
                    .accessibilityIdentifier("browse.\(kind.identifier)")
                }
            }
            .padding(.horizontal, GridLayout.margin)
        }
    }
}

/// Every Series, Person, Genre, or Tag in the Library picked, by name,
/// with how many Books name each, filtered as the user types.
struct ResourceListView: View {
    @Environment(AppModel.self) private var model
    @AppStorage("libraryID") private var storedLibraryID = 0
    let kind: ResourceKind

    @State private var items: [ResourceSummary]?
    @State private var filter = ""

    private var shown: [ResourceSummary] {
        let all = items ?? []
        let text = filter.trimmingCharacters(in: .whitespaces)
        guard !text.isEmpty else { return all }
        return all.filter { $0.name.localizedStandardContains(text) }
    }

    var body: some View {
        List {
            ForEach(shown) { item in
                NavigationLink(value: Route.resource(kind, id: item.id, name: item.name)) {
                    HStack {
                        Text(item.name)
                            .font(.geist(.body))
                            .foregroundStyle(Theme.foreground)
                        Spacer()
                        Text("\(item.bookCount)")
                            .font(.geist(.subheadline))
                            .foregroundStyle(Theme.mutedForeground)
                            .monospacedDigit()
                    }
                    .accessibilityElement(children: .ignore)
                    .accessibilityLabel(String(localized: "\(item.name), ^[\(item.bookCount) book](inflect: true)"))
                }
                .listRowBackground(Theme.card)
                .accessibilityIdentifier("browse.item")
            }
        }
        .overlay {
            if let items, items.isEmpty {
                EmptyNotice(title: String(localized: "Nothing here yet"),
                            message: String(localized: "\(kind.title) in your Libraries appear here."),
                            systemImage: kind.systemImage)
            }
        }
        .scrollContentBackground(.hidden)
        .background(Theme.background)
        .searchable(text: $filter, prompt: Text("Filter \(kind.title)"))
        .navigationTitle(kind.title)
        .navigationBarTitleDisplayMode(.large)
        .miniPlayerClearance()
        .refreshable { await model.syncNow() }
        .observing(model.session?.store, id: "\(kind)-\(storedLibraryID)", into: $items) { [kind, libraryID = storedLibraryID == 0 ? nil : storedLibraryID] db in
            try CatalogStore.resources(db, kind: kind, libraryID: libraryID)
        }
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
    @State private var showingFullDescription = false

    private var name: String { (detail ?? nil)?.name ?? initialName }

    var body: some View {
        GeometryReader { geometry in
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    if case .some(.none) = detail {
                        ContentUnavailableView(String(localized: "This isn't in your Libraries anymore"), systemImage: kind.systemImage)
                            .padding(.top, 80)
                    } else {
                        header
                        if let books {
                            if kind == .person {
                                if !books.isEmpty {
                                    SectionHeading(title: String(localized: "Books"), count: String(localized: "^[\(books.count) book](inflect: true)"))
                                    BookGrid(books: books, width: geometry.size.width, identifier: "resource.grid")
                                }
                                if let narrated, !narrated.isEmpty {
                                    SectionHeading(title: String(localized: "Narrated"), count: String(localized: "^[\(narrated.count) book](inflect: true)"))
                                    BookGrid(books: narrated, width: geometry.size.width, identifier: "resource.narrated")
                                }
                            } else {
                                SectionHeading(title: String(localized: "Books"), count: String(localized: "^[\(books.count) book](inflect: true)"))
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
        if let detail = detail ?? nil, detail.description?.isEmpty == false || !detail.aliases.isEmpty {
            VStack(alignment: .leading, spacing: 10) {
                if let description = detail.description, !description.isEmpty {
                    Text(PlainText.from(description))
                        .font(.geist(.subheadline))
                        .lineLimit(showingFullDescription ? nil : 4)
                        .onTapGesture { withAnimation { showingFullDescription.toggle() } }
                        .accessibilityIdentifier("resource.description")
                }
                if !detail.aliases.isEmpty {
                    Text("Also known as \(detail.aliases.joined(separator: ", "))")
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                }
            }
            .padding(.horizontal, GridLayout.margin)
            .padding(.top, 8)
        }
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
