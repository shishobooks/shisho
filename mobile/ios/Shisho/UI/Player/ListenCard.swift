import SwiftUI

/// Variant E's listen bar on a Book's page: the primary action for a Book
/// with an audiobook, with where the user is, the time left at their speed
/// for this Book, and a progress line. It opens Now Playing.
struct ListenCard: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let progress: ListenProgress
    let bookID: Int

    private var isThisFile: Bool { model.player?.file?.fileID == progress.fileID }
    private var playing: Bool { isThisFile && model.player?.isPlaying == true }
    private var seconds: Double { isThisFile ? model.player?.position ?? progress.seconds : progress.seconds }
    private var rate: Double {
        guard let account = model.session?.account else { return 1 }
        return isThisFile ? model.player?.rate ?? 1 : AudioPlayer.storedRate(accountID: account.id, fileID: progress.fileID)
    }

    private var fraction: Double {
        progress.duration > 0 ? min(1, max(0, seconds / progress.duration)) : 0
    }

    /// At the end, where playing again starts over.
    private var finished: Bool {
        progress.duration > 0 && seconds >= progress.duration - 1
    }

    private var title: LocalizedStringKey {
        if playing { return "Listening" }
        if finished { return "Listen again" }
        return seconds > 1 ? "Resume listening" : "Listen"
    }

    /// Which audiobook this plays, when the Book has more than one: its
    /// name, and its narrators, since editions often share a name. Each has
    /// a line of its own so neither is cut off.
    private var whichFile: [String] {
        guard progress.audiobookCount > 1 else { return [] }
        var lines: [String] = []
        if !progress.fileName.isEmpty { lines.append(progress.fileName) }
        if !progress.narrators.isEmpty {
            lines.append(String(localized: "Narrated by \(progress.narrators.joined(separator: ", "))"))
        }
        return lines
    }

    /// The chapter by its name, as Now Playing names it, or by number when
    /// it has none. It has a line of its own, so a long name can't push the
    /// time left out of view.
    private var chapter: String? {
        guard !(finished && !playing), let index = AudioChapters.index(at: seconds, in: progress.chapters) else { return nil }
        return AudioChapters.name(of: index, in: progress.chapters)
    }

    private var detail: String {
        if finished && !playing {
            return String(localized: "Finished")
        }
        return String(localized: "\(TimeLabels.clock(max(0, progress.duration - seconds) / rate)) left at \(PlaybackSpeed.label(rate))")
    }

    private var icon: some View {
        Group {
            if playing {
                PlayingIndicator(playing: true, height: 14)
            } else {
                Icon(finished ? .rotateCcw : .headphones, size: 22, relativeTo: nil)
            }
        }
        .frame(width: 24)
    }

    private var titleText: some View {
        Text(title)
            .font(.geist(.callout, weight: .bold))
            .fixedSize(horizontal: false, vertical: true)
    }

    var body: some View {
        Button { model.listen(fileID: progress.fileID) } label: {
            // At accessibility text sizes the parts stack, so no word breaks.
            let stacked = dynamicTypeSize.isAccessibilitySize
            let layout = stacked
                ? AnyLayout(VStackLayout(alignment: .leading, spacing: 6))
                : AnyLayout(HStackLayout(spacing: 12))
            layout {
                HStack(spacing: 12) {
                    icon
                    if stacked { titleText }
                }
                VStack(alignment: .leading, spacing: 2) {
                    if !stacked { titleText }
                    ForEach(whichFile, id: \.self) { line in
                        Text(line)
                            .font(.geist(.caption, weight: .medium))
                            .lineLimit(2)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    if let chapter {
                        Text(chapter)
                            .font(.geist(.caption, weight: .medium))
                            .lineLimit(stacked ? nil : 1)
                            .fixedSize(horizontal: false, vertical: stacked)
                    }
                    Text(detail)
                        .font(.geist(.caption))
                        .opacity(0.8)
                        .lineLimit(stacked ? nil : 1)
                        .fixedSize(horizontal: false, vertical: stacked)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                if seconds > 1 && !finished {
                    Text(fraction, format: .percent.precision(.fractionLength(0)))
                        .font(.geist(.title3, weight: .heavy))
                        .tracking(-0.4)
                        .monospacedDigit()
                }
            }
            // Room above and below however many lines it has, and above the
            // progress line when there is one.
            .padding(.top, 12)
            .padding(.bottom, seconds > 1 ? 15 : 12)
            .padding(.horizontal, 16)
            .frame(maxWidth: .infinity, minHeight: 60)
            .foregroundStyle(Theme.primaryForeground)
            .background(Theme.primary)
            .overlay(alignment: .bottomLeading) {
                GeometryReader { geometry in
                    Rectangle()
                        .fill(Theme.primaryForeground.opacity(0.55))
                        .frame(width: geometry.size.width * fraction, height: 3)
                        .frame(maxHeight: .infinity, alignment: .bottom)
                }
                .accessibilityHidden(true)
            }
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            .shadow(color: Theme.primary.opacity(0.35), radius: 14, y: 10)
        }
        .buttonStyle(PressableStyle())
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(title))
        .accessibilityValue((whichFile + [chapter].compactMap { $0 }
            + [seconds > 1 ? "\(fraction.formatted(.percent.precision(.fractionLength(0)))), \(detail)" : detail])
            .joined(separator: ", "))
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("book.listen")
    }
}
