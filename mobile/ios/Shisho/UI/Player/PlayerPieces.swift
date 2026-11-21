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

/// The skip buttons' icon: an arrow around the seconds it skips.
struct SkipIcon: View {
    let forward: Bool
    var size: CGFloat = 30

    var body: some View {
        Image(systemName: forward ? "goforward.30" : "gobackward.30")
            .font(.system(size: size, weight: .regular))
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

/// The player's position bar: drag anywhere on it to go to a place. It goes
/// exactly where the finger lifts, with no momentum, as Apple's own players'
/// scrubbers do; the system slider's thumb keeps moving after a fast drag.
/// value and length are seconds within what the bar spans.
struct Scrubber: View {
    let value: Double
    let length: Double
    /// Called with the place under the finger while dragging, and nil once
    /// it lifts.
    var onDrag: (Double?) -> Void = { _ in }
    /// Called once with where the finger lifted.
    let onCommit: (Double) -> Void
    /// What VoiceOver's swipe up and down moves by.
    var step: Double = 15

    @State private var dragging: Double?

    private static let thumb: CGFloat = 20

    var body: some View {
        GeometryReader { geometry in
            let track = max(1, geometry.size.width - Scrubber.thumb)
            let shown = dragging ?? value
            let fraction = length > 0 ? min(1, max(0, shown / length)) : 0
            ZStack(alignment: .leading) {
                Capsule()
                    .fill(Theme.foreground.opacity(0.14))
                    .frame(height: dragging == nil ? 6 : 10)
                Capsule()
                    .fill(Theme.primary)
                    .frame(width: Scrubber.thumb / 2 + track * fraction, height: dragging == nil ? 6 : 10)
                Circle()
                    .fill(Color.white)
                    .overlay(Circle().strokeBorder(Theme.primary, lineWidth: 2))
                    .shadow(color: .black.opacity(0.18), radius: 3, y: 1)
                    .frame(width: Scrubber.thumb, height: Scrubber.thumb)
                    .scaleEffect(dragging == nil ? 1 : 1.2)
                    .offset(x: track * fraction)
            }
            .frame(maxHeight: .infinity)
            .contentShape(Rectangle())
            .gesture(
                DragGesture(minimumDistance: 0)
                    .onChanged { drag in
                        let place = seconds(at: drag.location.x, track: track)
                        dragging = place
                        onDrag(place)
                    }
                    .onEnded { drag in
                        let place = seconds(at: drag.location.x, track: track)
                        dragging = nil
                        onDrag(nil)
                        onCommit(place)
                    }
            )
            .animation(.snappy(duration: 0.2), value: dragging == nil)
        }
        .frame(height: 32)
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

    private func seconds(at x: CGFloat, track: CGFloat) -> Double {
        let fraction = min(1, max(0, (x - Scrubber.thumb / 2) / track))
        return Double(fraction) * length
    }
}
