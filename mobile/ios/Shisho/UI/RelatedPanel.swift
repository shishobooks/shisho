import SwiftUI

/// The Books near this one, as the web Book page's Related panel shows them:
/// a tab for the rest of its Series, its first author's Books, and the Lists
/// it's on, led by To Read. Each tab is a window about four and a half Books
/// tall, opened on this Book, so going from Book to Book keeps it in place.
/// The web's Devices tab needs the server, so the app leaves it out.
struct RelatedPanel: View {
    @Environment(AppModel.self) private var model
    let bookID: Int
    let related: BookRelated

    enum Tab: String, CaseIterable {
        case series, author, lists

        var title: String {
            switch self {
            case .series: String(localized: "Series")
            case .author: String(localized: "Author")
            case .lists: String(localized: "Lists")
            }
        }
    }

    /// 24 points of heading and four and a half rows, as on the web, so the
    /// cut-off row says the window scrolls. It sits in a card, so the
    /// pinned headings have a surface to cover the rows with.
    private static let windowHeight: CGFloat = 24 + 4.5 * 56

    private var tabs: [Tab] {
        var tabs: [Tab] = []
        if !related.series.isEmpty { tabs.append(.series) }
        if related.author != nil { tabs.append(.author) }
        if !related.lists.isEmpty { tabs.append(.lists) }
        return tabs
    }

    /// The tab picked last, which the next Book opens on if it has it.
    private var active: Tab? {
        tabs.first { $0.rawValue == model.relatedTab } ?? tabs.first
    }

    private func groups(_ tab: Tab) -> [BookRelated.Group] {
        switch tab {
        case .series: related.series
        case .author: related.author.map { [$0] } ?? []
        case .lists: related.lists
        }
    }

    var body: some View {
        if let active {
            VStack(alignment: .leading, spacing: 12) {
                tabStrip(active)
                window(active)
            }
            .accessibilityIdentifier("book.related")
        }
    }

    /// The web's tab list: the tabs over a hairline, the open one underlined.
    private func tabStrip(_ active: Tab) -> some View {
        HStack(spacing: 4) {
            ForEach(tabs, id: \.self) { tab in
                let selected = tab == active
                Button {
                    withAnimation(.easeOut(duration: 0.15)) { model.relatedTab = tab.rawValue }
                } label: {
                    HStack(spacing: 6) {
                        Text(tab.title)
                            .font(.geist(.footnote, weight: .medium))
                            .foregroundStyle(selected ? Theme.foreground : Theme.mutedForeground)
                        if tab == .lists {
                            Text(verbatim: "\(related.lists.count)")
                                .font(.geist(size: 10, relativeTo: .caption2, weight: .semibold))
                                .monospacedDigit()
                                .foregroundStyle(Theme.mutedForeground)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 1)
                                .background(Theme.muted, in: Capsule())
                        }
                    }
                    .padding(.horizontal, 12)
                    .frame(minHeight: 36)
                    .overlay(alignment: .bottom) {
                        if selected {
                            Rectangle().fill(Theme.primary).frame(height: 2)
                        }
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel(tab == .lists ? "\(tab.title) \(related.lists.count)" : tab.title)
                .accessibilityAddTraits(selected ? [.isSelected] : [])
                .accessibilityIdentifier("book.related.\(tab.rawValue)")
            }
            Spacer(minLength: 0)
        }
        .overlay(alignment: .bottom) {
            Rectangle().fill(Theme.border).frame(height: 1)
        }
    }

    private func window(_ tab: Tab) -> some View {
        let groups = groups(tab)
        let current = groups.first { $0.lines.contains { $0.book.id == bookID } }.map { "\($0.id)-\(bookID)" }
        // As tall as its Books when they're few, so a short tab has no gap.
        let content = groups.reduce(CGFloat(0)) { $0 + 24 + CGFloat($1.lines.count) * 56 } + CGFloat(max(0, groups.count - 1)) * 10
        return ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 0, pinnedViews: [.sectionHeaders]) {
                    ForEach(groups) { group in
                        Section {
                            ForEach(group.lines) { line in
                                RelatedLine(line: line, current: line.book.id == bookID)
                                    .id("\(group.id)-\(line.book.id)")
                            }
                        } header: {
                            heading(group)
                        }
                        .padding(.bottom, group.id == groups.last?.id ? 0 : 10)
                    }
                }
            }
            .frame(height: min(content, Self.windowHeight))
            .padding(6)
            .cardStyle()
            // Opened on this Book, centered, as the web's window opens.
            .task(id: "\(tab.rawValue)-\(bookID)") {
                guard let current else { return }
                await Task.yield()
                proxy.scrollTo(current, anchor: .center)
            }
        }
    }

    /// A group's heading, which stays in view while its Books scroll: its
    /// name, linked to its page, and how many Books it has.
    private func heading(_ group: BookRelated.Group) -> some View {
        HStack(spacing: 0) {
            if let route = route(group.link) {
                Button { model.push(route) } label: {
                    Text(group.title)
                        .underline()
                        .foregroundStyle(Theme.foreground)
                }
                .buttonStyle(.plain)
            } else {
                Text(group.title)
                    .foregroundStyle(Theme.foreground)
            }
            Text(verbatim: " · " + Counts.books(group.total))
                .foregroundStyle(Theme.mutedForeground)
            Spacer(minLength: 0)
        }
        .font(.geist(.caption, weight: .medium))
        .lineLimit(1)
        .padding(.horizontal, 6)
        .frame(height: 24)
        .background(Theme.card)
        .accessibilityAddTraits(.isHeader)
    }

    private func route(_ link: BookRelated.Link) -> Route? {
        switch link {
        case .series(let id, let name): .resource(.series, id: id, name: name)
        case .person(let id, let name): .resource(.person, id: id, name: name)
        case .list(let id, let name): .list(id: id, name: name)
        case .none: nil
        }
    }
}

/// One Book in a tab: its cover, a small line (its number, year, or place),
/// and its title. This Book is marked, not linked.
private struct RelatedLine: View {
    let line: BookRelated.Line
    let current: Bool

    private var meta: String? {
        let parts = [line.meta, current ? String(localized: "This book") : nil].compactMap { $0 }
        return parts.isEmpty ? nil : parts.joined(separator: " · ")
    }

    var body: some View {
        if current {
            row
                .background(Theme.muted, in: RoundedRectangle(cornerRadius: 6, style: .continuous))
                .accessibilityElement(children: .combine)
        } else {
            NavigationLink(value: Route.book(line.book)) { row }
                .buttonStyle(.plain)
                .accessibilityElement(children: .combine)
        }
    }

    private var row: some View {
        HStack(spacing: 12) {
            CoverView(book: line.book, width: 32)
                .frame(width: 32, height: 48)
            VStack(alignment: .leading, spacing: 1) {
                if let meta {
                    Text(meta)
                        .font(.geist(.caption))
                        .foregroundStyle(Theme.mutedForeground)
                }
                Text(line.book.title)
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.foreground)
                    .lineLimit(2)
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 6)
        .frame(height: 56)
        .contentShape(Rectangle())
    }
}
