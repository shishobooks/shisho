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
    @State private var tint: Color?
    @State private var scrolled = false

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
                tint = average.map { Color(red: $0.red, green: $0.green, blue: $0.blue) }
            }
            .padding(.top, 8)

            if let detail = book ?? nil, let series = detail.series.first {
                HStack(spacing: 8) {
                    if let number = series.number {
                        SeriesNumberBadge(number: number)
                    }
                    Text(series.name)
                        .font(.geist(.callout, weight: .semibold))
                }
                .padding(.top, 22)
                .accessibilityElement(children: .combine)
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
        if !book.series.isEmpty {
            let text = book.series.map(seriesLine).joined(separator: ", ")
            rows.append(InfoRow(label: String(localized: "SERIES"), value: AnyView(Text(text).font(.geist(.subheadline)))))
        }
        let credited = book.authors.filter { $0.role != nil }
        if !credited.isEmpty {
            let text = credited.map { "\($0.name) (\(roleLabel($0.role ?? "")))" }.joined(separator: ", ")
            rows.append(InfoRow(label: String(localized: "CREATORS"), value: AnyView(Text(text).font(.geist(.subheadline)))))
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
            return String(localized: "\(series.name) #\(SeriesNumbers.format(number))–\(SeriesNumbers.format(end))")
        }
        return String(localized: "\(series.name) #\(SeriesNumbers.format(number))")
    }

    private func roleLabel(_ role: String) -> String {
        role.replacingOccurrences(of: "_", with: " ").capitalized
    }
}

/// One of a Book's Files, as the web app's file card shows it.
private struct FileCard: View {
    let file: BookDetail.File
    let book: BookSummary

    var body: some View {
        HStack(alignment: .center, spacing: 14) {
            CoverView(book: book, width: 46)
            VStack(alignment: .leading, spacing: 6) {
                // Each File by its own label, as the web app's file list does;
                // the Book's title would name every File the same.
                Text(file.displayName.isEmpty ? FileTypes.label(file.fileType) : file.displayName)
                    .font(.geist(.subheadline, weight: .semibold))
                    .lineLimit(2)
                if FileTypes.isKnown(file.fileType) {
                    HStack(spacing: 8) {
                        FileTypeChip(fileType: file.fileType, outlined: true)
                        Text(ByteCountFormatter.string(fromByteCount: Int64(file.sizeBytes), countStyle: .file))
                            .font(.geist(.footnote))
                            .foregroundStyle(Theme.mutedForeground)
                            .monospacedDigit()
                        if file.fileRole == "supplement" {
                            Text("Supplement")
                                .font(.geist(.footnote))
                                .foregroundStyle(Theme.mutedForeground)
                        }
                    }
                } else {
                    Label("Not supported in this version of the app", systemImage: "exclamationmark.circle")
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                }
            }
            Spacer(minLength: 0)
        }
        .padding(14)
        .cardStyle()
        .accessibilityElement(children: .combine)
    }
}
