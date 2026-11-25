import SwiftUI

/// The Reading tab: the Books being read and on To Read, as shelves, and the
/// way to Reading History and Reading Stats. All of it reads the device's
/// copy, with changes made offline on top.
struct ReadingView: View {
    @Environment(AppModel.self) private var model
    @State private var current: [BookSummary]?
    @State private var toRead: [BookSummary]?

    var body: some View {
        @Bindable var model = model
        NavigationStack(path: $model.readingPath) {
            ScrollViewReader { proxy in
                ScrollView {
                    VStack(alignment: .leading, spacing: 0) {
                        Color.clear.frame(height: 0).id(ScrollTop.anchor)
                        shelf(title: String(localized: "Currently Reading"), books: current, route: .currentlyReading,
                              empty: String(localized: "Books you start appear here."), identifier: "reading.current")
                        shelf(title: String(localized: "To Read"), books: toRead, route: .toRead,
                              empty: String(localized: "Books you add to To Read appear here, in the order you choose."),
                              identifier: "reading.toRead")
                        VStack(spacing: 8) {
                            ReadingLink(title: String(localized: "Reading History"), icon: .history, route: .history,
                                        identifier: "reading.history")
                            if model.offers(.readingStats) {
                                ReadingLink(title: String(localized: "Reading Stats"), icon: .chartColumn, route: .stats,
                                            identifier: "reading.stats")
                            }
                        }
                        .padding(.horizontal, GridLayout.margin)
                        .padding(.top, 28)
                    }
                    .padding(.bottom, 32)
                    .frame(maxWidth: 900)
                    .frame(maxWidth: .infinity)
                }
                .miniPlayerClearance()
                .accessibilityIdentifier("reading.root")
                .onChange(of: model.scrollToTop[.reading]) { _, _ in
                    withAnimation { proxy.scrollTo(ScrollTop.anchor, anchor: .top) }
                }
            }
            .background(Theme.background)
            .refreshable { await model.syncNow() }
            .navigationTitle("Reading")
            .navigationBarTitleDisplayMode(.large)
            .toolbar { RootToolbar() }
            .routeDestinations()
        }
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $current) { db in
            try CatalogStore.currentlyReading(db)
        }
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $toRead) { db in
            try CatalogStore.toReadBooks(db)
        }
    }

    @ViewBuilder
    private func shelf(title: String, books: [BookSummary]?, route: Route, empty: String, identifier: String) -> some View {
        HStack(alignment: .firstTextBaseline) {
            Text(title)
                .font(.geist(.title3, weight: .semibold))
                .accessibilityAddTraits(.isHeader)
            if let books, !books.isEmpty {
                Text(verbatim: "\(books.count)")
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
            }
            Spacer()
            if let books, !books.isEmpty {
                NavigationLink(value: route) {
                    Text("See all")
                        .font(.geist(.subheadline, weight: .medium))
                        .foregroundStyle(Theme.primary)
                        .frame(minHeight: 44)
                }
                .accessibilityLabel(String(localized: "See all \(title)"))
                .accessibilityIdentifier("\(identifier).all")
            }
        }
        .padding(.horizontal, GridLayout.margin)
        .padding(.top, 20)
        if let books {
            if books.isEmpty {
                Text(empty)
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
                    .padding(.horizontal, GridLayout.margin)
                    .padding(.vertical, 12)
                    .accessibilityIdentifier("\(identifier).empty")
            } else {
                ScrollView(.horizontal, showsIndicators: false) {
                    LazyHStack(alignment: .top, spacing: GridLayout.spacing) {
                        ForEach(books.prefix(20)) { book in
                            NavigationLink(value: Route.book(book)) {
                                BookCell(book: book, width: 112)
                                    .frame(width: 112)
                            }
                            .buttonStyle(PressableStyle())
                        }
                    }
                    .padding(.horizontal, GridLayout.margin)
                    .padding(.top, 10)
                }
                .accessibilityIdentifier(identifier)
            }
        }
    }
}

/// A row that opens one of the Reading tab's pages.
private struct ReadingLink: View {
    let title: String
    let icon: Lucide
    let route: Route
    let identifier: String

    var body: some View {
        NavigationLink(value: route) {
            HStack(spacing: 12) {
                Icon(icon, size: 18, relativeTo: .body)
                    .foregroundStyle(Theme.primary)
                    .frame(width: 24)
                Text(title)
                    .font(.geist(.callout, weight: .medium))
                    .foregroundStyle(Theme.foreground)
                Spacer()
                Icon(.chevronRight, size: 14, relativeTo: .body)
                    .foregroundStyle(Theme.mutedForeground)
                    .accessibilityHidden(true)
            }
            .padding(14)
            .frame(minHeight: 44)
            .cardStyle(radius: Theme.radius + 2)
            .contentShape(Rectangle())
        }
        .buttonStyle(PressableStyle())
        .accessibilityIdentifier(identifier)
    }
}

/// Every Book with a Read-through underway, the most recently read first.
struct CurrentlyReadingView: View {
    @Environment(AppModel.self) private var model
    @State private var books: [BookSummary]?

    var body: some View {
        GeometryReader { geometry in
            ScrollView {
                if let books {
                    if books.isEmpty {
                        EmptyNotice(title: String(localized: "Nothing underway"),
                                    message: String(localized: "Books you start appear here."), icon: .bookOpen)
                    } else {
                        BookGrid(books: books, width: geometry.size.width, identifier: "currentlyReading.grid")
                            .padding(.top, 8)
                    }
                }
            }
            .miniPlayerClearance()
        }
        .background(Theme.background)
        .refreshable { await model.syncNow() }
        .navigationTitle("Currently Reading")
        .navigationBarTitleDisplayMode(.large)
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $books) { db in
            try CatalogStore.currentlyReading(db)
        }
    }
}

/// To Read, in the user's order, which they change by dragging, offline or
/// not. The latest move of each Book wins when it syncs (ADR 0024).
struct ToReadView: View {
    @Environment(AppModel.self) private var model
    @State private var books: [BookSummary]?

    private var editable: Bool { model.offers(.toRead) }

    var body: some View {
        Group {
            if let books, books.isEmpty {
                EmptyNotice(title: String(localized: "To Read is empty"),
                            message: String(localized: "Books you add to To Read appear here, in the order you choose."), icon: .clock)
                    .frame(maxHeight: .infinity, alignment: .top)
            } else {
                List {
                    ForEach(Array((books ?? []).enumerated()), id: \.element.id) { index, book in
                        Button { model.push(.book(book)) } label: {
                            BookRow(book: book, place: index + 1)
                        }
                        .buttonStyle(.plain)
                        .listRowBackground(Theme.background)
                        .accessibilityIdentifier("toRead.row")
                        .accessibilityActions {
                            if editable {
                                if index > 0 {
                                    Button(String(localized: "Move up")) { move(from: index, to: index - 1) }
                                }
                                if index < (books?.count ?? 0) - 1 {
                                    Button(String(localized: "Move down")) { move(from: index, to: index + 1) }
                                }
                                Button(String(localized: "Remove from To Read")) { model.change(.removeFromToRead(book: book.id)) }
                            }
                        }
                        .swipeActions {
                            if editable {
                                Button(role: .destructive) { model.change(.removeFromToRead(book: book.id)) } label: {
                                    Label("Remove", menuIcon: .trash)
                                }
                                .accessibilityIdentifier("toRead.remove")
                            }
                        }
                        .contextMenu {
                            if editable {
                                if index > 0 {
                                    Button { move(from: index, to: 0) } label: { Label("Move to Top", menuIcon: .arrowUp) }
                                        .accessibilityIdentifier("toRead.moveToTop")
                                }
                                if index < (books?.count ?? 0) - 1 {
                                    Button { move(from: index, to: (books?.count ?? 1) - 1) } label: {
                                        Label("Move to Bottom", menuIcon: .arrowDown)
                                    }
                                    .accessibilityIdentifier("toRead.moveToBottom")
                                }
                                Button(role: .destructive) { model.change(.removeFromToRead(book: book.id)) } label: {
                                    Label("Remove from To Read", menuIcon: .trash)
                                }
                                .accessibilityIdentifier("toRead.menuRemove")
                            }
                        }
                    }
                    .onMove(perform: editable ? { source, destination in
                        guard let from = source.first else { return }
                        move(from: from, to: destination > from ? destination - 1 : destination)
                    } : nil)
                }
                .listStyle(.plain)
                .scrollContentBackground(.hidden)
                .miniPlayerClearance()
            }
        }
        .background(Theme.background)
        .refreshable { await model.syncNow() }
        .navigationTitle("To Read")
        .navigationBarTitleDisplayMode(.large)
        .toolbar {
            if editable, (books?.count ?? 0) > 1 {
                ToolbarItem(placement: .topBarTrailing) {
                    EditButton()
                        .font(.geist(.body, weight: .medium))
                        .accessibilityIdentifier("toRead.edit")
                }
            }
        }
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $books) { db in
            try CatalogStore.toReadBooks(db)
        }
    }

    /// Moves the Book at from to to, recording it as a move after the Book
    /// it now follows, or to the top.
    private func move(from: Int, to: Int) {
        guard let move = ReadingLayer.move(in: books?.map(\.id) ?? [], from: from, to: to) else { return }
        model.change(.moveToRead(book: move.book, after: move.after))
    }
}

/// A Book in a list: its cover, title, and authors, with its place when it
/// has one.
struct BookRow: View {
    let book: BookSummary
    var place: Int?

    var body: some View {
        HStack(spacing: 12) {
            if let place {
                Text(verbatim: "\(place)")
                    .font(.geist(.footnote, weight: .semibold))
                    .monospacedDigit()
                    .foregroundStyle(Theme.mutedForeground)
                    .frame(minWidth: 22)
            }
            CoverView(book: book, width: 44)
                .frame(width: 44)
            VStack(alignment: .leading, spacing: 3) {
                Text(book.title)
                    .font(.geist(.subheadline, weight: .semibold))
                    .foregroundStyle(Theme.foreground)
                    .lineLimit(2)
                if !book.authors.isEmpty {
                    Text(book.authors)
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                        .lineLimit(1)
                }
            }
            Spacer(minLength: 0)
        }
        .frame(minHeight: 60)
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
    }
}

/// Every Read-through, the ones underway first, then by when each ended,
/// as the web's Reading History lists them. One holding a change or
/// reading the server hasn't confirmed has a small mark.
struct ReadingHistoryView: View {
    @Environment(AppModel.self) private var model
    @State private var entries: [HistoryEntry]?
    @State private var deleting: HistoryEntry?

    var body: some View {
        Group {
            if let entries, entries.isEmpty {
                EmptyNotice(title: String(localized: "No reading history yet"),
                            message: String(localized: "Books you start, finish, or stop reading appear here."), icon: .history)
                    .frame(maxHeight: .infinity, alignment: .top)
            } else {
                List {
                    ForEach(entries ?? []) { entry in
                        Button {
                            if let book = entry.book { model.push(.book(book)) }
                        } label: {
                            HistoryRow(entry: entry)
                        }
                        .buttonStyle(.plain)
                        .disabled(entry.book == nil)
                        .listRowBackground(Theme.background)
                        .accessibilityIdentifier("history.row")
                        .swipeActions {
                            if model.offers(.readingStatus) {
                                Button(role: .destructive) { deleting = entry } label: {
                                    Label("Delete", menuIcon: .trash)
                                }
                                .accessibilityIdentifier("history.delete")
                            }
                        }
                        .accessibilityActions {
                            if model.offers(.readingStatus) {
                                Button(String(localized: "Delete Read-through")) { deleting = entry }
                            }
                        }
                    }
                }
                .listStyle(.plain)
                .scrollContentBackground(.hidden)
                .miniPlayerClearance()
            }
        }
        .background(Theme.background)
        .refreshable { await model.syncNow() }
        .navigationTitle("Reading History")
        .navigationBarTitleDisplayMode(.large)
        .alert(String(localized: "Delete this Read-through?"), isPresented: Binding(get: { deleting != nil }, set: { if !$0 { deleting = nil } }),
               presenting: deleting) { entry in
            Button(String(localized: "Delete"), role: .destructive) { model.change(.deleteReadThrough(id: entry.id)) }
                .accessibilityIdentifier("history.delete.confirm")
            Button(String(localized: "Cancel"), role: .cancel) {}
                .accessibilityIdentifier("history.delete.cancel")
        } message: { entry in
            Text(HistoryText.deleteDescription(entry))
        }
        .observing(model.session?.store, id: model.session?.account.id ?? "", into: $entries) { db in
            try CatalogStore.history(db)
        }
    }
}

private struct HistoryRow: View {
    let entry: HistoryEntry

    var body: some View {
        HStack(spacing: 12) {
            Group {
                if let book = entry.book {
                    CoverView(book: book, width: 44)
                } else {
                    RoundedRectangle(cornerRadius: 4, style: .continuous)
                        .fill(Theme.muted)
                        .frame(width: 44, height: 66)
                }
            }
            .frame(width: 44)
            VStack(alignment: .leading, spacing: 3) {
                Text(entry.title)
                    .font(.geist(.subheadline, weight: .semibold))
                    .foregroundStyle(Theme.foreground)
                    .lineLimit(2)
                if !entry.authors.isEmpty {
                    Text(entry.authors)
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                        .lineLimit(1)
                }
                HStack(spacing: 6) {
                    Text(HistoryText.outcome(entry))
                        .font(.geist(.caption, weight: .semibold))
                        .foregroundStyle(StatusStyle.color(HistoryText.status(entry)))
                    Text(HistoryText.days(entry))
                        .font(.geist(.caption))
                        .foregroundStyle(Theme.mutedForeground)
                    if entry.pending {
                        PendingMark()
                    }
                }
            }
            Spacer(minLength: 0)
        }
        .frame(minHeight: 70)
        .contentShape(Rectangle())
        // Said in words, since a combined row drops its mark's label.
        .accessibilityElement(children: .ignore)
        .accessibilityLabel([entry.title, entry.authors, HistoryText.outcome(entry), HistoryText.days(entry),
                             entry.pending ? String(localized: "Not synced yet") : ""]
            .filter { !$0.isEmpty }.joined(separator: ", "))
    }
}

/// How Reading History names a Read-through's outcome and days, as the web's
/// `readThroughOutcomeLabel` and `readThroughDatesLabel` do.
enum HistoryText {
    static func status(_ entry: HistoryEntry) -> String {
        switch entry.outcome {
        case "underway": entry.paused ? "paused" : "currently_reading"
        case "finished": "read"
        default: "did_not_finish"
        }
    }

    static func outcome(_ entry: HistoryEntry) -> String {
        switch entry.outcome {
        case "underway": entry.paused ? String(localized: "Paused") : String(localized: "Reading")
        case "finished": String(localized: "Finished")
        default: String(localized: "Did Not Finish")
        }
    }

    /// What deleting a Read-through takes with it, as the web's
    /// `readThroughDeleteDescription` says.
    static func deleteDescription(_ entry: HistoryEntry) -> String {
        if entry.book == nil {
            return String(localized: "It won't count in your reading history anymore. The reading time recorded during it is deleted too.")
        }
        if entry.outcome == "underway" {
            return String(localized: "It won't count toward this book's history or its Reading Status. The reading time recorded during it is deleted too. Every file opens at the start again, and if starting it took the book off To Read, it goes back to its place there.")
        }
        return String(localized: "It won't count toward this book's history or its Reading Status. The reading time recorded during it is deleted too.")
    }

    static func days(_ entry: HistoryEntry) -> String {
        let begin = entry.beginDay.flatMap(format)
        let end = entry.endDay.flatMap(format)
        if entry.outcome == "underway" {
            return begin.map { String(localized: "Started \($0)") } ?? String(localized: "Start date unknown")
        }
        if let begin, let end { return String(localized: "\(begin) to \(end)") }
        if let end { return String(localized: "Ended \(end), start date unknown") }
        if let begin { return String(localized: "Started \(begin), end date unknown") }
        return String(localized: "Dates unknown")
    }

    /// A day as the web formats it, such as "Mar 1, 2026".
    static func format(_ day: String) -> String? {
        let parts = day.split(separator: "-").compactMap { Int($0) }
        guard parts.count == 3 else { return nil }
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .current
        guard let date = calendar.date(from: DateComponents(year: parts[0], month: parts[1], day: parts[2])) else { return nil }
        return date.formatted(Date.FormatStyle(date: .abbreviated, time: .omitted, timeZone: calendar.timeZone))
    }
}
