import SwiftUI

/// A File's own page, as the web's: its cover, name, the Book it's from, its
/// narrators and facts, Listen, what edition it is, its chapters, and where
/// it sits on disk. It's all in the device's copy, so it works offline.
///
/// Left out from the web's page: the Book's other Files, a step back on the
/// Book page; a comic's or PDF's page grid, which waits for the reader's page
/// images (#921); and links out from identifiers, whose pages plugins define.
struct FileView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.horizontalSizeClass) private var sizeClass
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let bookID: Int
    let fileID: Int

    @State private var book: BookDetail??
    @State private var chapters: [FileChapter]?
    @State private var listen: ListenProgress??
    @State private var tint: Color?
    @State private var scrolled = false
    @State private var download: DownloadRow??

    private var file: BookDetail.File? { (book ?? nil)?.files.first { $0.id == fileID } }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                ScrollTopMarker(space: "file")
                if let detail = book ?? nil, let file {
                    content(detail, file)
                } else if book != nil {
                    EmptyNotice(title: String(localized: "This File isn't in your Libraries anymore"), icon: .fileX)
                        .padding(.top, 80)
                }
            }
            .padding(.horizontal, 20)
            .padding(.bottom, 40)
            .frame(maxWidth: 680)
            .frame(maxWidth: .infinity)
        }
        .environment(\.openURL, OpenURLAction { url in
            if let id = LinkedText.bookID(url), let detail = book ?? nil, detail.summary.id == id {
                model.returnToBook(detail.summary)
                return .handled
            }
            guard let route = LinkedText.route(url) else { return .systemAction }
            model.push(route)
            return .handled
        })
        .refreshable { await model.syncNow() }
        .miniPlayerClearance()
        .onScrolledDistanceChange(space: "file") { distance in
            let collapsed = distance > 200
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
                Text(file?.label ?? "")
                    .font(.geist(.headline, weight: .semibold))
                    .lineLimit(1)
                    .opacity(scrolled ? 1 : 0)
                    .accessibilityHidden(!scrolled)
            }
        }
        .observing(model.session?.store, id: bookID, into: $book) { [bookID] db in
            try CatalogStore.book(db, id: bookID)
        }
        .observing(model.session?.store, id: fileID, into: $chapters) { [fileID] db in
            try CatalogStore.fileChapters(db, fileID: fileID)
        }
        .observing(model.session?.store, id: fileID, into: $download) { [fileID] db in
            try CatalogStore.download(db, fileID: fileID)
        }
        .observing(model.session?.store, id: fileID, into: $listen) { [bookID, fileID, deviceID = model.session?.account.deviceID ?? ""] db in
            // Only an audiobook has a place to listen from.
            let type = try String.fetchOne(db, sql: "SELECT file_type FROM book_files WHERE id = ?", arguments: [fileID])
            guard type == "m4b" else { return nil }
            return try CatalogStore.listenProgress(db, bookID: bookID, fileID: fileID, ownDeviceID: deviceID)
        }
    }

    /// The cover's color, washed from the top, as on the Book page.
    private var backdrop: some View {
        LinearGradient(
            stops: [
                .init(color: (tint ?? Theme.primary).opacity(tint == nil ? 0.08 : 0.42), location: 0),
                .init(color: (tint ?? Theme.primary).opacity(tint == nil ? 0.03 : 0.16), location: 0.55),
                .init(color: Theme.background.opacity(0), location: 1),
            ],
            startPoint: .top, endPoint: .bottom
        )
        .frame(height: 600)
        .ignoresSafeArea(edges: .top)
        .animation(.easeOut(duration: 0.5), value: tint)
        .accessibilityHidden(true)
    }

    private func content(_ book: BookDetail, _ file: BookDetail.File) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            hero(book, file)
            fileActions(file)
            if let progress = listen ?? nil {
                ListenCard(progress: progress, bookID: book.summary.id)
                    .modifier(NeedsConnection(available: model.canOpen(fileID: progress.fileID)))
                    .padding(.top, 22)
            }
            let edition = editionRows(file)
            if !edition.isEmpty {
                LabelRows(rows: edition)
                    .padding(.top, 24)
            }
            chaptersSection(file)
            FileSectionHeading(text: String(localized: "On disk"))
            LabelRows(rows: diskRows(file))
                .accessibilityIdentifier("file.disk")
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func hero(_ book: BookDetail, _ file: BookDetail.File) -> some View {
        let supplement = file.fileRole == "supplement"
        // A supplement is the lesser File, so its picture is smaller, and an
        // audiobook's square cover is drawn a little narrower than a book's.
        let width: CGFloat = supplement ? 120 : file.fileType == "m4b" ? (sizeClass == .regular ? 240 : 190) : (sizeClass == .regular ? 220 : 170)
        return VStack(spacing: 0) {
            CoverView(book: book.summary, file: CoverFile(id: file.id, fileType: file.fileType, coverVersion: file.coverVersion),
                      width: width, shadow: .large) { image in
                let average = await CoverTint.average(of: image)
                guard !Task.isCancelled else { return }
                tint = average?.color
            }
            .padding(.top, 8)

            Text(file.label)
                .font(.geist(size: 28, relativeTo: .title, weight: .bold))
                .tracking(-0.4)
                .multilineTextAlignment(.center)
                .padding(.top, 20)
                .accessibilityAddTraits(.isHeader)
                .accessibilityIdentifier("file.title")

            Text(fromLine(book, supplement: supplement))
                .font(.geist(.callout))
                .multilineTextAlignment(.center)
                .tint(Theme.foreground)
                .padding(.top, 8)
                .accessibilityIdentifier("file.from")

            if !file.narrators.isEmpty {
                Text(LinkedText.muted(String(localized: "Narrated by "))
                    + Byline.links(zip(file.narratorIDs, file.narrators).map { (Route.resource(.person, id: $0.0, name: $0.1), $0.1) }))
                    .font(.geist(.callout))
                    .multilineTextAlignment(.center)
                    .tint(Theme.foreground)
                    .padding(.top, 4)
                    .accessibilityIdentifier("file.narrators")
            }

            HStack(alignment: .center, spacing: 8) {
                FileTypeChip(fileType: file.fileType, outlined: true)
                Text(file.headerFacts.joined(separator: " · "))
                    .font(.geist(.footnote))
                    .foregroundStyle(Theme.mutedForeground)
                    .monospacedDigit()
            }
            .padding(.top, 10)
            .accessibilityElement(children: .combine)
        }
        .frame(maxWidth: .infinity)
    }

    /// "From <Book> by <authors>", or "Supplement to <Book>", as the web's
    /// File page says where it belongs.
    private func fromLine(_ book: BookDetail, supplement: Bool) -> AttributedString {
        var line = LinkedText.muted(supplement ? String(localized: "Supplement to ") : String(localized: "From "))
        line += LinkedText.bookLink(book.summary.title, id: book.summary.id)
        // Each author once by name, without roles, as the web's File page
        // names them.
        var seen = Set<Int>()
        let authors = book.authors.filter { seen.insert($0.personID).inserted }
        if !supplement, !authors.isEmpty {
            line += LinkedText.muted(String(localized: " by "))
                + Byline.links(authors.map { (Route.resource(.person, id: $0.personID, name: $0.name), $0.name) })
        }
        return line
    }

    // MARK: - Download and share

    /// Download, or where its Download stands, and Share, which sends the
    /// File out with Shisho's current metadata when the server offers it.
    @ViewBuilder
    private func fileActions(_ file: BookDetail.File) -> some View {
        let supported = FileTypes.isKnown(file.fileType)
        let downloads = model.session?.downloads
        let status = downloads?.status(download ?? nil) ?? .notDownloaded
        // Side by side, or one over the other at accessibility text sizes.
        let layout = dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(spacing: 10)) : AnyLayout(HStackLayout(spacing: 10))
        VStack(spacing: 10) {
            layout {
                Group {
                    switch status {
                    case .notDownloaded, .failed:
                        Button { downloads?.download([file.id]) } label: {
                            Label(verbatim: supported ? String(localized: "Download") : String(localized: "Not supported"), icon: .download)
                        }
                        .disabled(!supported)
                        .modifier(NeedsConnection(available: !supported || model.connection == .online))
                        .accessibilityLabel(supported ? String(localized: "Download \(file.label)")
                                            : String(localized: "Download \(file.label), not supported in this version of the app"))
                    case .waiting, .running:
                        Button { downloads?.remove([file.id]) } label: {
                            Label("Stop Download", icon: .x)
                        }
                        .accessibilityLabel(String(localized: "Stop downloading \(file.label)"))
                    case .downloaded:
                        Button(role: .destructive) { downloads?.remove([file.id]) } label: {
                            // Short, as it shares the row with Share; the line
                            // under them says it's downloaded.
                            Label("Remove", icon: .trash)
                        }
                        .accessibilityLabel(String(localized: "Remove the Download of \(file.label)"))
                    }
                }
                .buttonStyle(OutlineButtonStyle(role: status == .downloaded ? .destructive : nil))
                .accessibilityValue(DownloadWords.status(status) ?? "")
                .accessibilityIdentifier("file.download")
                if supported, model.session?.sharesFiles == true {
                    ShareFileButton(fileID: file.id, label: file.label)
                        // The server makes the copy it shares.
                        .modifier(NeedsConnection(available: model.connection == .online))
                }
            }
            DownloadStatusLine(fileID: file.id, row: download ?? nil)
                .labelStyle(.titleAndIcon)
                .frame(maxWidth: .infinity, alignment: .center)
        }
        // The buttons' icons go above their titles at accessibility sizes.
        .labelStyle(ActionLabelStyle())
        .padding(.top, 20)
    }

    // MARK: - Chapters

    @ViewBuilder
    private func chaptersSection(_ file: BookDetail.File) -> some View {
        let chapters = chapters ?? []
        // A supplement rarely has chapters, so it shows them only when it does.
        if !(file.fileRole == "supplement" && chapters.isEmpty) {
            FileSectionHeading(text: chapters.isEmpty ? String(localized: "Chapters") : String(AttributedString(localized: "^[\(chapters.count) chapter](inflect: true)").characters))
            Group {
                if chapters.isEmpty {
                    Text("No chapters")
                        .font(.geist(.subheadline))
                        .foregroundStyle(Theme.mutedForeground)
                        .frame(maxWidth: .infinity, minHeight: 72)
                } else {
                    // Lazy, since an EPUB can have thousands.
                    LazyVStack(spacing: 0) {
                        ForEach(Array(chapters.enumerated()), id: \.element.id) { index, chapter in
                            if index > 0 {
                                Divider().overlay(Theme.border)
                            }
                            ChapterRow(chapter: chapter, file: file)
                        }
                    }
                }
            }
            .cardStyle()
            .accessibilityIdentifier("file.chapters")
        }
    }

    // MARK: - Rows

    /// What edition the File is, as the web's File page lists it under its
    /// name: publisher, release date, language, Reading Direction, abridged,
    /// identifiers, and its URL.
    private func editionRows(_ file: BookDetail.File) -> [LabelRows.Row] {
        var rows: [LabelRows.Row] = []
        if let publisher = file.publisher { rows.append(.init(String(localized: "Publisher"), publisher)) }
        if let released = file.releaseDate {
            // A release date is a day, read in UTC as the web reads it.
            var style = Date.FormatStyle(date: .abbreviated, time: .omitted)
            style.timeZone = TimeZone(identifier: "UTC") ?? .current
            rows.append(.init(String(localized: "Released"), released.formatted(style)))
        }
        if let language = file.language {
            rows.append(.init(String(localized: "Language"), Locale.current.localizedString(forIdentifier: language) ?? language))
        }
        // Left to right is how nearly every book runs, so only right to left
        // says something.
        if file.readingDirection == "rtl" {
            rows.append(.init(String(localized: "Reading Direction"), String(localized: "Right to left")))
        }
        // Audiobooks were often sold abridged, so an M4B always says which.
        if file.fileType == "m4b" || file.abridged == true {
            rows.append(.init(String(localized: "Abridged"),
                              file.abridged.map { $0 ? String(localized: "Yes") : String(localized: "No") } ?? String(localized: "Unknown")))
        }
        for identifier in file.identifiers {
            rows.append(.init(IdentifierLabel.text(identifier.type), identifier.value, monospaced: true))
        }
        if let link = file.url, let url = URL(string: link) {
            rows.append(.init(String(localized: "URL"), url.host()?.replacingOccurrences(of: "www.", with: "") ?? link, url: url))
        }
        return rows
    }

    /// Where the File sits and its format's technical facts, as the web's
    /// On disk section lists them.
    private func diskRows(_ file: BookDetail.File) -> [LabelRows.Row] {
        var rows: [LabelRows.Row] = [
            .init(String(localized: "Format"), FileTypes.label(file.fileType)),
            .init(String(localized: "Size"), FileFacts.size(file.sizeBytes)),
        ]
        if let pages = file.pageCount, pages > 0 { rows.append(.init(String(localized: "Pages"), FileFacts.pages(pages))) }
        if let seconds = file.durationSeconds, seconds > 0 { rows.append(.init(String(localized: "Duration"), FileFacts.duration(seconds))) }
        // Exact here, where the web gives it on hover.
        if let words = file.wordCount, words > 0 { rows.append(.init(String(localized: "Words"), FileFacts.exactWords(words))) }
        if let codec = file.codec { rows.append(.init(String(localized: "Codec"), codec)) }
        if let bitrate = file.bitrate, bitrate > 0 { rows.append(.init(String(localized: "Bitrate"), FileFacts.bitrate(bitrate))) }
        if !file.filepath.isEmpty {
            let path = file.filepath as NSString
            rows.append(.init(String(localized: "File"), path.lastPathComponent, monospaced: true))
            let folder = path.deletingLastPathComponent
            rows.append(.init(String(localized: "Folder"), folder.isEmpty ? "/" : folder, monospaced: true, muted: true))
        }
        if let modified = file.modifiedAt {
            rows.append(.init(String(localized: "Modified"), modified.formatted(date: .abbreviated, time: .shortened)))
        }
        if let added = file.addedAt {
            rows.append(.init(String(localized: "Added"), added.formatted(date: .abbreviated, time: .shortened)))
        }
        return rows
    }
}

extension BookDetail.File {
    /// The File's label among its Book's Files, as the web's `fileLabel`.
    var label: String { displayName.isEmpty ? FileTypes.label(fileType) : displayName }

    /// The quiet line under the File's name on its page: length, pages or
    /// words, the audio's codec and bitrate, and size, as the web's
    /// `fileHeaderFacts`.
    var headerFacts: [String] {
        var facts: [String] = []
        if let seconds = durationSeconds, seconds > 0 { facts.append(FileFacts.duration(seconds)) }
        if let pages = pageCount, pages > 0 { facts.append(FileFacts.pages(pages)) }
        if let words = wordCount, words > 0 { facts.append(FileFacts.words(words)) }
        if let codec { facts.append(codec) }
        if let bitrate, bitrate > 0 { facts.append(FileFacts.bitrate(bitrate)) }
        facts.append(FileFacts.size(sizeBytes))
        return facts
    }
}

/// A section's quiet heading, as the web's File page heads its sections.
private struct FileSectionHeading: View {
    let text: String

    var body: some View {
        Text(text)
            .font(.geist(.subheadline))
            .foregroundStyle(Theme.mutedForeground)
            .padding(.top, 32)
            .padding(.bottom, 10)
            .accessibilityAddTraits(.isHeader)
    }
}

/// Labels and their values side by side, as the web's `LabelGrid`, or the
/// label over its value at accessibility text sizes.
struct LabelRows: View {
    struct Row: Identifiable {
        let label: String
        let value: String
        var url: URL?
        var monospaced = false
        var muted = false
        var id: String { label + value }

        init(_ label: String, _ value: String, url: URL? = nil, monospaced: Bool = false, muted: Bool = false) {
            self.label = label
            self.value = value
            self.url = url
            self.monospaced = monospaced
            self.muted = muted
        }
    }

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let rows: [Row]

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            let stacked = dynamicTypeSize.isAccessibilitySize
            let layout = stacked
                ? AnyLayout(VStackLayout(alignment: .leading, spacing: 4))
                : AnyLayout(HStackLayout(alignment: .firstTextBaseline, spacing: 12))
            ForEach(rows) { row in
                layout {
                    // Capitals only on screen; VoiceOver reads the word.
                    Text(row.label.uppercased())
                        .accessibilityLabel(row.label)
                        .font(.geist(.caption, weight: .semibold))
                        .tracking(0.8)
                        .foregroundStyle(Theme.mutedForeground)
                        .frame(width: stacked ? nil : 104, alignment: .leading)
                    Group {
                        if let url = row.url {
                            Link(row.value, destination: url)
                                .underline()
                                .foregroundStyle(Theme.foreground)
                                .accessibilityIdentifier("file.url")
                        } else {
                            Text(row.value)
                                .foregroundStyle(row.muted ? Theme.mutedForeground : Theme.foreground)
                                .textSelection(.enabled)
                        }
                    }
                    .font(row.monospaced ? .geistMono(.subheadline) : .geist(.subheadline))
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
                .accessibilityElement(children: .combine)
            }
        }
    }
}

/// One of the File's chapters: an audiobook's plays from its start, and a
/// comic's or PDF's says which page it starts on. An EPUB's are indented
/// under the chapters they belong to.
private struct ChapterRow: View {
    @Environment(AppModel.self) private var model
    let chapter: FileChapter
    let file: BookDetail.File

    var body: some View {
        if file.fileType == "m4b", let start = chapter.startSeconds {
            Button { model.listen(fileID: file.id, from: start) } label: {
                HStack(alignment: .center, spacing: 10) {
                    title
                    Icon(.play, size: 14, relativeTo: nil)
                        .foregroundStyle(Theme.mutedForeground)
                    Text(verbatim: TimeLabels.clock(start))
                        .font(.geistMono(.footnote))
                        .foregroundStyle(Theme.mutedForeground)
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 12)
                .frame(minHeight: 46)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(chapter.title)
            .accessibilityValue(TimeLabels.clock(start))
            .accessibilityHint("Plays from this chapter")
            .accessibilityIdentifier("file.chapter")
        } else {
            HStack(alignment: .center, spacing: 10) {
                title
                if let page = chapter.startPage, file.fileType == "cbz" || file.fileType == "pdf" {
                    Text("Page \(page + 1)")
                        .font(.geist(.footnote))
                        .monospacedDigit()
                        .foregroundStyle(Theme.mutedForeground)
                }
            }
            .padding(.leading, 14 + CGFloat(min(chapter.depth, 6)) * 16)
            .padding(.trailing, 14)
            .padding(.vertical, 12)
            .frame(minHeight: 46)
            .accessibilityElement(children: .combine)
            // How deep it sits, which the indent shows on screen.
            .accessibilityValue(chapter.depth > 0 ? String(localized: "Level \(chapter.depth + 1)") : "")
            .accessibilityIdentifier("file.chapter")
        }
    }

    private var title: some View {
        Text(chapter.title)
            .font(.geist(.subheadline))
            .foregroundStyle(Theme.foreground)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}
