import SwiftUI

/// Variant E's listen bar on a Book's page: the primary action for a Book
/// with an audiobook, with where the user is, the time left at their speed
/// for this Book, and a progress line. It opens Now Playing.
struct ListenCard: View {
    @Environment(AppModel.self) private var model
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

    private var title: LocalizedStringKey {
        if playing { return "Listening" }
        return seconds > 1 ? "Resume listening" : "Listen"
    }

    private var detail: String {
        let left = String(localized: "\(TimeLabels.left(max(0, progress.duration - seconds) / rate)) left at \(PlaybackSpeed.label(rate))")
        guard let index = AudioChapters.index(at: seconds, in: progress.chapters) else { return left }
        return String(localized: "Chapter \(index + 1) · \(left)")
    }

    var body: some View {
        Button { model.listen(fileID: progress.fileID) } label: {
            HStack(spacing: 12) {
                Group {
                    if playing {
                        PlayingIndicator(playing: true, height: 14)
                    } else {
                        Image(systemName: "headphones")
                            .font(.system(size: 20, weight: .semibold))
                    }
                }
                .frame(width: 24)
                VStack(alignment: .leading, spacing: 2) {
                    Text(title)
                        .font(.geist(.callout, weight: .bold))
                    Text(detail)
                        .font(.geist(.caption))
                        .opacity(0.8)
                        .lineLimit(1)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                if seconds > 1 {
                    Text(fraction, format: .percent.precision(.fractionLength(0)))
                        .font(.geist(.title3, weight: .heavy))
                        .tracking(-0.4)
                        .monospacedDigit()
                }
            }
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
