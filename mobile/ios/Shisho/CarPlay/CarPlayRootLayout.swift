/// How CarPlay's first list fits the rows a car allows, which some cars
/// limit to a dozen: Continue Listening, then Downloads, then each Library,
/// or a single Libraries row leading to them when they don't all fit.
struct CarPlayRootLayout: Equatable {
    /// How many audiobooks Continue Listening shows, at most.
    static let continueListeningLimit = 6

    /// How many audiobooks Continue Listening shows.
    let continueListening: Int
    /// Each Library has its own row; otherwise one Libraries row leads to
    /// them.
    let librariesInline: Bool

    /// The layout for continueListening audiobooks and libraries Libraries
    /// in a list of at most maxItems rows. Downloads, and a way to the
    /// Libraries, always keep their rows.
    static func fit(continueListening: Int, libraries: Int, maxItems: Int) -> CarPlayRootLayout {
        let others = 1 + (libraries > 0 ? 1 : 0)
        let shown = max(0, min(continueListening, continueListeningLimit, maxItems - others))
        return CarPlayRootLayout(continueListening: shown, librariesInline: shown + 1 + libraries <= maxItems)
    }
}
