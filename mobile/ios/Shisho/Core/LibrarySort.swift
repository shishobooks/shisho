/// The order of a Library's Books from the user's saved sort for it, made of
/// the same expressions the server sorts with (`pkg/sortspec/sql.go`), so a
/// Library reads the same on the web and on the device. Change both together.
///
/// Levels on the reader's own reading data aren't in the device's copy yet,
/// so they're skipped, as are fields this version doesn't know. With no
/// level left, it's the web app's default: newest first.
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
        case "duration":
            return nullsLast(newestFile("duration_seconds"), direction)
        default:
            return []
        }
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
