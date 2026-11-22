import Foundation

/// Counts as words, for labels outside a SwiftUI Text. A Text inflects
/// "^[3 book](inflect: true)" itself; String(localized:) leaves the markup
/// as it is, so a String goes through AttributedString, which inflects.
enum Counts {
    /// "1 book", "3 books".
    static func books(_ count: Int) -> String {
        String(AttributedString(localized: "^[\(count) book](inflect: true)").characters)
    }
}

/// Ratings, kept in quarter-star units, as the web app words them.
enum Ratings {
    /// Stars from quarter-star units, as "4.25".
    static func text(_ units: Int) -> String {
        (Double(units) / 4).formatted(.number.precision(.fractionLength(0...2)))
    }

    /// A Rating filter's range, as the web's `ratingRangeLabel` words it;
    /// either end nil is open.
    static func range(min: Int?, max: Int?) -> String {
        switch (min, max) {
        case let (min?, max?) where min == max:
            // Only exactly one star is singular; inflection can't count a
            // fraction.
            return min == 4 ? String(localized: "1 star") : String(localized: "\(text(min)) stars")
        case let (min?, max?):
            return String(localized: "\(text(min)) to \(text(max)) stars")
        case let (min?, nil):
            return String(localized: "\(text(min))+ stars")
        case let (nil, max?):
            return String(localized: "Up to \(text(max)) stars")
        case (nil, nil):
            return String(localized: "Any rating")
        }
    }
}
