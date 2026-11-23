import SwiftUI

/// What's playing, docked above the bottom of every screen: tinted by the
/// cover, with the playing indicator, play and pause, and a skip. Tapping it
/// opens Now Playing.
struct MiniPlayer: View {
    @Environment(AppModel.self) private var model
    @State private var tint: Color?
    /// How far the bar has been dragged down.
    @State private var drag: CGFloat = 0

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
                Icon(player.isPlaying ? .pauseFill : .playFill, size: 22, relativeTo: nil)
                    .frame(width: 40, height: 40)
            }
            .buttonStyle(PlayerControlStyle())
            .accessibilityLabel(player.isPlaying ? "Pause" : "Play")
            .accessibilityIdentifier("miniplayer.playPause")
            Button { player.skipForward() } label: {
                SkipIcon(forward: true, seconds: player.forwardInterval, size: 26).frame(width: 40, height: 40)
            }
            .buttonStyle(PlayerControlStyle())
            .accessibilityLabel("Forward \(Int(player.forwardInterval)) Seconds")
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
        .accessibilityShowsLargeContentViewer()
        // Paused, it swipes down out of the way, as docked players do; playing,
        // it only gives a little and springs back.
        .offset(y: drag)
        .opacity(1 - Double(min(1, max(0, drag) / 120)) * 0.6)
        // Ahead of the buttons, so a swipe that starts on one never also
        // opens the player; a tap doesn't move far enough to start it.
        .highPriorityGesture(
            DragGesture(minimumDistance: 12)
                .onChanged { value in
                    let down = max(0, value.translation.height)
                    drag = player.isPlaying ? down * 0.2 : down
                }
                .onEnded { value in
                    if !player.isPlaying, value.translation.height > 50 || value.predictedEndTranslation.height > 120 {
                        withAnimation(.easeOut(duration: 0.2)) { drag = 160 }
                        Task {
                            try? await Task.sleep(for: .milliseconds(200))
                            player.dismiss()
                            drag = 0
                        }
                    } else {
                        withAnimation(.spring(response: 0.3, dampingFraction: 0.8)) { drag = 0 }
                    }
                }
        )
        .accessibilityElement(children: .contain)
        .accessibilityAction(named: Text("Dismiss")) {
            if !player.isPlaying { player.dismiss() }
        }
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

extension View {
    /// Room at the end of a scroll view for the mini player while it shows,
    /// so the last of the content can scroll clear of it.
    func miniPlayerClearance() -> some View {
        modifier(MiniPlayerClearance())
    }
}

private struct MiniPlayerClearance: ViewModifier {
    @Environment(AppModel.self) private var model
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    func body(content: Content) -> some View {
        let showing = model.player?.file != nil
        // The bar's height at the largest size it grows to, and its margin,
        // over the tab bar.
        let room: CGFloat = dynamicTypeSize >= .xxLarge ? 96 : 80
        content.contentMargins(.bottom, (showing ? room : 0) + AppTabBar.height, for: .scrollContent)
    }
}
