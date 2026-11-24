import AVKit
import SwiftUI

/// The tools under the transport, evenly spaced: the speed stepper, then
/// Sleep, Bookmark, and Chapters, each an icon over its name.
struct PlayerToolbar: View {
    let player: AudioPlayer
    let addBookmark: () -> Void
    let showChapters: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 0) {
            speed
            Spacer(minLength: 4)
            sleepMenu
            Spacer(minLength: 4)
            if player.bookmarksAvailable {
                Button(action: addBookmark) {
                    ToolLabel(icon: .bookmarkPlus, caption: Text("Bookmark"))
                }
                .accessibilityLabel("Add Bookmark")
                .accessibilityHint("Bookmarks where playback is")
                .accessibilityIdentifier("player.bookmark")
                Spacer(minLength: 4)
            }
            Button(action: showChapters) {
                ToolLabel(icon: .listMusic, caption: Text("Chapters"))
            }
            .disabled(player.file?.chapters.isEmpty ?? true)
            .accessibilityLabel("Chapters")
            .accessibilityIdentifier("player.chapters")
        }
        .buttonStyle(.plain)
        // The row keeps its size, as the transport does; at accessibility
        // sizes a long press shows each tool large.
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
        .accessibilityShowsLargeContentViewer()
    }

    /// Speed in the web player's 0.1 steps, remembered per File, in the web
    /// app's button group.
    private var speed: some View {
        VStack(spacing: 5) {
            HStack(spacing: 0) {
                Button { player.setRate(player.rate - PlaybackSpeed.step) } label: {
                    Icon(.minus, size: 15, relativeTo: nil).frame(width: 32, height: 30).hitArea(height: 30)
                }
                .disabled(player.rate <= PlaybackSpeed.range.lowerBound)
                .accessibilityLabel("Slower")
                .accessibilityIdentifier("player.speedDown")
                Text(PlaybackSpeed.label(player.rate))
                    .font(.geist(size: 14, relativeTo: .subheadline, weight: .heavy))
                    .tracking(-0.3)
                    .monospacedDigit()
                    .foregroundStyle(Theme.foreground)
                    .frame(minWidth: 46, minHeight: 30)
                    .overlay(alignment: .leading) { Rectangle().fill(Theme.foreground.opacity(0.14)).frame(width: 1) }
                    .overlay(alignment: .trailing) { Rectangle().fill(Theme.foreground.opacity(0.14)).frame(width: 1) }
                    .contentTransition(.numericText())
                    .accessibilityLabel("Speed \(PlaybackSpeed.label(player.rate))")
                    .accessibilityIdentifier("player.speed")
                Button { player.setRate(player.rate + PlaybackSpeed.step) } label: {
                    Icon(.plus, size: 15, relativeTo: nil).frame(width: 32, height: 30).hitArea(height: 30)
                }
                .disabled(player.rate >= PlaybackSpeed.range.upperBound)
                .accessibilityLabel("Faster")
                .accessibilityIdentifier("player.speedUp")
            }
            .foregroundStyle(Theme.foreground)
            .background(Theme.background.opacity(0.5), in: RoundedRectangle(cornerRadius: 8, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).strokeBorder(Theme.foreground.opacity(0.14)))
            .shadow(color: .black.opacity(0.05), radius: 1, y: 1)
            .animation(.snappy, value: player.rate)
            Text("Speed")
                .font(.geist(size: 11, relativeTo: .caption2, weight: .medium))
                .foregroundStyle(Theme.foreground.opacity(0.66))
                .accessibilityHidden(true)
        }
        .fixedSize()
    }

    private var sleepMenu: some View {
        Menu {
            if player.sleepTimer != .off {
                Button(role: .destructive) { player.setSleepTimer(.off) } label: {
                    Label("Turn Off Timer", menuIcon: .x)
                }
                .accessibilityIdentifier("sleep.off")
            }
            ForEach(SleepTimerRules.lengths, id: \.self) { length in
                Button("\(Int(length / 60)) Minutes") {
                    player.setSleepTimer(.counting(length: length, left: length))
                }
                .accessibilityIdentifier("sleep.minutes.\(Int(length / 60))")
            }
            Button("End of Chapter") { player.setSleepTimer(.endOfChapter) }
                .accessibilityIdentifier("sleep.endOfChapter")
        } label: {
            ToolLabel(icon: player.sleepTimer == .off ? .moon : .moonFill, caption: sleepCaption, active: player.sleepTimer != .off)
        }
        .accessibilityLabel(sleepLabel)
        .accessibilityIdentifier("player.sleep")
    }

    private var sleepCaption: Text {
        switch player.sleepTimer {
        case .off: Text("Sleep")
        case let .counting(_, left): Text(verbatim: TimeLabels.clock(left))
        case .endOfChapter: Text("Chapter End")
        }
    }

    /// What VoiceOver says, with the time left in the label itself: a
    /// menu's own value isn't always read.
    private var sleepLabel: Text {
        switch player.sleepTimer {
        case .off: Text("Sleep Timer, Off")
        case let .counting(_, left): Text("Sleep Timer, \(TimeLabels.clock(left)) left")
        case .endOfChapter: Text("Sleep Timer, stops at the end of the chapter")
        }
    }
}

/// A tool's icon over its name, in variant E's muted ink, or the brand color
/// while it's on.
private struct ToolLabel: View {
    let icon: Lucide
    let caption: Text
    var active = false

    var body: some View {
        VStack(spacing: 5) {
            Icon(icon, size: 22, relativeTo: nil)
                .frame(width: 36, height: 30)
            caption
                .font(.geist(size: 11, relativeTo: .caption2, weight: .medium))
                .monospacedDigit()
                .lineLimit(1)
                .minimumScaleFactor(0.8)
        }
        .foregroundStyle(active ? Theme.primary : Theme.foreground.opacity(0.66))
        .frame(minWidth: 60, minHeight: 44)
        .contentShape(Rectangle())
    }
}

/// The header's menu: the Bookmarks list, and the player's settings.
struct PlayerMenu: View {
    let player: AudioPlayer
    let showBookmarks: () -> Void

    var body: some View {
        Menu {
            if player.bookmarksAvailable {
                Button(action: showBookmarks) {
                    Label("Bookmarks", menuIcon: .bookmark)
                }
                .accessibilityIdentifier("player.bookmarks")
                Divider()
            }
            Picker(selection: Binding(get: { player.backInterval }, set: { player.setBackInterval($0) })) {
                ForEach(PlayerSettings.skipLengths, id: \.self) { seconds in
                    Text("\(Int(seconds)) Seconds").tag(seconds)
                }
            } label: {
                Label("Skip Back", menuIcon: .rotateCcw)
            }
            .pickerStyle(.menu)
            .accessibilityIdentifier("settings.skipBack")
            Picker(selection: Binding(get: { player.forwardInterval }, set: { player.setForwardInterval($0) })) {
                ForEach(PlayerSettings.skipLengths, id: \.self) { seconds in
                    Text("\(Int(seconds)) Seconds").tag(seconds)
                }
            } label: {
                Label("Skip Forward", menuIcon: .rotateCw)
            }
            .pickerStyle(.menu)
            .accessibilityIdentifier("settings.skipForward")
            Toggle(isOn: Binding(get: { player.smartRewind }, set: { player.setSmartRewind($0) })) {
                Label("Rewind After a Pause", menuIcon: .clockArrowUp)
            }
            .accessibilityIdentifier("settings.smartRewind")
        } label: {
            RoundIcon(.ellipsis)
        }
        .accessibilityLabel("More")
        .accessibilityIdentifier("player.more")
    }
}

/// Two choices in the web app's segmented style: a muted track with the
/// chosen one raised.
struct SegmentedChoice<Value: Hashable>: View {
    /// Each choice's value, label, and accessibility identifier.
    let options: [(Value, String, String)]
    @Binding var selection: Value
    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        HStack(spacing: 0) {
            ForEach(options, id: \.0) { value, label, id in
                let chosen = value == selection
                Button { selection = value } label: {
                    Text(label)
                        .font(.geist(.caption, weight: .medium))
                        .foregroundStyle(chosen ? Theme.foreground : Theme.foreground.opacity(0.65))
                        .lineLimit(1)
                        .padding(.horizontal, 10)
                        .frame(minHeight: 26)
                        .background {
                            if chosen {
                                RoundedRectangle(cornerRadius: 6, style: .continuous)
                                    .fill(colorScheme == .dark ? Theme.foreground.opacity(0.16) : Theme.background.opacity(0.85))
                                    .shadow(color: .black.opacity(colorScheme == .dark ? 0 : 0.08), radius: 1.5, y: 1)
                            }
                        }
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityAddTraits(chosen ? .isSelected : [])
                .accessibilityIdentifier(id)
            }
        }
        .padding(3)
        .background(Theme.foreground.opacity(0.08), in: RoundedRectangle(cornerRadius: 8, style: .continuous))
        .animation(.snappy(duration: 0.2), value: selection)
    }
}

/// The system's AirPlay picker, which lists speakers and headphones.
struct RoutePicker: UIViewRepresentable {
    func makeUIView(context: Context) -> AVRoutePickerView {
        let picker = AVRoutePickerView()
        picker.prioritizesVideoDevices = false
        picker.tintColor = UIColor(Theme.foreground)
        picker.activeTintColor = UIColor(Theme.primary)
        picker.accessibilityLabel = String(localized: "AirPlay")
        return picker
    }

    func updateUIView(_ picker: AVRoutePickerView, context: Context) {}
}

/// The File's chapters and the user's Bookmarks in it, as a sheet with a
/// tab for each, in the web app's look: bordered lists on the page's
/// background, the chapter playing in the brand color.
struct PlayerListSheet: View {
    enum Tab: String, Identifiable {
        case chapters, bookmarks
        var id: String { rawValue }
    }

    let player: AudioPlayer
    @State var tab: Tab
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var bookmarks: [Bookmark]?
    @State private var editing: Bookmark?
    @State private var note = ""

    init(player: AudioPlayer, tab: Tab) {
        self.player = player
        _tab = State(initialValue: tab)
    }

    var body: some View {
        VStack(spacing: 0) {
            header
            ScrollViewReader { scroller in
                ScrollView {
                    Group {
                        switch tab {
                        case .chapters: chapters
                        case .bookmarks: bookmarkList
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.bottom, 24)
                }
                .onAppear {
                    if tab == .chapters, let index = player.chapterIndex {
                        scroller.scrollTo(index, anchor: .center)
                    }
                }
            }
        }
        .background(Theme.background)
        .tint(Theme.primary)
        .observing(model.session?.store, id: player.file?.fileID, into: $bookmarks) { [fileID = player.file?.fileID ?? 0] db in
            try CatalogStore.bookmarks(db, fileID: fileID)
        }
        .alert("Edit Note", isPresented: Binding(get: { editing != nil }, set: { if !$0 { editing = nil } })) {
            TextField("Note", text: $note)
                .accessibilityIdentifier("bookmarks.note")
            Button("Save") {
                if let editing { player.editBookmark(editing, note: note) }
                editing = nil
            }
            .accessibilityIdentifier("bookmarks.save")
            Button("Cancel", role: .cancel) { editing = nil }
                .accessibilityIdentifier("bookmarks.cancel")
        } message: {
            Text("Leave it empty to remove the note.")
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
    }

    /// The title, or a switch between the tabs when the server keeps
    /// Bookmarks, with Done.
    private var header: some View {
        ZStack {
            if player.bookmarksAvailable {
                SegmentedChoice(
                    options: [(Tab.chapters, String(localized: "Chapters"), "list.chapters"),
                              (Tab.bookmarks, String(localized: "Bookmarks"), "list.bookmarks")],
                    selection: $tab
                )
            } else {
                Text("Chapters")
                    .font(.geist(.subheadline, weight: .semibold))
                    .foregroundStyle(Theme.foreground)
                    .accessibilityAddTraits(.isHeader)
            }
            HStack {
                Spacer()
                Button("Done") { dismiss() }
                    .font(.geist(.subheadline, weight: .semibold))
                    .foregroundStyle(Theme.primary)
                    .frame(minHeight: 44)
                    .accessibilityIdentifier("list.done")
            }
        }
        .padding(.horizontal, 16)
        .padding(.top, 18)
        .padding(.bottom, 10)
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
    }

    // MARK: - Chapters

    private var chapters: some View {
        VStack(spacing: 0) {
            if let file = player.file {
                ForEach(Array(file.chapters.enumerated()), id: \.offset) { index, chapter in
                    if index > 0 {
                        Rectangle().fill(Theme.border).frame(height: 1)
                    }
                    chapterRow(index: index, chapter: chapter, file: file)
                        .id(index)
                }
            }
        }
        .clipShape(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(Theme.border))
    }

    private func chapterRow(index: Int, chapter: AudioChapter, file: ListeningFile) -> some View {
        let playing = player.chapterIndex == index
        // Unknown for the last chapter of a File without a length.
        let end = AudioChapters.end(of: index, in: file.chapters, duration: file.duration)
        let length: String? = end > chapter.start ? FileFacts.duration(end - chapter.start) : nil
        let name = AudioChapters.name(of: index, in: file.chapters)
        return Button {
            player.seek(to: chapter.start)
            dismiss()
        } label: {
            HStack(spacing: 12) {
                Group {
                    if playing {
                        PlayingIndicator(playing: player.isPlaying && !player.isBuffering)
                    } else {
                        Text(verbatim: "\(index + 1)")
                            .font(.geist(.caption))
                            .monospacedDigit()
                    }
                }
                .frame(width: 20, alignment: .leading)
                .foregroundStyle(playing ? Theme.primary : Theme.mutedForeground)
                Text(name)
                    .font(.geist(.subheadline, weight: .medium))
                    .foregroundStyle(playing ? Theme.primary : Theme.foreground)
                    .frame(maxWidth: .infinity, alignment: .leading)
                if let length {
                    Text(length)
                        .font(.geist(.caption))
                        .monospacedDigit()
                        .foregroundStyle(playing ? Theme.primary : Theme.mutedForeground)
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 11)
            .background(playing ? Theme.primary.opacity(0.1) : Color.clear)
            .contentShape(Rectangle())
        }
        .buttonStyle(RowPressStyle())
        .accessibilityLabel(length.map { "\(name), \($0)" } ?? name)
        .accessibilityAddTraits(playing ? .isSelected : [])
        .accessibilityIdentifier("chapters.row")
    }

    // MARK: - Bookmarks

    /// As the web player lists them: each in its own bordered row, its time
    /// and note, with edit and delete beside it.
    private var bookmarkList: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let bookmarks, bookmarks.isEmpty {
                Text("No bookmarks yet. Use Bookmark under the player to mark where you are.")
                    .font(.geist(.subheadline))
                    .foregroundStyle(Theme.mutedForeground)
                    .padding(.vertical, 8)
                    .accessibilityIdentifier("bookmarks.empty")
            }
            ForEach(bookmarks ?? []) { bookmark in
                bookmarkRow(bookmark)
            }
        }
    }

    private func bookmarkRow(_ bookmark: Bookmark) -> some View {
        let time = TimeLabels.clock(bookmark.seconds)
        return HStack(alignment: .top, spacing: 4) {
            Button {
                player.seek(to: bookmark.seconds)
                dismiss()
            } label: {
                VStack(alignment: .leading, spacing: 3) {
                    Text(time)
                        .font(.geist(.subheadline, weight: .medium))
                        .monospacedDigit()
                        .foregroundStyle(Theme.foreground)
                    if let note = bookmark.note {
                        Text(note)
                            .font(.geist(.subheadline))
                            .foregroundStyle(Theme.mutedForeground)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    Text(detail(bookmark))
                        .font(.geist(.caption))
                        .foregroundStyle(Theme.mutedForeground)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 4)
                .padding(.vertical, 2)
                .contentShape(Rectangle())
            }
            .buttonStyle(RowPressStyle())
            .accessibilityLabel(bookmark.note.map { "\(time), \($0)" } ?? time)
            .accessibilityHint("Goes to this moment")
            .accessibilityIdentifier("bookmarks.row")
            Button { edit(bookmark) } label: {
                Icon(.pencil, size: 17, relativeTo: nil).frame(width: 44, height: 44).contentShape(Rectangle())
            }
            .accessibilityLabel("Edit note for bookmark at \(time)")
            .accessibilityIdentifier("bookmarks.editNote")
            Button { player.deleteBookmark(bookmark) } label: {
                Icon(.trash, size: 17, relativeTo: nil).frame(width: 44, height: 44).contentShape(Rectangle())
            }
            .accessibilityLabel("Delete bookmark at \(time)")
            .accessibilityIdentifier("bookmarks.delete")
        }
        .buttonStyle(.plain)
        .foregroundStyle(Theme.mutedForeground)
        .padding(8)
        .overlay(RoundedRectangle(cornerRadius: 6, style: .continuous).strokeBorder(Theme.border))
        .contextMenu {
            Button { edit(bookmark) } label: { Label("Edit Note", menuIcon: .pencil) }
            Button(role: .destructive) { player.deleteBookmark(bookmark) } label: { Label("Delete", menuIcon: .trash) }
        }
    }

    private func edit(_ bookmark: Bookmark) {
        note = bookmark.note ?? ""
        editing = bookmark
    }

    /// The chapter it's in and when it was made.
    private func detail(_ bookmark: Bookmark) -> String {
        let made = bookmark.createdAt.formatted(date: .abbreviated, time: .shortened)
        guard let chapters = player.file?.chapters, let index = AudioChapters.index(at: bookmark.seconds, in: chapters) else {
            return made
        }
        return "\(chapters[index].title) · \(made)"
    }
}

/// A list row that shades while pressed, as the web's rows do on hover.
struct RowPressStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .background(configuration.isPressed ? Theme.muted : Color.clear)
    }
}
