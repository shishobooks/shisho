import SwiftUI

/// Now Playing, presented as a sheet over the screen it came from. Variant
/// E's most expressive screen, in the web app's design language: the cover's
/// color washes it, the artwork is large, and the controls are big. Under
/// the transport sit speed, the sleep timer, Bookmark, and the chapter list;
/// the rest of the player's settings are in the header's menu.
struct NowPlayingView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var tint: Color?
    /// Every author and narrator shows, in a box of its own.
    @State private var showingAllPeople = false
    /// How tall the box of every author and narrator grows before it
    /// scrolls, so a long cast never pushes the controls down.
    @ScaledMetric(relativeTo: .subheadline) private var peopleBoxHeight: CGFloat = 72
    /// The chapters and Bookmarks sheet, open at a tab.
    @State private var list: PlayerListSheet.Tab?
    /// The Bookmark just added, which the toast offers a note for.
    @State private var added: Bookmark?
    @State private var noting: Bookmark?
    /// How tall the notices over the artwork are, which the artwork gives
    /// up, so the tools stay on screen.
    @State private var noticesHeight: CGFloat = 0
    @State private var note = ""

    var body: some View {
        Group {
            if let player = model.player, let file = player.file {
                content(player, file)
                    .overlay(alignment: .top) { toast(player) }
            } else {
                EmptyNotice(title: String(localized: "Nothing is playing"), icon: .headphones)
            }
        }
        .background(alignment: .top) { backdrop }
        .background(Theme.background)
        .sheet(item: $list) { tab in
            if let player = model.player {
                PlayerListSheet(player: player, tab: tab)
            }
        }
        .alert("Add a Note", isPresented: Binding(get: { noting != nil }, set: { if !$0 { noting = nil } })) {
            TextField("Note", text: $note)
                .accessibilityIdentifier("bookmark.note")
            Button("Save") {
                if let noting { model.player?.editBookmark(noting, note: note) }
                noting = nil
            }
            .accessibilityIdentifier("bookmark.saveNote")
            Button("Cancel", role: .cancel) { noting = nil }
                .accessibilityIdentifier("bookmark.cancelNote")
        } message: {
            Text("At \(TimeLabels.clock(noting?.seconds ?? 0))")
        }
        .presentationDragIndicator(.visible)
        .presentationCornerRadius(28)
    }

    /// The cover's color, strong at the top and fading into the page.
    private var backdrop: some View {
        LinearGradient(
            stops: [
                .init(color: (tint ?? Theme.primary).opacity(tint == nil ? 0.12 : 0.55), location: 0),
                .init(color: (tint ?? Theme.primary).opacity(tint == nil ? 0.04 : 0.2), location: 0.55),
                .init(color: Theme.background.opacity(0), location: 1),
            ],
            startPoint: .top, endPoint: .bottom
        )
        .ignoresSafeArea()
        .animation(.easeOut(duration: 0.5), value: tint)
        .accessibilityHidden(true)
    }

    private func content(_ player: AudioPlayer, _ file: ListeningFile) -> some View {
        GeometryReader { geometry in
            // When it all fits, the page fills the sheet, so the controls
            // can center in the room left and the tools sit at the bottom. A
            // scroll view would size it to its content instead, so it
            // scrolls only when it doesn't fit, as at large text sizes.
            ViewThatFits(in: .vertical) {
                page(player, file, space: geometry.size)
                ScrollView {
                    page(player, file, space: geometry.size)
                }
                .scrollBounceBehavior(.basedOnSize)
            }
        }
    }

    private func page(_ player: AudioPlayer, _ file: ListeningFile, space: CGSize) -> some View {
        VStack(spacing: 0) {
            header(player, file)
            VStack(spacing: 0) {
                if let offer = player.offer {
                    PlaceOfferBanner(offer: offer, accept: player.acceptOffer, dismiss: player.dismissOffer)
                        .padding(.top, 12)
                        .transition(.move(edge: .top).combined(with: .opacity))
                }
                if let error = player.error {
                    NoticeCard(text: error, icon: .triangleAlert, tint: Theme.destructive)
                        .padding(.top, 12)
                        .accessibilityIdentifier("player.error")
                }
            }
            .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { height in
                guard height != noticesHeight else { return }
                withAnimation(.spring(response: 0.4, dampingFraction: 0.85)) { noticesHeight = height }
            }
            artwork(player, file, size: artworkWidth(space))
            titles(file)
            NowPlayingPosition(player: player, file: file) { list = .chapters }
            Spacer(minLength: 12)
            transport(player)
            Spacer(minLength: 16)
            tools(player)
        }
        .padding(.horizontal, 22)
        // The home indicator's safe area is room enough under the tools.
        .padding(.bottom, 8)
        .frame(maxWidth: 560)
        .frame(maxWidth: .infinity)
        .animation(.spring(response: 0.4, dampingFraction: 0.85), value: player.offer)
    }

    private func artworkWidth(_ space: CGSize) -> CGFloat {
        NowPlayingLayout.artworkWidth(space: space, accessibilitySize: dynamicTypeSize.isAccessibilitySize, noticesHeight: noticesHeight)
    }

    // MARK: - Header

    /// Close, the Book's Series (or title) leading to its page, and the
    /// player's menu with AirPlay beside it.
    private func header(_ player: AudioPlayer, _ file: ListeningFile) -> some View {
        ZStack {
            // Goes to the Book's page, under the player.
            Button { model.showBook(file.book) } label: {
                // The number first, as the Book page and the Library show it.
                HStack(spacing: 6) {
                    if let number = file.series?.number {
                        SeriesNumberBadge(number: number, end: file.series?.numberEnd)
                    }
                    Text(file.series?.name ?? file.book.title)
                        .lineLimit(1)
                }
                .font(.geist(.footnote, weight: .semibold))
                .foregroundStyle(Theme.foreground)
                .padding(.horizontal, 10)
                .frame(minHeight: 40)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .padding(.horizontal, 96)
            .accessibilityLabel("Go to \(file.book.title)")
            .accessibilityIdentifier("player.book")
            HStack(spacing: 8) {
                Button { dismiss() } label: {
                    RoundIcon(.chevronDown, size: 22)
                }
                .accessibilityLabel("Close Player")
                .accessibilityIdentifier("player.close")
                Spacer()
                RoutePicker()
                    .frame(width: 40, height: 40)
                    .background(Theme.background.opacity(0.4), in: Circle())
                    .accessibilityIdentifier("player.airplay")
                PlayerMenu(player: player, showBookmarks: { list = .bookmarks })
            }
            .buttonStyle(PlayerControlStyle())
        }
        // A control keeps its size; VoiceOver and the large content viewer
        // read it at any text size.
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
        .padding(.top, 14)
    }

    private func artwork(_ player: AudioPlayer, _ file: ListeningFile, size: CGFloat) -> some View {
        CoverView(book: file.book, file: CoverFile(id: file.fileID, fileType: "m4b", coverVersion: file.coverVersion),
                  width: size, shadow: .large) { image in
            let average = await CoverTint.average(of: image)
            // A cover replaced meanwhile has its own tint coming.
            guard !Task.isCancelled else { return }
            tint = average?.color
        }
        .padding(.top, 12)
    }

    private func titles(_ file: ListeningFile) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(file.book.title)
                .font(.geist(size: 26, relativeTo: .title, weight: .heavy))
                .tracking(-0.8)
                .lineLimit(2)
                .foregroundStyle(Theme.foreground)
                .accessibilityAddTraits(.isHeader)
                .accessibilityIdentifier("player.title")
            if let byline = byline(file, all: showingAllPeople) {
                let text = Text(byline)
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.foreground.opacity(0.7))
                    // However long, so no name's link is cut off.
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
                Group {
                    if showingAllPeople {
                        // In a box that scrolls on its own, so the controls
                        // stay where they are.
                        ScrollView { text }
                            .frame(maxHeight: peopleBoxHeight)
                            .scrollBounceBehavior(.basedOnSize)
                    } else {
                        text
                    }
                }
                // A name goes to its Person's page, under the player; "more"
                // and "less" show every name or the first few.
                .environment(\.openURL, OpenURLAction { url in
                    if url == Self.morePeople || url == Self.fewerPeople {
                        withAnimation(.easeOut(duration: 0.2)) { showingAllPeople = url == Self.morePeople }
                        return .handled
                    }
                    guard let route = LinkedText.route(url) else { return .systemAction }
                    model.showFromPlayer(route)
                    return .handled
                })
                .onChange(of: file.fileID) { _, _ in showingAllPeople = false }
                .accessibilityIdentifier("player.byline")
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.top, 22)
    }

    /// Links in the byline that show every author and narrator, or the
    /// first few again.
    private static let morePeople = URL(string: "shisho-player:more-people")!
    private static let fewerPeople = URL(string: "shisho-player:fewer-people")!

    /// "Author · Narrated by Narrator", whichever there are, each name
    /// linked to its Person's page as on the Book's page. Unless all, at
    /// most two of each, then how many more there are, which shows them.
    private func byline(_ file: ListeningFile, all: Bool) -> AttributedString? {
        // Each Person once, though they may have several roles.
        var seen: Set<Int> = []
        let authors = file.authors.filter { seen.insert($0.personID).inserted }
            .map { (Route.resource(.person, id: $0.personID, name: $0.name), $0.name) }
        // Their Persons can be missing while a catalog downloads again, as
        // after an app update: the names show without links then.
        let linked = file.narratorIDs.count == file.narrators.count
        let narrators: [(Route?, String)] = file.narrators.enumerated().map { index, name in
            (linked ? Route.resource(.person, id: file.narratorIDs[index], name: name) : nil, name)
        }
        let cut = PlayerPeople.cut(authors: authors.count, narrators: narrators.count, all: all)
        let limit = cut.limit
        var parts: [AttributedString] = []
        if !authors.isEmpty {
            parts.append(Byline.links(Array(authors.prefix(limit))))
        }
        if !narrators.isEmpty {
            var names = AttributedString()
            for (index, narrator) in narrators.prefix(limit).enumerated() {
                if index > 0 { names += LinkedText.muted(", ") }
                names += narrator.0.map { LinkedText.link(narrator.1, to: $0) } ?? AttributedString(narrator.1)
            }
            parts.append(LinkedText.muted(String(localized: "Narrated by ")) + names)
        }
        guard var text = parts.first else { return nil }
        for part in parts.dropFirst() {
            text += LinkedText.muted(" · ") + part
        }
        if cut.hidden > 0 {
            text += LinkedText.muted(" ") + Self.toggleLink(String(localized: "+\(cut.hidden) more"), Self.morePeople)
        } else if cut.collapsible {
            text += LinkedText.muted(" ") + Self.toggleLink(String(localized: "Show less"), Self.fewerPeople)
        }
        return text
    }

    private static func toggleLink(_ title: String, _ url: URL) -> AttributedString {
        var link = AttributedString(title)
        link.link = url
        link.foregroundColor = Theme.primary
        return link
    }

    // MARK: - Transport

    private func transport(_ player: AudioPlayer) -> some View {
        HStack {
            Button { player.previousChapter() } label: {
                Icon(.skipBack, size: 24, relativeTo: nil).frame(width: 48, height: 48)
            }
            .accessibilityLabel("Previous Chapter")
            .accessibilityIdentifier("player.prevChapter")
            Spacer()
            Button { player.skipBack() } label: {
                SkipIcon(forward: false, seconds: player.backInterval, size: 34).frame(width: 48, height: 48)
            }
            .accessibilityLabel("Back \(Int(player.backInterval)) Seconds")
            .accessibilityIdentifier("player.skipBack")
            Spacer()
            Button { player.togglePlay() } label: {
                Icon(player.isPlaying ? .pauseFill : .playFill, size: 30, relativeTo: nil)
                    // The triangle's weight sits left of its box's center.
                    .offset(x: player.isPlaying ? 0 : 2)
                    .foregroundStyle(Theme.primaryForeground)
                    .frame(width: 72, height: 72)
                    .background(Theme.primary, in: Circle())
                    .shadow(color: Theme.primary.opacity(0.45), radius: 16)
            }
            .accessibilityLabel(player.isPlaying ? "Pause" : "Play")
            .accessibilityIdentifier("player.playPause")
            Spacer()
            Button { player.skipForward() } label: {
                SkipIcon(forward: true, seconds: player.forwardInterval, size: 34).frame(width: 48, height: 48)
            }
            .accessibilityLabel("Forward \(Int(player.forwardInterval)) Seconds")
            .accessibilityIdentifier("player.skipForward")
            Spacer()
            Button { player.nextChapter() } label: {
                Icon(.skipForward, size: 24, relativeTo: nil).frame(width: 48, height: 48)
            }
            .accessibilityLabel("Next Chapter")
            .accessibilityIdentifier("player.nextChapter")
        }
        .buttonStyle(PlayerControlStyle())
        .foregroundStyle(Theme.foreground)
        .padding(.top, 2)
        // The controls keep their size; at accessibility sizes a long press
        // shows each large.
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
        .accessibilityShowsLargeContentViewer()
    }

    // MARK: - Tools

    /// Speed, the sleep timer, Bookmark, and Chapters, under a rule.
    private func tools(_ player: AudioPlayer) -> some View {
        PlayerToolbar(
            player: player,
            addBookmark: {
                guard let bookmark = player.addBookmark(note: nil) else { return }
                added = bookmark
                AccessibilityNotification.Announcement(String(localized: "Bookmark added at \(TimeLabels.clock(bookmark.seconds))")).post()
            },
            showChapters: { list = .chapters }
        )
        .padding(.top, 12)
        .overlay(alignment: .top) {
            Rectangle().fill(Theme.foreground.opacity(0.12)).frame(height: 1)
        }
    }

    // MARK: - Toast

    /// Says a Bookmark was added, offering to add a note to it, for a few
    /// seconds.
    @ViewBuilder
    private func toast(_ player: AudioPlayer) -> some View {
        if let added {
            HStack(spacing: 12) {
                Text("Bookmark added at \(TimeLabels.clock(added.seconds))")
                    .monospacedDigit()
                Button {
                    note = ""
                    noting = added
                    self.added = nil
                } label: {
                    Text("Add Note")
                        .fontWeight(.semibold)
                        .underline()
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("bookmark.addNote")
            }
            .font(.geist(.footnote, weight: .medium))
            .foregroundStyle(Color.white)
            .padding(.horizontal, 14)
            .padding(.vertical, 9)
            .background(Color(white: 0.07), in: Capsule())
            .shadow(color: .black.opacity(0.25), radius: 10, y: 4)
            // Under the header, so its controls stay in reach.
            .padding(.top, 64)
            .transition(.move(edge: .top).combined(with: .opacity))
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("bookmark.toast")
            .task(id: added) {
                try? await Task.sleep(for: .seconds(4))
                guard !Task.isCancelled else { return }
                withAnimation(.easeOut(duration: 0.25)) { self.added = nil }
            }
        }
    }
}

/// An icon in the round, translucent button variant E's player header uses.
struct RoundIcon: View {
    let name: Lucide
    var size: CGFloat = 20

    init(_ name: Lucide, size: CGFloat = 20) {
        self.name = name
        self.size = size
    }

    var body: some View {
        Icon(name, size: size, relativeTo: nil)
            .foregroundStyle(Theme.foreground)
            .frame(width: 40, height: 40)
            .background(Theme.background.opacity(0.4), in: Circle())
    }
}

/// How many of a File's authors and narrators the player names before
/// "+N more": two of each, unless the user asked for everyone.
enum PlayerPeople {
    static let shown = 2

    struct Cut: Equatable {
        /// How many authors, and how many narrators, show.
        let limit: Int
        /// How many more there are, which "+N more" shows.
        let hidden: Int
        /// Everyone shows, and there are more than the first few: "Show
        /// less" cuts them again.
        let collapsible: Bool
    }

    static func cut(authors: Int, narrators: Int, all: Bool) -> Cut {
        let limit = all ? Int.max : shown
        return Cut(limit: limit, hidden: max(0, authors - limit) + max(0, narrators - limit),
                   collapsible: all && (authors > shown || narrators > shown))
    }
}

/// Now Playing's sizes, worked out apart from the view so they can be
/// tested.
enum NowPlayingLayout {
    /// The artwork's width: as large as fits beside the controls and any
    /// notice over it, by its height for a portrait cover. A notice takes
    /// its height from the artwork, so the tools stay on screen; with no
    /// notice the artwork keeps a larger floor.
    static func artworkWidth(space: CGSize, accessibilitySize: Bool, noticesHeight: CGFloat) -> CGFloat {
        let height = space.height * (accessibilitySize ? 0.28 : 0.38) - noticesHeight
        // An audiobook's cover is square.
        return max(noticesHeight > 0 ? 72 : 120, min(336, space.width - 44, height))
    }
}

/// Now Playing's chapter and the bar under it: the only part of the screen
/// that reads where playback is, so the rest doesn't redraw as it plays or
/// as the bar is dragged.
private struct NowPlayingPosition: View {
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let player: AudioPlayer
    let file: ListeningFile
    let showChapters: () -> Void
    /// Where the scrubber is while it's dragged, in seconds into the File.
    @State private var scrubbing: Double?
    /// The chapter the scrubber spans for the whole of a drag, so playing
    /// into the next chapter meanwhile doesn't move the target.
    @State private var dragBounds: (start: Double, end: Double)?
    /// The bar spans the whole book instead of the chapter; tapping the time
    /// left switches it.
    @AppStorage("player.scrubsWholeBook") private var wholeBook = false

    var body: some View {
        VStack(spacing: 0) {
            chapterRow
            scrubber
        }
    }

    /// The chapter playing, which opens the chapter list.
    private var chapterRow: some View {
        HStack(spacing: 10) {
            Button(action: showChapters) {
                HStack(spacing: 7) {
                    PlayingIndicator(playing: player.isPlaying && !player.isBuffering)
                    Text(chapterLabel)
                        .font(.geist(.footnote, weight: .medium))
                        .lineLimit(1)
                        .accessibilityIdentifier("player.chapter")
                    if !file.chapters.isEmpty {
                        Icon(.chevronDown, size: 14, relativeTo: .footnote)
                            .opacity(0.6)
                    }
                    if player.isBuffering {
                        ProgressView()
                            .controlSize(.mini)
                            .accessibilityLabel("Loading")
                    }
                }
                .foregroundStyle(Theme.primary)
                .frame(minHeight: 32)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .disabled(file.chapters.isEmpty)
            .accessibilityHint("Shows the chapters")
            .accessibilityIdentifier("player.chapterLink")
            Spacer(minLength: 0)
        }
        .padding(.top, 12)
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
    }

    /// The chapter playing, or while the bar spans the whole book and is
    /// dragged, the chapter the drag is over, so it isn't blind.
    private var chapterLabel: String {
        if wholeBook, let scrubbing, let chapters = player.file?.chapters,
           let index = AudioChapters.index(at: scrubbing, in: chapters) {
            return AudioChapters.name(of: index, in: chapters)
        }
        return player.chapterTitle ?? String(localized: "No chapters")
    }

    private var scrubber: some View {
        let bounds = dragBounds ?? (wholeBook ? (start: 0, end: file.duration) : player.chapterBounds)
        let length = max(1, bounds.end - bounds.start)
        let shown = scrubbing ?? player.position
        let elapsed = min(length, max(0, shown - bounds.start))
        let left = PlaybackSpeed.timeLeft(from: shown, to: bounds.end, at: player.rate)
        return VStack(spacing: 6) {
            Scrubber(value: elapsed, length: length) { dragged in
                if let dragged {
                    if dragBounds == nil { dragBounds = bounds }
                    scrubbing = bounds.start + dragged
                } else {
                    scrubbing = nil
                    dragBounds = nil
                }
            } onCommit: { place in
                // The end of a chapter is just before the next one starts.
                player.seek(to: bounds.start + min(place, max(0, length - 0.5)))
            }
            .accessibilityLabel(wholeBook ? "Position in book" : "Position in chapter")
            .accessibilityValue(String(localized: "\(TimeLabels.clock(elapsed)) of \(TimeLabels.clock(length))"))
            .accessibilityIdentifier("player.scrubber")
            // At accessibility text sizes the time left goes under the
            // times, so none of them wraps.
            let stacked = dynamicTypeSize.isAccessibilitySize
            let timeLeft = Button { wholeBook.toggle() } label: {
                Text(wholeBook
                    ? "\(TimeLabels.clock(left)) left in the book at \(PlaybackSpeed.label(player.rate))"
                    : "\(TimeLabels.clock(left)) left in the chapter at \(PlaybackSpeed.label(player.rate))")
                    .contentTransition(.numericText())
                    .frame(minHeight: 32)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityHint(wholeBook ? "Makes the bar span the chapter" : "Makes the bar span the whole book")
            .accessibilityIdentifier("player.left")
            VStack(spacing: 4) {
                HStack {
                    Text(TimeLabels.clock(elapsed))
                        .accessibilityLabel("Elapsed \(TimeLabels.clock(elapsed))")
                        .accessibilityIdentifier("player.position")
                    Spacer()
                    if !stacked {
                        timeLeft
                        Spacer()
                    }
                    Text("-\(TimeLabels.clock(length - elapsed))")
                        .accessibilityLabel(wholeBook
                            ? "Book remaining \(TimeLabels.clock(length - elapsed))"
                            : "Chapter remaining \(TimeLabels.clock(length - elapsed))")
                }
                if stacked {
                    timeLeft
                }
            }
            .font(.geist(.caption, weight: .medium))
            .monospacedDigit()
            .foregroundStyle(Theme.foreground.opacity(0.7))
        }
        .padding(.top, 12)
    }
}
