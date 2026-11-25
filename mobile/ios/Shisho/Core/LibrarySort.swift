/// The order of a Library's Books from the user's saved sort for it, made of
/// the same expressions the server sorts with (`pkg/sortspec/sql.go`), so a
/// Library reads the same on the web and on the device. Change both together.
///
/// Levels on the reader's own reading data read the device's copy of it.
/// Last read isn't in the copy yet, so it's skipped, as are fields this
/// version doesn't know. With no level left, it's the web app's default:
/// newest first.
enum LibrarySort {
    /// An ORDER BY for `books b` in the device's copy, ending with the
    /// Book's id so ties always come out the same.
    static func orderBy(_ spec: String) -> String {
        var clauses: [String] = []
        for level in spec.split(separator: ",") {
            let parts = level.split(separator: ":")
            guard parts.count == 2, let direction = direction(parts[1]) else { continue }
            clauses += Self.clauses(field: String(parts[0]), direction: direction)
        }
        if clauses.isEmpty {
            clauses = Self.clauses(field: "date_added", direction: "DESC")
        }
        return (clauses + ["b.id ASC"]).joined(separator: ", ")
    }

    private static func direction(_ token: Substring) -> String? {
        switch token {
        case "asc": "ASC"
        case "desc": "DESC"
        default: nil
        }
    }

    private static func clauses(field: String, direction: String) -> [String] {
        switch field {
        case "title":
            return nullsLast("b.sort_title", direction)
        case "author":
            return nullsLast("(SELECT sort_name FROM book_authors WHERE book_id = b.id ORDER BY position LIMIT 1)", direction)
        case "series":
            // The first Series: its name, then single numbers before ranges,
            // then the number, then a range's end.
            let first = { (value: String) in
                "(SELECT \(value) FROM book_series WHERE book_id = b.id ORDER BY position LIMIT 1)"
            }
            return nullsLast(first("sort_name"), direction)
                + nullsLast(first("(number_end IS NOT NULL)"), "ASC")
                + nullsLast(first("number"), "ASC")
                + nullsLast(first("COALESCE(number_end, number)"), "ASC")
        case "date_added":
            return nullsLast("b.created_at", direction)
        case "date_released":
            return nullsLast(newestFile("release_date"), direction)
        case "page_count":
            return nullsLast(newestFile("page_count"), direction)
        case "word_count":
            return nullsLast(newestFile("word_count"), direction)
        case "duration":
            return nullsLast(newestFile("duration_seconds"), direction)
        case "reading_status":
            // The rank is never NULL.
            return ["\(statusRank("b.id")) \(direction)"]
        case "rating":
            return nullsLast("(SELECT rating FROM user_books WHERE book_id = b.id)", direction)
        case "date_finished":
            return nullsLast("(SELECT MAX(end_day) FROM read_throughs WHERE book_id = b.id AND outcome = 'finished')", direction)
        case "last_read":
            // When the Read-through underway was last read, which the server
            // works out as its `LastReadExpr` does, since the device can't
            // tell a place a link opened from reading.
            return nullsLast("(SELECT MAX(last_read_at) FROM read_throughs WHERE book_id = b.id AND outcome = 'underway')", direction)
        default:
            return []
        }
    }

    /// The Reading Status of the Book in bookColumn ranked for sorting, as
    /// `reading.StatusRankExpr` ranks it: Currently Reading 1 through
    /// Reference 6, and none 7.
    static func statusRank(_ bookColumn: String) -> String {
        """
        (CASE \(CatalogStore.statusExpression(bookColumn)) WHEN 'currently_reading' THEN 1 WHEN 'paused' THEN 2 WHEN 'to_read' THEN 3
            WHEN 'read' THEN 4 WHEN 'did_not_finish' THEN 5 WHEN 'reference' THEN 6 ELSE 7 END)
        """
    }

    /// Missing values last in either direction, as SQLite has no NULLS LAST.
    private static func nullsLast(_ expression: String, _ direction: String) -> [String] {
        ["\(expression) IS NULL", "\(expression) \(direction)"]
    }

    /// The Book's newest File's value, or else the newest File's that has
    /// one: newer editions are likelier to have the metadata.
    private static func newestFile(_ column: String) -> String {
        """
        COALESCE(
            (SELECT \(column) FROM book_files WHERE book_id = b.id ORDER BY id DESC LIMIT 1),
            (SELECT \(column) FROM book_files WHERE book_id = b.id AND \(column) IS NOT NULL ORDER BY id DESC LIMIT 1))
        """
    }
}

/// One field of a sort and its direction, as the web's sort sheet has them.
struct SortLevel: Hashable, Sendable {
    var field: String
    var descending: Bool
}

extension LibrarySort {
    /// The fields the sort sheet offers, in the web's order.
    static let fields = [
        "title", "author", "series", "date_added", "date_released",
        "page_count", "word_count", "duration", "reading_status", "rating", "date_finished", "last_read",
    ]
    /// As many levels as `pkg/sortspec` takes.
    static let maxLevels = 10
    /// The order of a Library with no sort saved on the web.
    static let builtInDefault = [SortLevel(field: "date_added", descending: true)]

    static func label(_ field: String) -> String {
        switch field {
        case "title": String(localized: "Title")
        case "author": String(localized: "Author")
        case "series": String(localized: "Series")
        case "date_added": String(localized: "Date added")
        case "date_released": String(localized: "Date released")
        case "page_count": String(localized: "Page count")
        case "word_count": String(localized: "Word count")
        case "duration": String(localized: "Duration")
        case "reading_status": String(localized: "Reading status")
        case "rating": String(localized: "Rating")
        case "date_finished": String(localized: "Date finished")
        case "last_read": String(localized: "Last read")
        default: field
        }
    }

    /// A newly added level puts the best rated and most recently finished
    /// or read first, as on the web.
    static func initiallyDescending(_ field: String) -> Bool {
        field == "rating" || field == "date_finished" || field == "last_read"
    }

    /// The levels of a spec in the sort parameter form, leaving out any it
    /// can't use and any field already named.
    static func levels(_ spec: String) -> [SortLevel] {
        var levels: [SortLevel] = []
        for level in spec.split(separator: ",") {
            let parts = level.split(separator: ":")
            guard parts.count == 2, fields.contains(String(parts[0])), parts[1] == "asc" || parts[1] == "desc",
                  !levels.contains(where: { $0.field == parts[0] }) else { continue }
            levels.append(SortLevel(field: String(parts[0]), descending: parts[1] == "desc"))
        }
        return Array(levels.prefix(maxLevels))
    }

    static func spec(_ levels: [SortLevel]) -> String {
        levels.map { "\($0.field):\($0.descending ? "desc" : "asc")" }.joined(separator: ",")
    }
}

/// The Library's sort as the web gallery works it out: the sort picked on
/// this device wins, then the one saved on the web, then newest first.
struct GallerySort: Equatable {
    /// The sort saved on the web, in the sort parameter form.
    let saved: String
    /// The sort picked on this device, or nil for the saved one.
    let device: String?

    var defaultLevels: [SortLevel] {
        let saved = LibrarySort.levels(saved)
        return saved.isEmpty ? LibrarySort.builtInDefault : saved
    }

    var levels: [SortLevel] {
        let picked = device.map(LibrarySort.levels) ?? []
        return picked.isEmpty ? defaultLevels : picked
    }

    /// Whether the order differs from the saved one, as the web's dot says.
    var isDirty: Bool { levels != defaultLevels }

    /// What to keep as this device's pick for levels: nil when they're the
    /// saved sort, or empty, which goes back to it, as the web's URL does.
    func deviceSpec(for levels: [SortLevel]) -> String? {
        levels.isEmpty || levels == defaultLevels ? nil : LibrarySort.spec(levels)
    }
}
