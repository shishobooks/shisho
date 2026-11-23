import SwiftUI

/// A Book from the device's copy of the catalog: its cover, with the cover's
/// color washed behind it, its title and authors, and its Files.
struct BookDetailView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.horizontalSizeClass) private var sizeClass
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let bookID: Int
    /// What the grid already knows, shown until the full Book loads.
    let initial: BookSummary

    @State private var book: BookDetail??
    @State private var listen: ListenProgress??
    @State private var tint: Color?
    @State private var scrolled = false
    @State private var standing: BookStanding?
    @State private var related: BookRelated??
    /// This page's Related tab, nil for the first.
    @State private var relatedTab: String?
    @State private var tookRelatedTab = false

    private var summary: BookSummary {
        book??.summary ?? initial
    }

    var body: some View {
        ScrollView {
            VStack(spacing: 0) {
                ScrollTopMarker(space: "book")
                if case .some(.none) = book {
                    EmptyState("This Book isn't in your Libraries anymore", icon: .book)
                        .padding(.top, 80)
                } else {
                    hero
                    if let progress = listen ?? nil {
                        ListenCard(progress: progress, bookID: bookID)
                            .padding(.top, 22)
                            .frame(maxWidth: 520)
                    }
                    if let detail = book ?? nil {
                        details(detail)
                    }
                }
            }
            .padding(.horizontal, 20)
            .padding(.bottom, 40)
            .frame(maxWidth: 680)
            .frame(maxWidth: .infinity)
        }
        .refreshable { await model.syncNow() }
        .opensRouteLinks(model)
        .miniPlayerClearance()
        .onScrolledDistanceChange(space: "book") { distance in
            // The title shows in the bar once the cover has scrolled away.
            let collapsed = distance > 210
            if collapsed != scrolled {
                withAnimation(.easeOut(duration: 0.2)) { scrolled = collapsed }
            }
        }
        .background(alignment: .top) { backdrop }
        .background(Theme.background)
        .navigationBarTitleDisplayMode(.inline)
        .toolbarBackground(scrolled ? .visible : .hidden, for: .navigationBar)
        .toolbar {
            ToolbarItem(placement: .principal) {
                Text(summary.title)
                    .font(.geist(.headline, weight: .semibold))
                    .lineLimit(1)
                    .opacity(scrolled ? 1 : 0)
                    .accessibilityHidden(!scrolled)
            }
        }
        .observing(model.session?.store, id: bookID, into: $book) { [bookID] db in
            try CatalogStore.book(db, id: bookID)
        }
        .onAppear {
            // Once, so coming back keeps this page's tab.
            guard !tookRelatedTab else { return }
            tookRelatedTab = true
            relatedTab = model.relatedTabForNextBook
            model.relatedTabForNextBook = nil
        }
        .observing(model.session?.store, id: bookID, into: $related) { [bookID] db in
            try CatalogStore.related(db, bookID: bookID)
        }
        .observing(model.session?.store, id: bookID, into: $standing) { [bookID] db in
            try CatalogStore.standing(db, bookID: bookID)
        }
        .observing(model.session?.store, id: bookID, into: $listen) { [bookID, deviceID = model.session?.account.deviceID ?? ""] db in
            try CatalogStore.listenProgress(db, bookID: bookID, ownDeviceID: deviceID)
        }
    }

    /// The cover's color, washed from the top and fading into the page.
    private var backdrop: some View {
        LinearGradient(
            stops: [
                .init(color: (tint ?? Theme.primary).opacity(tint == nil ? 0.08 : 0.42), location: 0),
                .init(color: (tint ?? Theme.primary).opacity(tint == nil ? 0.03 : 0.16), location: 0.55),
                .init(color: Theme.background.opacity(0), location: 1),
            ],
            startPoint: .top, endPoint: .bottom
        )
        .frame(height: 640)
        .ignoresSafeArea(edges: .top)
        .animation(.easeOut(duration: 0.5), value: tint)
        .accessibilityHidden(true)
    }

    private var hero: some View {
        VStack(spacing: 0) {
            CoverView(book: summary, width: sizeClass == .regular ? 260 : 200, shadow: .large) { image in
                let average = await CoverTint.average(of: image)
                // A cover replaced meanwhile has its own tint coming.
                guard !Task.isCancelled else { return }
                tint = average?.color
            }
            // The Reading Status ribbon, as on the grid and the prototype's
            // Book page.
            .overlay(alignment: .topLeading) {
                if let status = standing?.status {
                    ReadingStatusRibbon(status: status, progress: summary.progress, scale: 1.25)
                        .padding(.leading, 10)
                }
            }
            .padding(.top, 8)

            if let detail = book ?? nil, let series = detail.series.first {
                NavigationLink(value: Route.resource(.series, id: series.seriesID, name: series.name)) {
                    HStack(spacing: 8) {
                        if let number = series.number {
                            SeriesNumberBadge(number: number, end: series.numberEnd)
                        }
                        Text(series.name)
                            .font(.geist(.callout, weight: .semibold))
                            .foregroundStyle(Theme.foreground)
                    }
                    // A 44-point target, as Apple asks.
                    .frame(minHeight: 44)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .padding(.top, 11)
                .accessibilityElement(children: .combine)
                .accessibilityIdentifier("book.detail.series")
            }

            Text(summary.title)
                .font(.geist(size: 30, relativeTo: .title, weight: .bold))
                .tracking(-0.4)
                .multilineTextAlignment(.center)
                .padding(.top, 10)
                .accessibilityAddTraits(.isHeader)
                .accessibilityIdentifier("book.detail.title")

            if let subtitle = (book ?? nil)?.subtitle, !subtitle.isEmpty {
                Text(subtitle)
                    .font(.geist(.callout))
                    .foregroundStyle(Theme.mutedForeground)
                    .multilineTextAlignment(.center)
                    .padding(.top, 6)
            }

            if let detail = book ?? nil, !detail.authors.isEmpty {
                Text(Byline.text(detail.authors))
                    .font(.geist(.callout))
                    .multilineTextAlignment(.center)
                    .tint(Theme.foreground)
                    .padding(.top, 10)
                    .accessibilityIdentifier("book.detail.authors")
            } else if !summary.authors.isEmpty {
                (Text("by ").foregroundStyle(Theme.mutedForeground) + Text(summary.authors))
                    .font(.geist(.callout))
                    .multilineTextAlignment(.center)
                    .padding(.top, 10)
            }
        }
    }

    private func details(_ book: BookDetail) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            // The user's Reading Status and Rating, as the prototype sets
            // them above the details. They're changed on the web for now.
            if let standing, standing.status != nil || standing.rating != nil {
                HStack {
                    if let status = standing.status {
                        Label(verbatim: ReadingStatusLabel.text(status), icon: StatusStyle.icon(status), size: 14, relativeTo: .footnote)
                            .font(.geist(.footnote, weight: .medium))
                            .padding(.horizontal, 12)
                            .frame(minHeight: 34)
                            .background(Theme.card, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
                            .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(Theme.border))
                            .accessibilityIdentifier("book.detail.status")
                    }
                    Spacer(minLength: 8)
                    if let rating = standing.rating {
                        RatingStars(units: rating)
                    }
                }
                .padding(.top, 24)
            }
            let rows = infoRows(book)
            if !rows.isEmpty {
                VStack(alignment: .leading, spacing: 14) {
                    // At accessibility text sizes the label sits above its
                    // value, so neither has to wrap mid-word.
                    let stacked = dynamicTypeSize.isAccessibilitySize
                    let layout = stacked
                        ? AnyLayout(VStackLayout(alignment: .leading, spacing: 4))
                        : AnyLayout(HStackLayout(alignment: .firstTextBaseline, spacing: 12))
                    ForEach(rows, id: \.label) { row in
                        layout {
                            // Capitals only on screen; VoiceOver reads the word.
                            Text(row.label.uppercased())
                                .accessibilityLabel(row.label)
                                .font(.geist(.caption, weight: .semibold))
                                .tracking(0.8)
                                .foregroundStyle(Theme.mutedForeground)
                                .frame(width: stacked ? nil : 96, alignment: .leading)
                            row.value
                            if !stacked { Spacer(minLength: 0) }
                        }
                        .accessibilityElement(children: .combine)
                    }
                }
                .padding(.top, 22)
            }

            if let description = book.description, !description.isEmpty {
                ExpandableText(text: PlainText.from(description), identifier: "book.detail.description")
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.foreground.opacity(0.85))
                    .lineSpacing(3)
                    .padding(.top, 22)
            }

            if !book.files.isEmpty {
                Text("^[\(book.files.count) file](inflect: true)")
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
                    .padding(.top, 32)
                    .padding(.bottom, 10)
                    .accessibilityAddTraits(.isHeader)
                VStack(spacing: 10) {
                    ForEach(book.files) { file in
                        FileCard(file: file, book: book.summary, listen: listen ?? nil)
                    }
                }
            }

            // The Books near this one, at the bottom as the web's phone
            // layout has its Related panel.
            if let related = related ?? nil, !related.isEmpty {
                RelatedPanel(bookID: book.summary.id, related: related, selected: $relatedTab)
                    .padding(.top, 32)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private struct InfoRow {
        let label: String
        let value: AnyView
    }

    /// The details the prototype lists under labels: Series as links, genres
    /// and tags as badges, and when the Book was published. Creators are
    /// named with their roles under the title, and narrators on their Files.
    private func infoRows(_ book: BookDetail) -> [InfoRow] {
        var rows: [InfoRow] = []
        if !book.series.isEmpty {
            rows.append(InfoRow(label: String(localized: "Series"), value: AnyView(
                Text(Byline.links(book.series.map { (Route.resource(.series, id: $0.seriesID, name: $0.name), seriesLine($0)) }))
                    .font(.geist(.subheadline))
                    .tint(Theme.foreground)
                    .accessibilityIdentifier("book.detail.series.link")
            )))
        }
        if !book.genres.isEmpty {
            rows.append(InfoRow(label: String(localized: "Genres"), value: AnyView(LinkChips(items: book.genres.map {
                (Route.resource(.genre, id: $0.id, name: $0.name), $0.name)
            }, identifier: "book.detail.genre"))))
        }
        if !book.tags.isEmpty {
            rows.append(InfoRow(label: String(localized: "Tags"), value: AnyView(LinkChips(items: book.tags.map {
                (Route.resource(.tag, id: $0.id, name: $0.name), $0.name)
            }, identifier: "book.detail.tag"))))
        }
        if let published = book.published {
            rows.append(InfoRow(label: String(localized: "Published"), value: AnyView(
                Text(published, format: .dateTime.year()).font(.geist(.subheadline))
            )))
        }
        return rows
    }

    private func seriesLine(_ series: BookDetail.Series) -> String {
        guard let number = series.number else { return series.name }
        if let end = series.numberEnd {
            return String(localized: "\(series.name) #\(SeriesNumbers.format(number))-\(SeriesNumbers.format(end))")
        }
        return String(localized: "\(series.name) #\(SeriesNumbers.format(number))")
    }

}

/// One of a Book's Files, as the web app's file row shows it: its cover,
/// name, format, a quiet line of facts, its narrators, how far through it
/// the user is, and Listen. Tapping it opens the File's own page, with the
/// rest of what's known about it.
private struct FileCard: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.horizontalSizeClass) private var sizeClass
    let file: BookDetail.File
    let book: BookSummary
    /// The Listen card's progress, which this row matches when it's for
    /// this File, so the two never show different numbers.
    var listen: ListenProgress?

    private var isAudiobook: Bool { file.fileType == "m4b" }

    /// How far through the File the user is: for the audiobook the Listen
    /// card is about, the card's own place, live while it plays, so both say
    /// the same; for any other File, the place saved with its percentage.
    private var progress: Double? {
        guard isAudiobook else { return file.progress }
        if let player = model.player, player.file?.fileID == file.id, let duration = player.file?.duration, duration > 0 {
            return min(1, max(0, player.position / duration))
        }
        if let listen, listen.fileID == file.id {
            return listen.seconds > 1 ? listen.fraction : file.progress
        }
        return file.progress
    }

    /// An audiobook's length at the speed it plays at, so the time left
    /// matches the Listen card's.
    private var listeningLength: Double? {
        guard isAudiobook, let duration = file.durationSeconds else { return file.durationSeconds }
        let rate: Double
        if let player = model.player, player.file?.fileID == file.id {
            rate = player.rate
        } else if let account = model.session?.account {
            rate = AudioPlayer.storedRate(accountID: account.id, fileID: file.id)
        } else {
            rate = 1
        }
        return duration / max(rate, 0.1)
    }
    /// The square the cover sits in: an audiobook's fills it, and a book's
    /// two-thirds-wide cover is as tall as it.
    private static let coverSlot: CGFloat = 52
    private var name: String { file.label }

    /// Names the File and its narrators, since editions often share a name.
    private var listenLabel: String {
        guard !file.narrators.isEmpty else { return String(localized: "Listen to \(name)") }
        return String(localized: "Listen to \(name), narrated by \(file.narrators.joined(separator: ", "))")
    }
    private var playing: Bool { model.player?.file?.fileID == file.id && model.player?.isPlaying == true }

    private var narrators: [(Route, String)] {
        zip(file.narratorIDs, file.narrators).map { (Route.resource(.person, id: $0.0, name: $0.1), $0.1) }
    }

    var body: some View {
        // One line: the File, then its one action. How far through it the
        // user is sits under its facts, as on the web's row.
        HStack(alignment: .center, spacing: 10) {
            Button { model.push(.file(bookID: book.id, fileID: file.id)) } label: {
            // The cover in a square slot, as on the web, so a tall ebook cover
            // doesn't make its row taller than an audiobook's. A cover taller
            // than the text centers the text beside it; text taller than the
            // cover keeps the cover at the top.
            HStack(alignment: .top, spacing: 12) {
                CoverView(book: book, file: CoverFile(id: file.id, fileType: file.fileType, coverVersion: file.coverVersion),
                          width: isAudiobook ? Self.coverSlot : Self.coverSlot * 2 / 3)
                    .frame(width: Self.coverSlot)
                VStack(alignment: .leading, spacing: 5) {
                    // Each File by its own label, as the web app's file list does;
                    // the Book's title would name every File the same.
                    Text(name)
                        .font(.geist(.subheadline, weight: .semibold))
                        .lineLimit(2)
                    if FileTypes.isKnown(file.fileType) {
                        // The chip stays centered on facts that wrap, and sits
                        // above them at accessibility text sizes, so they
                        // keep the row's width.
                        let factsLayout = dynamicTypeSize.isAccessibilitySize
                            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 4))
                            : AnyLayout(HStackLayout(alignment: .center, spacing: 8))
                        factsLayout {
                            FileTypeChip(fileType: file.fileType, outlined: true)
                            Text(facts)
                                .font(.geist(.footnote))
                                .foregroundStyle(Theme.mutedForeground)
                                .monospacedDigit()
                        }
                        if !narrators.isEmpty {
                            Text(LinkedText.muted(String(localized: "Narrated by ")) + Byline.links(narrators))
                                .font(.geist(.footnote))
                                .tint(Theme.foreground)
                                .accessibilityIdentifier("book.detail.narrator")
                        }
                        if let progress {
                            FileProgressLine(progress: progress, durationSeconds: listeningLength, pageCount: file.pageCount)
                                .padding(.top, 3)
                        }
                    } else {
                        Label("Not supported in this version of the app", icon: .circleAlert, size: 14, relativeTo: .footnote)
                            .font(.geist(.footnote))
                            .foregroundStyle(Theme.mutedForeground)
                    }
                }
                .frame(maxWidth: .infinity, minHeight: Self.coverSlot, alignment: .leading)
            }
            .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityElement(children: .combine)
            .accessibilityHint("Opens the File's page")
            .accessibilityIdentifier("file.open.\(file.id)")
            .accessibilityActions {
                ForEach(Array(narrators.enumerated()), id: \.offset) { _, narrator in
                    Button(String(localized: "Open \(narrator.1)")) { model.push(narrator.0) }
                }
            }
            actions
        }
        .padding(12)
        .cardStyle()
        .accessibilityElement(children: .contain)
    }

    private var actions: some View {
        Group {
            if isAudiobook {
                // Every audiobook File plays from here; the Listen button
                // above plays the one listened to last.
                Button { model.listen(fileID: file.id) } label: {
                    Group {
                        if playing {
                            PlayingIndicator(playing: true, height: 14)
                        } else {
                            Icon(.headphones, size: 18)
                        }
                    }
                    .foregroundStyle(Theme.primary)
                    .frame(width: 40, height: 40)
                    .background(Theme.primary.opacity(0.12), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
                }
                .buttonStyle(PlayerControlStyle())
                .accessibilityLabel(listenLabel)
                // Editions can share a name, so the File's id tells them apart.
                .accessibilityIdentifier("file.listen.\(file.id)")
            }
        }
    }

    /// The facts the web's row shows under the name: length, pages, an
    /// audiobook's bitrate, year, and size, and whether it's a supplement.
    private var facts: String {
        var parts: [String] = []
        if let seconds = file.durationSeconds, seconds > 0 { parts.append(FileFacts.duration(seconds)) }
        if let pages = file.pageCount, pages > 0 { parts.append(FileFacts.pages(pages)) }
        if let bitrate = file.bitrate, bitrate > 0 { parts.append(FileFacts.bitrate(bitrate)) }
        if let released = file.releaseDate { parts.append(FileFacts.year(released)) }
        parts.append(FileFacts.size(file.sizeBytes))
        if file.fileRole == "supplement" { parts.append(String(localized: "Supplement")) }
        return parts.joined(separator: " · ")
    }
}

/// Each Reading Status as the web app names it.
enum ReadingStatusLabel {
    static func text(_ status: String) -> String {
        switch status {
        case "currently_reading": String(localized: "Currently Reading")
        case "paused": String(localized: "Paused")
        case "to_read": String(localized: "To Read")
        case "read": String(localized: "Read")
        case "did_not_finish": String(localized: "Did Not Finish")
        case "reference": String(localized: "Reference")
        default: status.replacingOccurrences(of: "_", with: " ").capitalized
        }
    }
}

/// An identifier's type as the web app names it.
enum IdentifierLabel {
    static func text(_ type: String) -> String {
        switch type {
        case "isbn_10": "ISBN-10"
        case "isbn_13": "ISBN-13"
        case "asin": "ASIN"
        case "uuid": "UUID"
        case "goodreads": "Goodreads"
        case "google": "Google"
        default: type.replacingOccurrences(of: "_", with: " ").uppercased()
        }
    }
}

/// A Rating in stars, from quarter stars, read only here.
struct RatingStars: View {
    let units: Int

    private var stars: Double { Double(units) / 4 }

    var body: some View {
        HStack(spacing: 2) {
            ForEach(0..<5, id: \.self) { index in
                let fill = min(1, max(0, stars - Double(index)))
                // As the web draws a rating: filled, half filled, or empty.
                ZStack {
                    Icon(fill >= 0.75 ? .starFill : .star, size: 17, relativeTo: .subheadline)
                    if fill >= 0.25, fill < 0.75 {
                        Icon(.starHalfFill, size: 17, relativeTo: .subheadline)
                    }
                }
                .foregroundStyle(Theme.rating)
            }
            Text(stars.formatted(.number.precision(.fractionLength(0...2))))
                .font(.geist(.subheadline))
                .foregroundStyle(Theme.mutedForeground)
                .padding(.leading, 4)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(String(localized: "Rated \(stars.formatted(.number.precision(.fractionLength(0...2)))) of 5 stars"))
        .accessibilityIdentifier("book.detail.rating")
    }
}

/// Links that wrap onto as many lines as they need, each opening its page.
struct LinkChips: View {
    let items: [(Route, String)]
    let identifier: String
    @State private var expanded = false

    /// As many as show before the rest collapse, as on the web: sources
    /// like MangaUpdates attach dozens of tags, which would push the rest of
    /// the page far down. A row a couple past it shows whole, so the button
    /// never hides just one or two.
    private static let collapsed = 12
    private static let slack = 2

    private var collapsible: Bool { items.count > Self.collapsed + Self.slack }

    var body: some View {
        let shown = collapsible && !expanded ? Array(items.prefix(Self.collapsed)) : items
        FlowLayout(spacing: 6) {
            ForEach(Array(shown.enumerated()), id: \.offset) { _, item in
                NavigationLink(value: item.0) {
                    CountBadge(text: item.1)
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier(identifier)
            }
            if collapsible {
                Button {
                    withAnimation(.easeOut(duration: 0.2)) { expanded.toggle() }
                } label: {
                    Text(expanded ? String(localized: "Show less") : String(localized: "+\(items.count - Self.collapsed) more"))
                        .font(.geist(.footnote, weight: .medium))
                        .foregroundStyle(Theme.primary)
                        .padding(.vertical, 4)
                        .hitArea(height: 26)
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("\(identifier).more")
            }
        }
    }
}

/// Names that link to their pages in running text: the Book's byline, as
/// the web's Book page writes it ("by Fujita (Writer, Penciller),
/// AndWorld Design (Letterer)"), and lists of Series or narrators.
enum Byline {
    /// Each creator once, with their roles, linked to their page.
    static func text(_ authors: [BookDetail.Author]) -> AttributedString {
        var order: [Int] = []
        var names: [Int: String] = [:]
        var roles: [Int: [String]] = [:]
        for author in authors {
            if names[author.personID] == nil {
                order.append(author.personID)
                names[author.personID] = author.name
            }
            if let role = author.role, !(roles[author.personID] ?? []).contains(role) {
                roles[author.personID, default: []].append(role)
            }
        }
        var text = LinkedText.muted(String(localized: "by "))
        for (index, id) in order.enumerated() {
            text += LinkedText.link(names[id] ?? "", to: .resource(.person, id: id, name: names[id] ?? ""))
            if let personRoles = roles[id], !personRoles.isEmpty {
                text += LinkedText.muted(" (" + personRoles.map(roleLabel).joined(separator: ", ") + ")")
            }
            if index < order.count - 1 {
                text += LinkedText.muted(", ")
            }
        }
        return text
    }

    /// Linked names separated by commas.
    static func links(_ items: [(Route, String)]) -> AttributedString {
        var text = AttributedString()
        for (index, item) in items.enumerated() {
            text += LinkedText.link(item.1, to: item.0)
            if index < items.count - 1 {
                text += LinkedText.muted(", ")
            }
        }
        return text
    }

    /// A creator's role as the web app names it.
    static func roleLabel(_ role: String) -> String {
        switch role {
        case "writer": String(localized: "Writer")
        case "penciller": String(localized: "Penciller")
        case "inker": String(localized: "Inker")
        case "colorist": String(localized: "Colorist")
        case "letterer": String(localized: "Letterer")
        case "cover_artist": String(localized: "Cover Artist")
        case "editor": String(localized: "Editor")
        case "translator": String(localized: "Translator")
        default: role.replacingOccurrences(of: "_", with: " ").capitalized
        }
    }
}

/// Lays its views out in rows, wrapping to the next as a line of text does.
struct FlowLayout: Layout {
    var spacing: CGFloat = 6

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let rows = arrange(width: proposal.width ?? .infinity, subviews: subviews)
        let height = rows.map(\.height).reduce(0, +) + spacing * CGFloat(max(0, rows.count - 1))
        let width = rows.map(\.width).max() ?? 0
        return CGSize(width: proposal.width ?? width, height: height)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var y = bounds.minY
        for row in arrange(width: bounds.width, subviews: subviews) {
            var x = bounds.minX
            for index in row.indices {
                var size = subviews[index].sizeThatFits(.unspecified)
                size.width = min(size.width, bounds.width)
                // Centered in the row, so a label lines up with chips.
                subviews[index].place(at: CGPoint(x: x, y: y + (row.height - size.height) / 2), proposal: ProposedViewSize(size))
                x += size.width + spacing
            }
            y += row.height + spacing
        }
    }

    private struct Row {
        var indices: [Int] = []
        var width: CGFloat = 0
        var height: CGFloat = 0
    }

    private func arrange(width: CGFloat, subviews: Subviews) -> [Row] {
        var rows: [Row] = []
        var row = Row()
        for index in subviews.indices {
            var size = subviews[index].sizeThatFits(.unspecified)
            size.width = min(size.width, width)
            if !row.indices.isEmpty, row.width + spacing + size.width > width {
                rows.append(row)
                row = Row()
            }
            row.width += (row.indices.isEmpty ? 0 : spacing) + size.width
            row.height = max(row.height, size.height)
            row.indices.append(index)
        }
        if !row.indices.isEmpty {
            rows.append(row)
        }
        return rows
    }
}
