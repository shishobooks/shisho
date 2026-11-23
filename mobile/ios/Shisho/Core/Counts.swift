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

enum FileFacts {
    /// A length as the web shows it: "6h 43m", "12m", or "<1m".
    static func duration(_ seconds: Double) -> String {
        if seconds > 0, seconds < 60 { return "<1m" }
        let hours = Int(seconds) / 3600
        let minutes = (Int(seconds) % 3600) / 60
        return hours > 0 ? "\(hours)h \(minutes)m" : "\(minutes)m"
    }

    static func bitrate(_ bitsPerSecond: Int) -> String {
        "\(Int((Double(bitsPerSecond) / 1000).rounded())) kbps"
    }

    /// The year a date falls in, read in UTC as the web does.
    static func year(_ date: Date) -> String {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .current
        return String(calendar.component(.year, from: date))
    }

    static func pages(_ count: Int) -> String {
        String(AttributedString(localized: "^[\(count) page](inflect: true)").characters)
    }

    static func size(_ bytes: Int) -> String {
        ByteCountFormatter.string(fromByteCount: Int64(bytes), countStyle: .file)
    }
}

/// How far through a File the user is, as its row says it under its
/// facts: the percentage, and the time or pages left.
enum FileProgressText {
    static func text(_ progress: Double, durationSeconds: Double?, pageCount: Int?) -> String {
        let clamped = min(1, max(0, progress))
        let percent = Percent.text(clamped)
        guard clamped < 1 else { return percent }
        let rest = 1 - clamped
        // To the second, as the Listen card says it, so the two agree.
        if let seconds = durationSeconds, seconds > 0 {
            // To the millisecond first, so the fraction's rounding error
            // can't take a second off.
            let left = (seconds * rest * 1000).rounded() / 1000
            return String(localized: "\(percent) · \(TimeLabels.clock(left)) left")
        }
        if let pages = pageCount, pages > 0 {
            return String(localized: "\(percent) · \(FileFacts.pages(Int((Double(pages) * rest).rounded()))) left")
        }
        return percent
    }
}

enum SeriesNumbers {
    /// A Series Number as the web writes it: "1", "1.5".
    static func format(_ number: Double) -> String {
        number.formatted(.number.precision(.fractionLength(0...2)))
    }

    /// A short label as the web's `seriesNumberLabel` writes it: "#5", or a
    /// comic's "Vol. 5" or "Ch. 42".
    static func label(_ number: Double, end: Double?, unit: String?, comic: Bool) -> String {
        let range = format(number) + (end.map { "-" + format($0) } ?? "")
        guard comic else { return "#" + range }
        return unit == "chapter" ? String(localized: "Ch. \(range)") : String(localized: "Vol. \(range)")
    }
}

/// A fraction as the web's `formatProgress` writes it: "37%".
enum Percent {
    static func text(_ value: Double) -> String {
        "\(Int((min(1, max(0, value)) * 100).rounded()))%"
    }
}
