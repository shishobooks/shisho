import SwiftUI

/// The user's Lists and the ones shared with them, read from the device,
/// as the web app's Lists page shows them.
struct ListsView: View {
    @Environment(AppModel.self) private var model
    @State private var lists: [ListSummary]?
    @State private var making = false

    var body: some View {
        @Bindable var model = model
        NavigationStack(path: $model.listsPath) {
            ScrollViewReader { proxy in
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    Text("Organize your books into custom collections")
                        .font(.geist(.subheadline))
                        .foregroundStyle(Theme.mutedForeground)
                        .padding(.horizontal, GridLayout.margin)
                        .padding(.bottom, 16)
                        .id(ScrollTop.anchor)
                    if let lists {
                        if lists.isEmpty {
                            EmptyNotice(title: String(localized: "No Lists yet"),
                                        message: String(localized: "Lists you make, and Lists shared with you, appear here."),
                                        icon: .list)
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
            .onChange(of: model.scrollToTop[.lists]) { _, _ in
                withAnimation { proxy.scrollTo(ScrollTop.anchor, anchor: .top) }
            }
            }
            .background(Theme.background)
            .refreshable { await model.syncNow() }
            .navigationTitle("Lists")
            .navigationBarTitleDisplayMode(.large)
            .toolbar {
                if model.offers(.lists) {
                    ToolbarItem(placement: .topBarLeading) {
                        Button { making = true } label: {
                            Icon(.plus, size: 20, relativeTo: nil)
                                .foregroundStyle(Theme.foreground)
                                .frame(minWidth: 44, minHeight: 44)
                        }
                        .accessibilityLabel(String(localized: "New List"))
                        .accessibilityIdentifier("lists.new")
                    }
                }
                RootToolbar()
            }
            .routeDestinations()
            .sheet(isPresented: $making) {
                ListFormSheet(title: String(localized: "New List")) { name, ordered in
                    if let id = model.change(.createList(name: name, description: nil, ordered: ordered)) {
                        model.push(.list(id: id, name: name))
                    }
                }
            }
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
    @State private var downloadable: DownloadAllButton.Files?
    @State private var abilities: ListAbilities?
    @State private var editing = false
    @State private var renaming = false
    @State private var confirmingDelete = false

    /// What the user may do with the List here.
    struct ListAbilities: Equatable, Sendable {
        var ordered: Bool
        var permission: String
    }

    private var canEditBooks: Bool {
        model.offers(.lists) && ["owner", "editor", "manager"].contains(abilities?.permission ?? "")
    }
    private var canRename: Bool { model.offers(.lists) && ["owner", "manager"].contains(abilities?.permission ?? "") }
    private var canDelete: Bool { model.offers(.lists) && abilities?.permission == "owner" }

    var body: some View {
        GeometryReader { geometry in
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    if case .some(.none) = list {
                        EmptyNotice(title: String(localized: "This List isn't available anymore"), icon: .listX)
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
                            Label(verbatim: String(localized: "Shared with \(shares.map { "\($0.username) (\(SharePermission.text($0.permission)))" }.joined(separator: ", "))"),
                                  icon: .users, size: 14, relativeTo: .footnote)
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
                                            icon: .book)
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
        .toolbar {
            if let downloadable, !downloadable.fileIDs.isEmpty {
                ToolbarItem(placement: .topBarTrailing) {
                    DownloadAllButton(files: downloadable)
                }
            }
            if canEditBooks || canRename || canDelete {
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
                        if canEditBooks, !(books ?? []).isEmpty {
                            Button { editing = true } label: {
                                Label(abilities?.ordered == true ? "Reorder or Remove Books" : "Remove Books", menuIcon: .gripVertical)
                            }
                            .accessibilityIdentifier("list.edit")
                        }
                        if canRename {
                            Button { renaming = true } label: { Label("Rename", menuIcon: .pencil) }
                                .accessibilityIdentifier("list.rename")
                        }
                        if canDelete {
                            Button(role: .destructive) { confirmingDelete = true } label: { Label("Delete List", menuIcon: .trash) }
                                .accessibilityIdentifier("list.delete")
                        }
                    } label: {
                        Icon(.ellipsis, size: 20, relativeTo: nil)
                            .foregroundStyle(Theme.foreground)
                            .frame(minWidth: 44, minHeight: 44)
                    }
                    .accessibilityLabel(String(localized: "List actions"))
                    .accessibilityIdentifier("list.menu")
                }
            }
        }
        .sheet(isPresented: $editing) {
            ListEditSheet(listID: listID, ordered: abilities?.ordered ?? false)
        }
        .sheet(isPresented: $renaming) {
            ListFormSheet(title: String(localized: "Rename List"), name: (list ?? nil)?.name ?? initialName, asksOrdered: false) { name, _ in
                model.change(.renameList(id: listID, name: name))
            }
        }
        .alert(String(localized: "Delete this List?"), isPresented: $confirmingDelete) {
            Button(String(localized: "Delete"), role: .destructive) {
                model.change(.deleteList(id: listID))
                model.popToRoot(.lists)
            }
            Button(String(localized: "Cancel"), role: .cancel) {}
        } message: {
            Text("The List goes for everyone it's shared with. The Books in it stay in your Libraries.")
        }
        .observing(model.session?.store, id: listID, into: $abilities) { [listID] db in
            try CatalogStore.listAbilities(db, listID: listID).map { ListAbilities(ordered: $0.ordered, permission: $0.permission) }
                ?? ListAbilities(ordered: false, permission: "")
        }
        .observing(model.session?.store, id: listID, into: $downloadable) { [listID] db in
            try DownloadAllButton.Files.fetch(db, listID: listID)
        }
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

/// A List's Books to remove, and in an ordered List, to drag into a new
/// order, offline or not. The latest move of each Book wins when it syncs.
private struct ListEditSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    let listID: Int
    let ordered: Bool

    @State private var books: [BookSummary]?

    var body: some View {
        NavigationStack {
            List {
                ForEach(Array((books ?? []).enumerated()), id: \.element.id) { index, book in
                    BookRow(book: book, place: ordered ? index + 1 : nil)
                        .accessibilityIdentifier("list.edit.row")
                        .accessibilityActions {
                            if ordered, index > 0 {
                                Button(String(localized: "Move up")) { move(from: index, to: index - 1) }
                            }
                            if ordered, index < (books?.count ?? 0) - 1 {
                                Button(String(localized: "Move down")) { move(from: index, to: index + 1) }
                            }
                            Button(String(localized: "Remove from List")) { model.change(.removeFromList(id: listID, book: book.id)) }
                        }
                }
                .onMove(perform: ordered ? { source, destination in
                    guard let from = source.first else { return }
                    move(from: from, to: destination > from ? destination - 1 : destination)
                } : nil)
                .onDelete { offsets in
                    for index in offsets {
                        if let book = books?[index] { model.change(.removeFromList(id: listID, book: book.id)) }
                    }
                }
            }
            .environment(\.editMode, .constant(.active))
            .navigationTitle(ordered ? String(localized: "Reorder or Remove") : String(localized: "Remove Books"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button(String(localized: "Done")) { dismiss() }
                        .accessibilityIdentifier("list.edit.done")
                }
            }
        }
        .presentationDragIndicator(.visible)
        .observing(model.session?.store, id: listID, into: $books) { [listID] db in
            // The manual order when it has one, which this sheet changes.
            let order = try CatalogStore.listOrder(db, listID: listID)
            let byID = Dictionary(try CatalogStore.listBooks(db, listID: listID).map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
            return order.compactMap { byID[$0] }
        }
    }

    private func move(from: Int, to: Int) {
        guard var order = books?.map(\.id), order.indices.contains(from), from != to else { return }
        let id = order.remove(at: from)
        let target = min(max(0, to), order.count)
        order.insert(id, at: target)
        model.change(.moveInList(id: listID, book: id, after: target == 0 ? nil : order[target - 1]))
    }
}
