# Omnibus series number ranges

Status: accepted

## Context and decision

A book can be an omnibus that collects a contiguous run of a series' entries (for example, one volume containing books 1 through 3). We record this as a range on the existing single `book_series` membership: `series_number` holds the range start and a new nullable `series_number_end` holds the end. A normal single book leaves `series_number_end` NULL, so all existing rows and every numeric consumer keep reading `series_number` as before. The start, end, and unit move together as one atomic group through every metadata source, merge, priority, and sidecar path, so no source can partially overwrite a range (no start from one source and end from another), and clearing the group clears all three values.

A present stored group has a finite start and an optional finite end strictly greater than the start; an end cannot exist without a start. An unnumbered membership leaves the start, end, and unit unset. The API normalizes an end equal to the start into a single number, while external parsers reject equal or reversed endpoints. A malformed external group is discarded atomically rather than partially merged.

Omnibuses are ordered after the individually numbered books of a series, not interleaved at their start position. The series book list uses `(series_number_end IS NOT NULL) ASC, series_number ASC, COALESCE(series_number_end, series_number) ASC, sort_title ASC`. Series-cover selection prepends the omnibus discriminator to its existing whole-number preference: `(series_number_end IS NOT NULL) ASC, CASE WHEN series_number = CAST(series_number AS INTEGER) THEN 0 ELSE 1 END ASC, series_number ASC, COALESCE(series_number_end, series_number) ASC, title ASC`. This keeps fractional prequels and unnumbered books from replacing a numbered entry as the cover, and matches the convention readers already see on Goodreads and Audible, where an omnibus sits after the numbered entries rather than among them.

Range support round-trips through the file formats that can represent it. CBZ (ComicInfo `Number`, plus the `v001-003` / `c005-008` organized filename) and M4B (`SERIES-PART` and the grouping atom) both parse and write `start-end`, tolerantly on read and with a clean hyphenated form on write. EPUB (`calibre:series_index`, numeric) and Kobo sync (numeric fields) carry the start only. The sidecar, the plugin bridge, and the TypeScript SDK gain additive, optional range fields.

## Considered options

- **Multiple memberships, one row per covered position.** Rejected: the existing `UNIQUE(book_id, series_id)` index forbids it, and it would make a single omnibus appear three separate times in the series list while erasing the fact that one book covers the whole run.
- **A positions child table (`book_series_positions`).** Rejected as premature. It would natively support non-contiguous sets, but real omnibuses are contiguous spans, non-contiguous bundles are rare, and the child table imposes a permanent join-and-aggregate tax across scan, merge, sidecar, four file-format writers, OPDS, Kobo, and the frontend.
- **A free-text number string (`"1-3"`).** Rejected: stringly typed, drops the numeric end needed for sorting and range queries, and conflates display with data.
- **Contiguous `start + end` (chosen).** The smallest durable change, it keeps every numeric consumer reading `series_number` as the start and models an omnibus as the contiguous span it actually is.

## Consequences

- Non-contiguous bundles (for example books 1, 2, 10) are explicitly out of scope. They are a distinct concept (a discrete bundle, not a span) that can be added later. The design stays forward compatible: `series_number` remains the canonical sort key, label formatting lives behind one formatter per stack, and the sidecar and plugin fields are additive, so a future set model (`numbers[]`) is a migration rather than a rewrite.
- EPUB embedded metadata and Kobo's numeric series fields carry the start only. Shisho's DB-backed displays still show the full stored range. Ordinary scans preserve a manually set range through the sidecar, but refresh and reset discard cached sidecars by design; if the underlying metadata source carries only the start, those modes reduce the range to a single number.
