import CoreGraphics

/// Where a footnote's card sits beside its reference: above it when there's
/// room, else below, inside the page's margins, with its arrow on the
/// reference and clear of the card's rounded corners.
struct NotePlacement: Equatable {
    static let margin: CGFloat = 16
    static let gap: CGFloat = 4
    static let arrowWidth: CGFloat = 16
    static let arrowHeight: CGFloat = 8
    static let radius: CGFloat = 10

    /// The card's top-leading corner in the page.
    let origin: CGPoint
    let above: Bool
    /// The arrow's middle, from the card's leading edge.
    let arrowX: CGFloat

    init(reference: CGRect, card: CGSize, in page: CGSize) {
        let reach = card.height + Self.arrowHeight + Self.gap
        above = reference.minY - reach >= Self.margin || reference.maxY + reach > page.height - Self.margin
        let y = above ? reference.minY - reach : reference.maxY + Self.gap + Self.arrowHeight
        let x = min(max(Self.margin, reference.midX - card.width / 2), max(Self.margin, page.width - Self.margin - card.width))
        origin = CGPoint(x: x, y: y)
        let inset = Self.radius + Self.arrowWidth / 2
        arrowX = min(max(inset, reference.midX - x), max(inset, card.width - inset))
    }
}
