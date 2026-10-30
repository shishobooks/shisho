/**
 * Parses, validates, and serializes gallery sort specs.
 * Mirrors pkg/sortspec (Go). Keep SORT_FIELDS and the grammar in sync
 * with the Go whitelist — a matching test in the Go side pins the list.
 */

export type SortDirection = "asc" | "desc";

export type SortField =
  | "title"
  | "author"
  | "series"
  | "date_added"
  | "date_released"
  | "page_count"
  | "duration"
  | "reading_status"
  | "rating"
  | "date_finished";

/**
 * One field and direction. The field defaults to the book gallery's fields;
 * the resource lists pass their own (app/libraries/resourceSort.ts).
 */
export interface SortLevel<F extends string = SortField> {
  field: F;
  direction: SortDirection;
}

/** Canonical list of sort fields in UI display order. */
export const SORT_FIELDS: readonly SortField[] = [
  "title",
  "author",
  "series",
  "date_added",
  "date_released",
  "page_count",
  "duration",
  "reading_status",
  "rating",
  "date_finished",
] as const;

/** Human-readable labels for each field. */
export const SORT_FIELD_LABELS: Record<SortField, string> = {
  title: "Title",
  author: "Author",
  series: "Series",
  date_added: "Date added",
  date_released: "Date released",
  page_count: "Page count",
  duration: "Duration",
  reading_status: "Reading status",
  rating: "Rating",
  date_finished: "Date finished",
};

/**
 * Fields a newly added sort level orders descending: the reader's best
 * rated and most recently finished books come first.
 */
const DESCENDING_FIRST: ReadonlySet<string> = new Set([
  "rating",
  "date_finished",
]);

/** The direction a newly added level of field starts in. */
export const initialSortDirection = (field: string): SortDirection =>
  DESCENDING_FIRST.has(field) ? "desc" : "asc";

/**
 * The fields a sorted view offers and the order it shows when the URL has no
 * sort. Each view's fields must match what its endpoint accepts.
 */
export interface SortConfig<F extends string> {
  /** Fields in UI display order. */
  fields: readonly F[];
  labels: Record<F, string>;
  /**
   * The order without a URL sort. Empty means the endpoint's own order,
   * which `emptyLabel` names in lowercase ("series order").
   */
  defaultSort: readonly SortLevel<F>[];
  emptyLabel?: string;
}

/** Hard cap matching pkg/sortspec.MaxLevels. */
export const MAX_SORT_LEVELS = 10;

/**
 * Builtin default when the URL has no sort and the DB has no saved default.
 *
 * Mirrors `sortspec.BuiltinDefault()` in pkg/sortspec/spec.go (Go).
 * Keep both sides in sync — the OPDS and eReader handlers fall back to
 * the Go side when no preference is stored, so a drift here would mean
 * the gallery, OPDS feeds, and eReader UI list the same library in
 * different orders for users who have not saved a default.
 */
export const BUILTIN_DEFAULT_SORT: readonly SortLevel[] = [
  { field: "date_added", direction: "desc" },
];

function isSortDirection(s: string): s is SortDirection {
  return s === "asc" || s === "desc";
}

/**
 * Parse a serialized book sort spec. Returns null for any invalid input;
 * callers should treat that as "no sort specified" and fall back.
 */
export function parseSortSpec(s: string): SortLevel[] | null {
  return parseSortSpecFor(s, SORT_FIELDS);
}

/** parseSortSpec for a caller-supplied field whitelist. */
export function parseSortSpecFor<F extends string>(
  s: string,
  fields: readonly F[],
): SortLevel<F>[] | null {
  const isField = (f: string): f is F =>
    (fields as readonly string[]).includes(f);
  if (!s) return null;
  if (/\s/.test(s)) return null;

  const parts = s.split(",");
  if (parts.length > MAX_SORT_LEVELS) return null;

  const levels: SortLevel<F>[] = [];
  const seen = new Set<string>();

  for (const part of parts) {
    if (!part) return null;
    // Use unbounded split + length check so trailing-colon junk like
    // "title:asc:extra" rejects instead of being silently truncated by
    // JS's split(_, 2). This mirrors Go's strings.SplitN(_, _, 2) +
    // direction-validation behavior in pkg/sortspec.
    const pieces = part.split(":");
    if (pieces.length !== 2) return null;
    const [field, direction] = pieces;
    if (!field || !direction) return null;
    if (!isField(field)) return null;
    if (!isSortDirection(direction)) return null;
    if (seen.has(field)) return null;
    seen.add(field);
    levels.push({ field, direction });
  }

  return levels;
}

/**
 * Serialize a spec back into the URL-param form.
 *
 * Returns "" for an empty array — callers MUST treat empty output as
 * "no sort" and either omit the URL param entirely or skip sending a
 * `sort` query param to the API. Setting `?sort=` with an empty value
 * would round-trip through parseSortSpec as null (no sort) but pollutes
 * the URL with a meaningless empty key.
 */
export function serializeSortSpec(
  levels: readonly SortLevel<string>[],
): string {
  return levels.map((l) => `${l.field}:${l.direction}`).join(",");
}

/** Deep equality for two specs (order matters). */
export function sortSpecsEqual(
  a: readonly SortLevel<string>[] | null | undefined,
  b: readonly SortLevel<string>[] | null | undefined,
): boolean {
  if (!a || !b) return !a && !b;
  if (a.length !== b.length) return false;
  return a.every(
    (l, i) => l.field === b[i].field && l.direction === b[i].direction,
  );
}

/**
 * Books on tag, genre, and person pages: newest release first. Books without
 * a release date sort last in either direction. Keep the default non-empty:
 * these pages list through /books with a library_id, which applies the
 * user's saved gallery sort whenever no sort is sent.
 */
export const BOOK_DETAIL_SORT: SortConfig<SortField> = {
  fields: SORT_FIELDS,
  labels: SORT_FIELD_LABELS,
  defaultSort: [
    { field: "date_released", direction: "desc" },
    { field: "title", direction: "asc" },
  ],
};

/**
 * Books on a series page keep series order until a sort is chosen. Series is
 * left out because it orders by each book's first series, not this one.
 */
export const SERIES_BOOKS_SORT: SortConfig<Exclude<SortField, "series">> = {
  fields: SORT_FIELDS.filter(
    (f): f is Exclude<SortField, "series"> => f !== "series",
  ),
  labels: SORT_FIELD_LABELS,
  defaultSort: [],
  emptyLabel: "series order",
};
