import SwiftUI

/// Now Playing, presented as a sheet over the screen it came from. Variant
/// E's most expressive screen: the cover's color washes it, the artwork is
/// large and settles back when paused, and the controls are big. The full
/// player (chapters list, sleep timer, Bookmarks) builds on it in #916.
struct NowPlayingView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var tint: Color?
    /// Where the scrubber is while it's dragged, in seconds into the File.
    @State private var scrubbing: Double?
    /// The chapter the scrubber spans for the whole of a drag, so playing
    /// into the next chapter meanwhile doesn't move the target.
    @State private var dragBounds: (start: Double, end: Double)?

    var body: some View {
        Group {
            if let player = model.player, let file = player.file {
                content(player, file)
            } else {
                ContentUnavailableView("Nothing is playing", systemImage: "headphones")
            }
        }
        .background(alignment: .top) { backdrop }
        .background(Theme.background)
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
            ScrollView {
                VStack(spacing: 0) {
                    header(file)
                    if let offer = player.offer {
                        PlaceOfferBanner(offer: offer, accept: player.acceptOffer, dismiss: player.dismissOffer)
                            .padding(.top, 12)
                            .transition(.move(edge: .top).combined(with: .opacity))
                    }
                    if let error = player.error {
                        NoticeCard(text: error, systemImage: "exclamationmark.triangle.fill", tint: Theme.destructive)
                            .padding(.top, 12)
                            .accessibilityIdentifier("player.error")
                    }
                    artwork(player, file, size: artworkWidth(geometry.size))
                    titles(file)
                    chapterRow(player)
                    scrubber(player, file)
                    transport(player)
                    tools(player)
                }
                .padding(.horizontal, 22)
                .padding(.bottom, 24)
                .frame(maxWidth: 560)
                .frame(maxWidth: .infinity)
                .frame(minHeight: geometry.size.height, alignment: .top)
                .animation(.spring(response: 0.4, dampingFraction: 0.85), value: player.offer)
            }
            .scrollBounceBehavior(.basedOnSize)
        }
    }

    /// The artwork's width: as large as fits beside the controls, by its
    /// height for a portrait cover.
    private func artworkWidth(_ space: CGSize) -> CGFloat {
        let height = space.height * (dynamicTypeSize.isAccessibilitySize ? 0.28 : 0.38)
        // An audiobook's cover is square.
        return max(120, min(336, space.width - 44, height))
    }

    private func header(_ file: ListeningFile) -> some View {
        HStack {
            Button { dismiss() } label: {
                Image(systemName: "chevron.down")
                    .font(.geist(.body, weight: .semibold))
                    .foregroundStyle(Theme.foreground)
                    .frame(width: 40, height: 40)
                    .background(Theme.background.opacity(0.4), in: Circle())
            }
            .accessibilityLabel("Close Player")
            .accessibilityIdentifier("player.close")
            Spacer()
            Text(file.book.title)
                .font(.geist(.footnote, weight: .semibold))
                .foregroundStyle(Theme.foreground.opacity(0.75))
                .lineLimit(1)
                .accessibilityHidden(true)
            Spacer()
            Color.clear.frame(width: 40, height: 40)
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
        .padding(.top, 18)
    }

    private func titles(_ file: ListeningFile) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(file.book.title)
                .font(.geist(size: 26, relativeTo: .title, weight: .heavy))
                .tracking(-0.6)
                .lineLimit(2)
                .foregroundStyle(Theme.foreground)
                .accessibilityAddTraits(.isHeader)
                .accessibilityIdentifier("player.title")
            if let byline = byline(file) {
                Text(byline)
                    .font(.geist(.callout))
                    .foregroundStyle(Theme.foreground.opacity(0.7))
                    .lineLimit(1)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.top, 24)
    }

    /// "Author · Narrated by Narrator", whichever there are.
    private func byline(_ file: ListeningFile) -> String? {
        var parts: [String] = []
        if !file.book.authors.isEmpty { parts.append(file.book.authors) }
        if !file.narrators.isEmpty {
            parts.append(String(localized: "Narrated by \(file.narrators.joined(separator: ", "))"))
        }
        return parts.isEmpty ? nil : parts.joined(separator: " · ")
    }

    private func chapterRow(_ player: AudioPlayer) -> some View {
        HStack(spacing: 8) {
            PlayingIndicator(playing: player.isPlaying && !player.isBuffering)
                .foregroundStyle(Theme.primary)
            Text(player.chapterTitle ?? String(localized: "No chapters"))
                .font(.geist(.subheadline, weight: .medium))
                .foregroundStyle(Theme.primary)
                .lineLimit(1)
                .accessibilityIdentifier("player.chapter")
            if player.isBuffering {
                ProgressView()
                    .controlSize(.mini)
                    .accessibilityLabel("Loading")
            }
            Spacer(minLength: 0)
        }
        .padding(.top, 14)
    }

    private func scrubber(_ player: AudioPlayer, _ file: ListeningFile) -> some View {
        let bounds = dragBounds ?? player.chapterBounds
        let length = max(1, bounds.end - bounds.start)
        let shown = scrubbing ?? player.position
        let elapsed = min(length, max(0, shown - bounds.start))
        let left = max(0, file.duration - shown) / player.rate
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
            .accessibilityLabel("Position in chapter")
            .accessibilityValue(String(localized: "\(TimeLabels.clock(elapsed)) of \(TimeLabels.clock(length))"))
            .accessibilityIdentifier("player.scrubber")
            // At accessibility text sizes the time left goes under the
            // times, so none of them wraps.
            let stacked = dynamicTypeSize.isAccessibilitySize
            let timeLeft = Text("\(TimeLabels.left(left)) left at \(PlaybackSpeed.label(player.rate))")
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
                        .accessibilityLabel("Chapter remaining \(TimeLabels.clock(length - elapsed))")
                }
                if stacked {
                    timeLeft
                }
            }
            .font(.geist(.caption, weight: .medium))
            .monospacedDigit()
            .foregroundStyle(Theme.foreground.opacity(0.7))
        }
        .padding(.top, 14)
    }

    private func transport(_ player: AudioPlayer) -> some View {
        HStack {
            control("backward.end.fill", label: "Previous Chapter", id: "player.prevChapter", size: 22) { player.previousChapter() }
            Spacer()
            Button { player.skip(by: -AudioPlayer.skipInterval) } label: {
                SkipIcon(forward: false).frame(width: 52, height: 52)
            }
            .accessibilityLabel("Back 30 Seconds")
            .accessibilityIdentifier("player.skipBack")
            Spacer()
            Button { player.togglePlay() } label: {
                Image(systemName: player.isPlaying ? "pause.fill" : "play.fill")
                    .font(.system(size: 30, weight: .bold))
                    .contentTransition(.symbolEffect(.replace))
                    .foregroundStyle(Theme.primaryForeground)
                    .frame(width: 76, height: 76)
                    .background(Theme.primary, in: Circle())
                    .shadow(color: Theme.primary.opacity(0.45), radius: 14, y: 10)
            }
            .accessibilityLabel(player.isPlaying ? "Pause" : "Play")
            .accessibilityIdentifier("player.playPause")
            Spacer()
            Button { player.skip(by: AudioPlayer.skipInterval) } label: {
                SkipIcon(forward: true).frame(width: 52, height: 52)
            }
            .accessibilityLabel("Forward 30 Seconds")
            .accessibilityIdentifier("player.skipForward")
            Spacer()
            control("forward.end.fill", label: "Next Chapter", id: "player.nextChapter", size: 22) { player.nextChapter() }
        }
        .buttonStyle(PlayerControlStyle())
        .foregroundStyle(Theme.foreground)
        .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
        .padding(.top, 20)
    }

    private func control(_ symbol: String, label: LocalizedStringKey, id: String, size: CGFloat, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: symbol)
                .font(.system(size: size, weight: .semibold))
                .frame(width: 44, height: 44)
        }
        .accessibilityLabel(label)
        .accessibilityIdentifier(id)
    }

    /// Speed in the web player's 0.1 steps, remembered per Book.
    private func tools(_ player: AudioPlayer) -> some View {
        VStack(spacing: 6) {
            HStack(spacing: 0) {
                Button { player.setRate(player.rate - PlaybackSpeed.step) } label: {
                    Image(systemName: "minus").frame(width: 44, height: 40)
                }
                .disabled(player.rate <= PlaybackSpeed.range.lowerBound)
                .accessibilityLabel("Slower")
                .accessibilityIdentifier("player.speedDown")
                Text(PlaybackSpeed.label(player.rate))
                    .font(.geist(.subheadline, weight: .heavy))
                    .monospacedDigit()
                    .frame(minWidth: 52)
                    .padding(.vertical, 8)
                    .overlay(alignment: .leading) { Rectangle().fill(Theme.border).frame(width: 1) }
                    .overlay(alignment: .trailing) { Rectangle().fill(Theme.border).frame(width: 1) }
                    .contentTransition(.numericText())
                    .accessibilityLabel("Speed \(PlaybackSpeed.label(player.rate))")
                    .accessibilityIdentifier("player.speed")
                Button { player.setRate(player.rate + PlaybackSpeed.step) } label: {
                    Image(systemName: "plus").frame(width: 44, height: 40)
                }
                .disabled(player.rate >= PlaybackSpeed.range.upperBound)
                .accessibilityLabel("Faster")
                .accessibilityIdentifier("player.speedUp")
            }
            .font(.geist(.footnote, weight: .semibold))
            .foregroundStyle(Theme.foreground)
            .background(Theme.background.opacity(0.5), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).strokeBorder(Theme.border))
            .animation(.snappy, value: player.rate)
            .dynamicTypeSize(...DynamicTypeSize.accessibility1)
            Text("Speed")
                .font(.geist(.caption))
                .foregroundStyle(Theme.foreground.opacity(0.66))
                .accessibilityHidden(true)
        }
        .frame(maxWidth: .infinity)
        .padding(.top, 22)
        .overlay(alignment: .top) {
            Rectangle().fill(Theme.foreground.opacity(0.1)).frame(height: 1).padding(.top, 8)
        }
    }
}
