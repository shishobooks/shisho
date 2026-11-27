import SwiftUI

/// A button's background filled from the left as far as the user is through
/// what it opens, so the button is its own progress bar: the Book page's
/// Listen and Read cards, and Home's resume cards.
struct ProgressFill: View {
    enum Style {
        /// The purple call to action: the part done in a deeper purple, so
        /// the label reads the same over both.
        case prominent
        /// A quiet button: a muted track with the part done in purple.
        case subtle
    }

    /// How far through, 0 to 1.
    let fraction: Double
    var style = Style.prominent

    var body: some View {
        GeometryReader { geometry in
            ZStack(alignment: .leading) {
                switch style {
                case .prominent:
                    Theme.primary
                    Color.black.opacity(0.2)
                        .frame(width: geometry.size.width * clamped)
                case .subtle:
                    Theme.secondary
                    Theme.primary.opacity(0.3)
                        .frame(width: geometry.size.width * clamped)
                }
            }
        }
        .accessibilityHidden(true)
    }

    private var clamped: Double { min(1, max(0, fraction)) }
}
