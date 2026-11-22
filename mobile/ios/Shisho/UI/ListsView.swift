import SwiftUI

/// The user's Lists and the ones shared with them, read from the device,
/// as the web app's Lists page shows them.
struct ListsView: View {
    @Environment(AppModel.self) private var model
    @State private var lists: [ListSummary]?

    var body: some View {
        @Bindable var model = model
        NavigationStack(path: $model.listsPath) {
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    Text("Organize your books into custom collections")
                        .font(.geist(.subheadline))
                        .foregroundStyle(Theme.mutedForeground)
                        .padding(.horizontal, GridLayout.margin)
                        .padding(.bottom, 16)
                    if let lists {
                        if lists.isEmpty {
                            EmptyNotice(title: String(localized: "No Lists yet"),
                                        message: String(localized: "Lists you make on the web, and Lists shared with you, appear here."),
                                        systemImage: "list.bullet")
                                .accessibilityIdentifier("lists.empty")
                        }
                        LazyVStack(spacing: 8) {
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
                .padding(.bottom, 32)
                .frame(maxWidth: 680)
                .frame(maxWidth: .infinity)
            }
            .miniPlayerClearance()
            .background(Theme.background)
            .refreshable { await model.syncNow() }
            .navigationTitle("Lists")
            .navigationBarTitleDisplayMode(.large)
            .toolbar { RootToolbar() }
            .routeDestinations()
        }
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $lists) { db in
            try CatalogStore.lists(db)
        }
    }
}

/// A List as the web app's Lists page shows one: its name over its
/// description, what the user may do with it when it's someone else's, and
/// how many Books it has.
private struct ListRow: View {
    let list: ListSummary

    var body: some View {
        HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 3) {
                Text(list.name)
                    .font(.geist(.callout, weight: .medium))
                    .foregroundStyle(Theme.foreground)
                    .multilineTextAlignment(.leading)
                if let description = list.description, !description.isEmpty {
                    Text(PlainText.from(description))
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                        .lineLimit(1)
                } else if list.permission != "owner" {
                    Text("Shared by \(list.ownerName)")
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                        .lineLimit(1)
                }
            }
            Spacer(minLength: 8)
            if list.permission != "owner" {
                Text(SharePermission.name(list.permission))
                    .font(.geist(.caption, weight: .medium))
                    .padding(.horizontal, 8)
                    .padding(.vertical, 3)
                    .overlay(RoundedRectangle(cornerRadius: 6, style: .continuous).strokeBorder(Theme.border))
            }
            CountBadge(text: Counts.books(list.bookCount))
        }
        .padding(12)
        .cardStyle(radius: Theme.radius + 2)
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
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
    /// The permission on its own, as the web app's badge shows it.
    static func name(_ permission: String) -> String {
        switch permission {
        case "viewer": String(localized: "viewer")
        case "editor": String(localized: "editor")
        case "manager": String(localized: "manager")
        default: permission
        }
    }

    static func text(_ permission: String) -> String {
        switch permission {
        case "viewer": String(localized: "can view")
        case "editor": String(localized: "can edit")
        case "manager": String(localized: "can manage")
        default: permission
        }
    }
}
