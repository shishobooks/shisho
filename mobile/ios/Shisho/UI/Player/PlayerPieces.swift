import SwiftUI

/// Three bars that bounce while audio plays and rest when it doesn't, as
/// variant E marks what's playing.
struct PlayingIndicator: View {
    let playing: Bool
    var height: CGFloat = 11
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        TimelineView(.animation(paused: !playing || reduceMotion)) { context in
            let time = context.date.timeIntervalSinceReferenceDate
            HStack(alignment: .bottom, spacing: 2) {
                ForEach(0..<3, id: \.self) { bar in
                    let phase = time * 7 + Double(bar) * 2.1
                    let scale = playing && !reduceMotion ? 0.35 + 0.65 * abs(sin(phase)) : 0.35
                    RoundedRectangle(cornerRadius: 1)
                        .frame(width: 2.5, height: height * scale)
                }
            }
            .frame(height: height, alignment: .bottom)
        }
        .accessibilityHidden(true)
    }
}

/// The skip buttons' icon: an arrow around the seconds it skips, the
/// listener's setting.
struct SkipIcon: View {
    let forward: Bool
    let seconds: Double
    var size: CGFloat = 30

    /// The system's icon for a skip, which exists for each of
    /// `PlayerSettings.skipLengths`.
    nonisolated static func symbol(forward: Bool, seconds: Double) -> String {
        "\(forward ? "goforward" : "gobackward").\(Int(seconds))"
    }

    var body: some View {
        Image(systemName: SkipIcon.symbol(forward: forward, seconds: seconds))
            .font(.system(size: size, weight: .regular))
            .contentTransition(.symbolEffect(.replace))
            .accessibilityHidden(true)
    }
}

/// A round control in the player that gives under the finger.
struct PlayerControlStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .contentShape(Rectangle())
            .scaleEffect(configuration.isPressed ? 0.9 : 1)
            .animation(.spring(response: 0.26, dampingFraction: 0.6), value: configuration.isPressed)
    }
}

/// Offers another device's place without jumping to it (#912).
struct PlaceOfferBanner: View {
    let offer: ServerPlace
    let accept: () -> Void
    let dismiss: () -> Void

    private var message: String {
        let time = TimeLabels.clock(offer.seconds)
        return offer.source == "web_reader"
            ? String(localized: "You listened up to \(time) on the web.")
            : String(localized: "You listened up to \(time) on another device.")
    }

    var body: some View {
        HStack(alignment: .center, spacing: 12) {
            Image(systemName: "arrow.triangle.2.circlepath")
                .font(.geist(.body, weight: .semibold))
                .foregroundStyle(Theme.primary)
                .accessibilityHidden(true)
            Text(message)
                .font(.geist(.subheadline))
                .foregroundStyle(Theme.foreground)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
            Button("Go There", action: accept)
                .font(.geist(.subheadline, weight: .semibold))
                .foregroundStyle(Theme.primary)
                .accessibilityIdentifier("player.offer.accept")
            Button(action: dismiss) {
                Image(systemName: "xmark")
                    .font(.geist(.footnote, weight: .semibold))
                    .foregroundStyle(Theme.mutedForeground)
                    .frame(width: 28, height: 28)
            }
            .accessibilityLabel("Stay Here")
            .accessibilityIdentifier("player.offer.dismiss")
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .cardStyle()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("player.offer")
    }
}

/// The player's position bar. Only its handle moves it: drag the handle to
/// a place, as Apple's own players do; a tap on the bar does nothing. It goes
/// exactly where the finger lifts, with no momentum, which the system
/// slider's thumb has. value and length are seconds within what the bar
/// spans.
struct Scrubber: View {
    let value: Double
    let length: Double
    /// Called with the place under the handle while it's dragged, and nil
    /// once it's let go.
    var onDrag: (Double?) -> Void = { _ in }
    /// Called once with where the handle was let go.
    let onCommit: (Double) -> Void
    /// What VoiceOver's swipe up and down moves by.
    var step: Double = 15

    /// Where a drag began, and where the handle is now, in seconds.
    @State private var dragStart: Double?
    @State private var dragging: Double?
    @GestureState private var touching = false

    private static let thumb: CGFloat = 20
    private static let track: CGFloat = 8
    /// The handle's touch target, the size Apple's guidelines ask for.
    private static let target: CGFloat = 44

    var body: some View {
        GeometryReader { geometry in
            let width = geometry.size.width
            let travel = max(1, width - Scrubber.thumb)
            let shown = dragging ?? value
            let fraction = length > 0 ? min(1, max(0, shown / length)) : 0
            let x = travel * fraction
            ZStack(alignment: .leading) {
                Capsule()
                    .fill(Theme.foreground.opacity(0.14))
                    .frame(height: Scrubber.track)
                Capsule()
                    .fill(Theme.primary)
                    .frame(width: min(width, x + Scrubber.thumb / 2), height: Scrubber.track)
                // White in both appearances, as the system slider's handle
                // is, ringed in the brand color.
                Circle()
                    .fill(Color.white)
                    .overlay(Circle().strokeBorder(Theme.primary, lineWidth: 2))
                    .shadow(color: .black.opacity(0.18), radius: 3, y: 1)
                    .frame(width: Scrubber.thumb, height: Scrubber.thumb)
                    // Only the handle grows; nothing it moves along is
                    // animated, so it can't overshoot.
                    .scaleEffect(dragging == nil ? 1 : 1.25)
                    .animation(.snappy(duration: 0.15), value: dragging == nil)
                    .frame(width: Scrubber.target, height: Scrubber.target)
                    .contentShape(Rectangle())
                    .gesture(
                        DragGesture(minimumDistance: 1, coordinateSpace: .named("scrubber"))
                            .updating($touching) { _, touching, _ in touching = true }
                            .onChanged { drag in
                                // A new drag starts from the handle.
                                let start = dragging == nil ? value : dragStart ?? value
                                dragStart = start
                                let place = min(length, max(0, start + Double(drag.translation.width / travel) * length))
                                dragging = place
                                onDrag(place)
                            }
                            .onEnded { drag in
                                // Where the finger lifted, which a quick
                                // release may not have reported to onChanged.
                                let start = dragStart ?? value
                                let place = min(length, max(0, start + Double(drag.translation.width / travel) * length))
                                dragStart = nil
                                dragging = nil
                                onDrag(nil)
                                onCommit(place)
                            }
                    )
                    // A drag the system cancelled never ends: it lets go of
                    // the handle without seeking. dragStart stays for an
                    // onEnded that comes after this.
                    .onChange(of: touching) { _, touching in
                        guard !touching, dragging != nil else { return }
                        dragging = nil
                        onDrag(nil)
                    }
                    .offset(x: x - (Scrubber.target - Scrubber.thumb) / 2)
            }
            .frame(width: width, height: geometry.size.height)
            .coordinateSpace(name: "scrubber")
        }
        .frame(height: Scrubber.thumb + 4)
        .accessibilityElement()
        .accessibilityAddTraits(.updatesFrequently)
        .accessibilityAdjustableAction { direction in
            switch direction {
            case .increment: onCommit(min(length, value + step))
            case .decrement: onCommit(max(0, value - step))
            @unknown default: break
            }
        }
    }
}
