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
        return isThisFile ? model.player?.rate ?? 1 : AudioPlayer.storedRate(accountID: account.id, bookID: bookID)
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

    private var detail: String {
        if finished && !playing {
            return progress.audiobookCount > 1 && !progress.fileName.isEmpty
                ? String(localized: "\(progress.fileName) · Finished") : String(localized: "Finished")
        }
        var parts: [String] = []
        // With more than one audiobook, it says which this plays.
        if progress.audiobookCount > 1, !progress.fileName.isEmpty {
            parts.append(progress.fileName)
        }
        if let index = AudioChapters.index(at: seconds, in: progress.chapters) {
            parts.append(String(localized: "Chapter \(index + 1)"))
        }
        parts.append(String(localized: "\(TimeLabels.left(max(0, progress.duration - seconds) / rate)) left at \(PlaybackSpeed.label(rate))"))
        return parts.joined(separator: " · ")
    }

    private var icon: some View {
        Group {
            if playing {
                PlayingIndicator(playing: true, height: 14)
            } else {
                Image(systemName: finished ? "arrow.counterclockwise" : "headphones")
                    .font(.system(size: 20, weight: .semibold))
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
            .padding(.vertical, stacked ? 12 : 0)
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
        .accessibilityValue(seconds > 1 ? "\(fraction.formatted(.percent.precision(.fractionLength(0)))), \(detail)" : detail)
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("book.listen")
    }
}
