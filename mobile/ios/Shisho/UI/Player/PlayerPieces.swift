import SwiftUI

/// Three bars that bounce while audio plays and rest when it doesn't, as
/// variant E marks what's playing.
struct PlayingIndicator: View {
    let playing: Bool
    var height: CGFloat = 11
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        // 30 frames a second is smooth for three small bars, and lets the
        // screen drop below its full rate while audio plays.
        TimelineView(.animation(minimumInterval: 1.0 / 30, paused: !playing || reduceMotion)) { context in
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

/// The skip buttons' icon, as the web player draws it: Lucide's rotate arrow,
/// back or forward, around the seconds it skips, the listener's setting. It
/// stays one size, as the player's controls do.
struct SkipIcon: View {
    let forward: Bool
    let seconds: Double
    var size: CGFloat = 30

    var body: some View {
        ZStack {
            Icon(forward ? .rotateCw : .rotateCcw, size: size, relativeTo: nil)
            Text(verbatim: "\(Int(seconds))")
                .font(.custom(Theme.Fonts.geist, fixedSize: size * 0.3).weight(.bold))
                .monospacedDigit()
                .offset(y: size * 0.02)
        }
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

/// Offers another device's place without jumping to it (#912), in the
/// player or the reader.
struct PlaceOfferBanner: View {
    enum Kind {
        case listening, reading

        /// Starts the banner's identifiers, the same on every platform.
        var prefix: String { self == .listening ? "player" : "reader" }
    }

    let offer: ServerPlace
    var kind = Kind.listening
    let accept: () -> Void
    let dismiss: () -> Void

    private var message: String { PlaceOfferBanner.message(for: offer, kind: kind) }

    /// What the banner says about offer, which VoiceOver reads too: where
    /// the place came from, by the Device's name when it has one.
    nonisolated static func message(for offer: ServerPlace, kind: Kind) -> String {
        let web = offer.source == "web_reader"
        var device = offer.deviceName?.trimmingCharacters(in: .whitespacesAndNewlines)
        if device?.isEmpty == true { device = nil }
        switch kind {
        case .listening:
            let time = TimeLabels.clock(offer.seconds)
            if web { return String(localized: "You listened up to \(time) on the web.") }
            if let device { return String(localized: "You listened up to \(time) on \(device).") }
            return String(localized: "You listened up to \(time) on another device.")
        case .reading:
            let percent = offer.percentage.formatted(.percent.precision(.fractionLength(0)))
            if web { return String(localized: "You read up to \(percent) on the web.") }
            if let device { return String(localized: "You read up to \(percent) on \(device).") }
            return String(localized: "You read up to \(percent) on another device.")
        }
    }

    var body: some View {
        HStack(alignment: .center, spacing: 12) {
            Icon(.refresh, size: 18)
                .foregroundStyle(Theme.primary)
            Text(message)
                .font(.geist(.subheadline))
                .foregroundStyle(Theme.foreground)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
            Button("Go There", action: accept)
                .font(.geist(.subheadline, weight: .semibold))
                .foregroundStyle(Theme.primary)
                .accessibilityIdentifier("\(kind.prefix).offer.accept")
            Button(action: dismiss) {
                Icon(.x, size: 16, relativeTo: .footnote)
                    .foregroundStyle(Theme.mutedForeground)
                    .frame(width: 28, height: 28)
            }
            .accessibilityLabel("Stay Here")
            .accessibilityIdentifier("\(kind.prefix).offer.dismiss")
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .cardStyle()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("\(kind.prefix).offer")
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
    /// Called for each of VoiceOver's swipes up and down instead of
    /// onCommit, for a bar that treats a run of them as one move.
    var onAdjust: ((Double) -> Void)?
    /// Places marked on the bar, such as where each chapter starts, in the
    /// same units as value.
    var marks: [Double] = []

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
                // One drawing for every mark, rather than a view each: a
                // book can have a hundred chapters.
                if !marks.isEmpty {
                    Canvas { context, size in
                        for mark in marks {
                            let centre = Scrubber.thumb / 2 + travel * min(1, max(0, length > 0 ? mark / length : 0))
                            let tick = CGRect(x: centre - 0.75, y: (size.height - 12) / 2, width: 1.5, height: 12)
                            context.fill(Capsule().path(in: tick), with: .color(Theme.foreground.opacity(0.3)))
                        }
                    }
                    .frame(width: width, height: 12)
                    .allowsHitTesting(false)
                    .accessibilityHidden(true)
                }
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
            case .increment: (onAdjust ?? onCommit)(min(length, value + step))
            case .decrement: (onAdjust ?? onCommit)(max(0, value - step))
            @unknown default: break
            }
        }
    }
}
