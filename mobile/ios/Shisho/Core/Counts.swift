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
