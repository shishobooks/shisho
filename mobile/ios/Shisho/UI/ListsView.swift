import SwiftUI

/// The user's Lists and the ones shared with them, read from the device.
struct ListsView: View {
    @Environment(AppModel.self) private var model
    @State private var lists: [ListSummary]?

    var body: some View {
        @Bindable var model = model
        NavigationStack(path: $model.listsPath) {
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    if let lists {
                        let mine = lists.filter { $0.permission == "owner" }
                        let shared = lists.filter { $0.permission != "owner" }
                        if lists.isEmpty {
                            EmptyNotice(title: String(localized: "No Lists yet"),
                                        message: String(localized: "Lists you make on the web, and Lists shared with you, appear here."),
                                        systemImage: "list.bullet.rectangle")
                                .accessibilityIdentifier("lists.empty")
                        }
                        if !mine.isEmpty {
                            SectionHeading(title: String(localized: "My Lists"))
                            rows(mine)
                        }
                        if !shared.isEmpty {
                            SectionHeading(title: String(localized: "Shared with Me"))
                            rows(shared)
                        }
                    }
                }
                .padding(.bottom, 32)
                .frame(maxWidth: 680)
                .frame(maxWidth: .infinity)
            }
            .miniPlayerClearance()
            .background(Theme.background)
            .refreshable { await model.syncNow() }
            .navigationTitle("Lists")
            .navigationBarTitleDisplayMode(.large)
            .routeDestinations()
        }
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $lists) { db in
            try CatalogStore.lists(db)
        }
    }

    private func rows(_ lists: [ListSummary]) -> some View {
        VStack(spacing: 10) {
            ForEach(lists) { list in
                NavigationLink(value: Route.list(id: list.id, name: list.name)) {
                    ListRow(list: list)
                }
                .buttonStyle(PressableStyle())
                .accessibilityIdentifier("lists.row")
            }
        }
        .padding(.horizontal, GridLayout.margin)
    }
}

/// A List with its first covers fanned beside its name.
private struct ListRow: View {
    let list: ListSummary

    var body: some View {
        HStack(spacing: 14) {
            ZStack(alignment: .leading) {
                ForEach(Array(list.covers.enumerated().reversed()), id: \.element.id) { index, book in
                    CoverView(book: book, width: 44)
                        .offset(x: CGFloat(index) * 14)
                }
                if list.covers.isEmpty {
                    RoundedRectangle(cornerRadius: 6, style: .continuous)
                        .fill(Theme.muted)
                        .frame(width: 44, height: 66)
                }
            }
            .frame(width: 44 + 28, alignment: .leading)
            .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 4) {
                Text(list.name)
                    .font(.geist(.body, weight: .semibold))
                    .foregroundStyle(Theme.foreground)
                    .multilineTextAlignment(.leading)
                Text(subtitle)
                    .font(.geist(.footnote))
                    .foregroundStyle(Theme.mutedForeground)
            }
            Spacer(minLength: 0)
            Image(systemName: "chevron.right")
                .font(.geist(.footnote, weight: .semibold))
                .foregroundStyle(Theme.mutedForeground)
                .accessibilityHidden(true)
        }
        .padding(12)
        .cardStyle()
        .accessibilityElement(children: .combine)
    }

    private var subtitle: String {
        let count = Counts.books(list.bookCount)
        guard list.permission != "owner" else { return count }
        return String(localized: "\(count) · by \(list.ownerName)")
    }
}

/// A List's Books, in the order the List's people chose for it.
struct ListDetailView: View {
    @Environment(AppModel.self) private var model
    let listID: Int
    let initialName: String

    @State private var list: ListSummary??
    @State private var books: [BookSummary]?

    var body: some View {
        GeometryReader { geometry in
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    if case .some(.none) = list {
                        ContentUnavailableView(String(localized: "This List isn't available anymore"), systemImage: "list.bullet.rectangle")
                            .padding(.top, 80)
                    } else {
                        if let description = (list ?? nil)?.description, !description.isEmpty {
                            Text(PlainText.from(description))
                                .font(.geist(.subheadline))
                                .foregroundStyle(Theme.mutedForeground)
                                .padding(.horizontal, GridLayout.margin)
                                .padding(.top, 8)
                        }
                        if let shares = (list ?? nil)?.shares, !shares.isEmpty {
                            Label(String(localized: "Shared with \(shares.map { "\($0.username) (\(SharePermission.text($0.permission)))" }.joined(separator: ", "))"),
                                  systemImage: "person.2")
                                .font(.geist(.footnote))
                                .foregroundStyle(Theme.mutedForeground)
                                .padding(.horizontal, GridLayout.margin)
                                .padding(.top, 8)
                                .accessibilityIdentifier("list.shares")
                        }
                        if let books {
                            SectionHeading(title: String(localized: "Books"), count: Counts.books(books.count))
                            if books.isEmpty {
                                EmptyNotice(title: String(localized: "No Books"), message: String(localized: "Books added to this List appear here."),
                                            systemImage: "book.closed")
                            } else {
                                BookGrid(books: books, width: geometry.size.width, identifier: "list.grid")
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
        .navigationTitle((list ?? nil)?.name ?? initialName)
        .navigationBarTitleDisplayMode(.large)
        .observing(model.session?.store, id: listID, into: $list) { [listID] db in
            try CatalogStore.list(db, id: listID)
        }
        .observing(model.session?.store, id: listID, into: $books) { [listID] db in
            try CatalogStore.listBooks(db, listID: listID)
        }
    }
}

/// What a List Share allows, as the web app names it.
enum SharePermission {
    static func text(_ permission: String) -> String {
        switch permission {
        case "viewer": String(localized: "can view")
        case "editor": String(localized: "can edit")
        case "manager": String(localized: "can manage")
        default: permission
        }
    }
}
