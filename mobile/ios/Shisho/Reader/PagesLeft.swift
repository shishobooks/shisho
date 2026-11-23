import Foundation

/// Pages left to a place further on, as the reader's bar shows for the
/// chapter and the book. The section showing is laid out, so its pages are
/// exact; past it the count is an estimate, at that section's pages per
/// share of the book, since the rest isn't laid out.
enum PagesLeft {
    /// The section showing, as a share of the book, and the page showing
    /// in it, from 1.
    struct Section: Equatable {
        let from: Double
        let to: Double
        let page: Int
        let pages: Int
    }

    /// Pages after the one showing, up to the one end falls on. Past the
    /// section, pagesPerShare is the rate measured over the sections seen,
    /// or this section's own when there's none.
    static func count(in section: Section, to end: Double, pagesPerShare: Double? = nil) -> Int {
        let pages = max(1, section.pages)
        let page = min(max(1, section.page), pages)
        let perPage = (section.to - section.from) / Double(pages)
        guard perPage > 0 else { return 0 }
        let leftHere = pages - page
        // Within a hair of the section's end is its end.
        if end >= section.to - perPage / 2 {
            let rate = pagesPerShare ?? 1 / perPage
            return leftHere + max(0, Int(((end - section.to) * rate).rounded()))
        }
        let pageStart = section.from + Double(page - 1) * perPage
        let endPage = Int(((end - pageStart) / perPage).rounded(.up))
        return min(max(0, endPage - 1), leftHere)
    }

    /// Pages after the one showing, before the chapter that starts on page
    /// next in the section: the page it starts on is the next chapter's
    /// when it starts at the top, and this one's too when it starts lower.
    static func count(in section: Section, toChapterOn next: Int, atTop: Bool) -> Int {
        max(0, next - section.page - (atTop ? 1 : 0))
    }

    /// Pages per share of the book over the sections seen, each weighed by
    /// its share, so a short title page barely moves it.
    static func pagesPerShare(_ sections: [(pages: Int, share: Double)]) -> Double? {
        let share = sections.reduce(0) { $0 + $1.share }
        guard share > 0 else { return nil }
        return Double(sections.reduce(0) { $0 + $1.pages }) / share
    }
}
