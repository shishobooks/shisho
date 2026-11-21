import SwiftUI

/// What's playing, docked above the bottom of every screen: tinted by the
/// cover, with the playing indicator, play and pause, and a skip. Tapping it
/// opens Now Playing.
struct MiniPlayer: View {
    @Environment(AppModel.self) private var model
    @State private var tint: Color?

    var body: some View {
        if let player = model.player, let file = player.file {
            bar(player, file)
                .transition(.move(edge: .bottom).combined(with: .opacity))
        }
    }

    private func bar(_ player: AudioPlayer, _ file: ListeningFile) -> some View {
        HStack(spacing: 10) {
            Button { model.showingPlayer = true } label: { summary(player, file) }
                .buttonStyle(.plain)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("\(file.book.title), \(player.chapterTitle ?? file.book.authors)")
                .accessibilityHint("Opens the player")
                .accessibilityAddTraits(.isButton)
                .accessibilityIdentifier("miniplayer.open")
            Button { player.togglePlay() } label: {
                Image(systemName: player.isPlaying ? "pause.fill" : "play.fill")
                    .font(.system(size: 20, weight: .semibold))
                    .contentTransition(.symbolEffect(.replace))
                    .frame(width: 40, height: 40)
            }
            .buttonStyle(PlayerControlStyle())
            .accessibilityLabel(player.isPlaying ? "Pause" : "Play")
            .accessibilityIdentifier("miniplayer.playPause")
            Button { player.skip(by: AudioPlayer.skipInterval) } label: {
                SkipIcon(forward: true, size: 22).frame(width: 40, height: 40)
            }
            .buttonStyle(PlayerControlStyle())
            .accessibilityLabel("Forward 30 Seconds")
            .accessibilityIdentifier("miniplayer.skipForward")
        }
        .foregroundStyle(Theme.foreground)
        .padding(.leading, 9)
        .padding(.trailing, 6)
        .frame(minHeight: 60)
        .background {
            ZStack {
                Theme.card
                (tint ?? Theme.primary).opacity(tint == nil ? 0.08 : 0.22)
            }
        }
        .overlay(alignment: .bottom) {
            GeometryReader { geometry in
                Rectangle()
                    .fill(Theme.primary)
                    .frame(width: geometry.size.width * fraction(player, file))
            }
            .frame(height: 2)
            .background(Theme.foreground.opacity(0.1))
            .accessibilityHidden(true)
        }
        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).strokeBorder((tint ?? Theme.border).opacity(0.35)))
        .shadow(color: .black.opacity(0.14), radius: 12, y: 6)
        .padding(.horizontal, 8)
        .padding(.bottom, 6)
        // It docks over every screen, so it grows only so far with the text
        // size; Now Playing shows everything in it at full size.
        .dynamicTypeSize(...DynamicTypeSize.xxLarge)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("miniplayer")
    }

    private func summary(_ player: AudioPlayer, _ file: ListeningFile) -> some View {
        HStack(spacing: 10) {
            CoverView(book: file.book, file: CoverFile(id: file.fileID, fileType: "m4b", coverVersion: file.coverVersion),
                      width: 42) { image in
                let average = await CoverTint.average(of: image)
                // A cover replaced meanwhile has its own tint coming.
                guard !Task.isCancelled else { return }
                tint = average?.color
            }
            .frame(width: 42, height: 42)
            .clipped()
            VStack(alignment: .leading, spacing: 2) {
                Text(file.book.title)
                    .font(.geist(.subheadline, weight: .semibold))
                    .foregroundStyle(Theme.foreground)
                    .lineLimit(1)
                HStack(spacing: 6) {
                    PlayingIndicator(playing: player.isPlaying && !player.isBuffering)
                        .foregroundStyle(Theme.primary)
                    Text(player.chapterTitle ?? file.book.authors)
                        .font(.geist(.footnote))
                        .foregroundStyle(Theme.foreground.opacity(0.66))
                        .lineLimit(1)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .contentShape(Rectangle())
    }

    private func fraction(_ player: AudioPlayer, _ file: ListeningFile) -> CGFloat {
        file.duration > 0 ? CGFloat(min(1, max(0, player.position / file.duration))) : 0
    }
}
