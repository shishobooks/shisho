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
    @State private var showingFullDescription = false

    private var summary: BookSummary {
        book??.summary ?? initial
    }

    var body: some View {
        ScrollView {
            VStack(spacing: 0) {
                ScrollTopMarker(space: "book")
                if case .some(.none) = book {
                    ContentUnavailableView("This Book isn't in your Libraries anymore", systemImage: "book.closed")
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
                }
                .buttonStyle(.plain)
                .padding(.top, 22)
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

            if !summary.authors.isEmpty {
                (Text("by ").foregroundStyle(Theme.mutedForeground) + Text(summary.authors).underline(color: Theme.mutedForeground.opacity(0.5)))
                    .font(.geist(.callout))
                    .multilineTextAlignment(.center)
                    .padding(.top, 10)
                    .accessibilityLabel(String(localized: "By \(summary.authors)"))
                    .accessibilityIdentifier("book.detail.authors")
            }
        }
    }

    private func details(_ book: BookDetail) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            if let description = book.description, !description.isEmpty {
                VStack(alignment: .leading, spacing: 6) {
                    Text(PlainText.from(description))
                        .font(.geist(.subheadline))
                        .lineLimit(showingFullDescription ? nil : 5)
                        .accessibilityIdentifier("book.detail.description")
                    Button(showingFullDescription ? String(localized: "Show less") : String(localized: "Show more")) {
                        withAnimation(.easeInOut(duration: 0.25)) { showingFullDescription.toggle() }
                    }
                    .font(.geist(.subheadline, weight: .semibold))
                    .accessibilityIdentifier("book.detail.description.more")
                }
                .padding(.top, 28)
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
                            Text(row.label)
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
                .padding(.top, 34)
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
                        FileCard(file: file, book: book.summary)
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private struct InfoRow {
        let label: String
        let value: AnyView
    }

    private func infoRows(_ book: BookDetail) -> [InfoRow] {
        var rows: [InfoRow] = []
        if let status = standing?.status {
            rows.append(InfoRow(label: String(localized: "STATUS"), value: AnyView(
                Text(ReadingStatusLabel.text(status)).font(.geist(.subheadline, weight: .medium))
                    .accessibilityIdentifier("book.detail.status")
            )))
        }
        if let rating = standing?.rating {
            rows.append(InfoRow(label: String(localized: "RATING"), value: AnyView(RatingStars(units: rating))))
        }
        if !book.series.isEmpty {
            rows.append(InfoRow(label: String(localized: "SERIES"), value: AnyView(LinkChips(items: book.series.map {
                (Route.resource(.series, id: $0.seriesID, name: $0.name), seriesLine($0))
            }, identifier: "book.detail.series.link"))))
        }
        let uncredited = book.authors.filter { $0.role == nil }
        if !uncredited.isEmpty {
            rows.append(InfoRow(label: String(localized: "AUTHORS"), value: AnyView(LinkChips(items: uncredited.map {
                (Route.resource(.person, id: $0.personID, name: $0.name), $0.name)
            }, identifier: "book.detail.author"))))
        }
        let credited = book.authors.filter { $0.role != nil }
        if !credited.isEmpty {
            rows.append(InfoRow(label: String(localized: "CREATORS"), value: AnyView(LinkChips(items: credited.map {
                (Route.resource(.person, id: $0.personID, name: $0.name), "\($0.name) (\(roleLabel($0.role ?? "")))")
            }, identifier: "book.detail.creator"))))
        }
        if !book.genres.isEmpty {
            rows.append(InfoRow(label: String(localized: "GENRES"), value: AnyView(LinkChips(items: book.genres.map {
                (Route.resource(.genre, id: $0.id, name: $0.name), $0.name)
            }, identifier: "book.detail.genre"))))
        }
        if !book.tags.isEmpty {
            rows.append(InfoRow(label: String(localized: "TAGS"), value: AnyView(LinkChips(items: book.tags.map {
                (Route.resource(.tag, id: $0.id, name: $0.name), $0.name)
            }, identifier: "book.detail.tag"))))
        }
        if !book.summary.fileTypes.isEmpty {
            rows.append(InfoRow(label: String(localized: "FORMATS"), value: AnyView(
                HStack(spacing: 6) { ForEach(book.summary.fileTypes, id: \.self) { FileTypeChip(fileType: $0) } }
                    .accessibilityElement(children: .ignore)
                    .accessibilityLabel(book.summary.fileTypes.map(FileTypes.label).joined(separator: ", "))
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

    private func roleLabel(_ role: String) -> String {
        role.replacingOccurrences(of: "_", with: " ").capitalized
    }
}

/// One of a Book's Files, as the web app's file card shows it.
private struct FileCard: View {
    @Environment(AppModel.self) private var model
    let file: BookDetail.File
    let book: BookSummary

    private var isAudiobook: Bool { file.fileType == "m4b" }

    /// Names the File and its narrators, since editions often share a name.
    private var listenLabel: String {
        let name = file.displayName.isEmpty ? FileTypes.label(file.fileType) : file.displayName
        guard !file.narrators.isEmpty else { return String(localized: "Listen to \(name)") }
        return String(localized: "Listen to \(name), narrated by \(file.narrators.joined(separator: ", "))")
    }
    private var playing: Bool { model.player?.file?.fileID == file.id && model.player?.isPlaying == true }

    var body: some View {
        HStack(alignment: .center, spacing: 14) {
            HStack(alignment: .center, spacing: 14) {
                CoverView(book: book, file: CoverFile(id: file.id, fileType: file.fileType, coverVersion: file.coverVersion), width: 46)
                VStack(alignment: .leading, spacing: 6) {
                    // Each File by its own label, as the web app's file list does;
                    // the Book's title would name every File the same.
                    Text(file.displayName.isEmpty ? FileTypes.label(file.fileType) : file.displayName)
                        .font(.geist(.subheadline, weight: .semibold))
                        .lineLimit(2)
                    if FileTypes.isKnown(file.fileType) {
                        HStack(spacing: 8) {
                            FileTypeChip(fileType: file.fileType, outlined: true)
                            Text(details)
                                .font(.geist(.footnote))
                                .foregroundStyle(Theme.mutedForeground)
                                .monospacedDigit()
                        }
                        if !file.narrators.isEmpty {
                            Text("Narrated by \(file.narrators.joined(separator: ", "))")
                                .font(.geist(.footnote))
                                .foregroundStyle(Theme.mutedForeground)
                                .lineLimit(2)
                        }
                        if let publisher = file.publisher {
                            Text("Published by \(publisher)")
                                .font(.geist(.footnote))
                                .foregroundStyle(Theme.mutedForeground)
                                .lineLimit(1)
                        }
                        ForEach(file.identifiers, id: \.self) { identifier in
                            Text("\(IdentifierLabel.text(identifier.type)) \(identifier.value)")
                                .font(.geist(.caption))
                                .foregroundStyle(Theme.mutedForeground)
                                .textSelection(.enabled)
                                .lineLimit(1)
                        }
                    } else {
                        Label("Not supported in this version of the app", systemImage: "exclamationmark.circle")
                            .font(.geist(.footnote))
                            .foregroundStyle(Theme.mutedForeground)
                    }
                }
                Spacer(minLength: 0)
            }
            .accessibilityElement(children: .combine)
            if isAudiobook {
                // Every audiobook File plays from here; the Listen button
                // above plays the one listened to last.
                Button { model.listen(fileID: file.id) } label: {
                    Group {
                        if playing {
                            PlayingIndicator(playing: true, height: 14)
                        } else {
                            Image(systemName: "headphones")
                                .font(.system(size: 16, weight: .semibold))
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
        .padding(14)
        .cardStyle()
        .accessibilityElement(children: .contain)
    }

    /// Its length for an audiobook, its size, and whether it's a supplement.
    private var details: String {
        var parts: [String] = []
        if isAudiobook, let seconds = file.durationSeconds, seconds > 0 {
            parts.append(TimeLabels.left(seconds))
        }
        parts.append(ByteCountFormatter.string(fromByteCount: Int64(file.sizeBytes), countStyle: .file))
        if file.fileRole == "supplement" {
            parts.append(String(localized: "Supplement"))
        }
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
                Image(systemName: fill >= 0.75 ? "star.fill" : fill >= 0.25 ? "star.leadinghalf.filled" : "star")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundStyle(Theme.primary)
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

    var body: some View {
        FlowLayout(spacing: 6) {
            ForEach(Array(items.enumerated()), id: \.offset) { _, item in
                NavigationLink(value: item.0) {
                    Text(item.1)
                        .font(.geist(.subheadline, weight: .medium))
                        .foregroundStyle(Theme.foreground)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 4)
                        .background(Theme.secondary, in: RoundedRectangle(cornerRadius: 6, style: .continuous))
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier(identifier)
            }
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
                let size = subviews[index].sizeThatFits(.unspecified)
                subviews[index].place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(size))
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
