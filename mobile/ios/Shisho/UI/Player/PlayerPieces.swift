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

    private var whereText: String {
        offer.source == "web_reader" ? String(localized: "on the web") : String(localized: "on another device")
    }

    var body: some View {
        HStack(alignment: .center, spacing: 12) {
            Image(systemName: "arrow.triangle.2.circlepath")
                .font(.geist(.body, weight: .semibold))
                .foregroundStyle(Theme.primary)
                .accessibilityHidden(true)
            Text("You listened to \(TimeLabels.clock(offer.seconds)) \(whereText).")
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
