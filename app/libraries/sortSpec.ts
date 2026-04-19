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
  | "duration";

export interface SortLevel {
  field: SortField;
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
};

/** Hard cap matching pkg/sortspec.MaxLevels. */
export const MAX_SORT_LEVELS = 10;

/** Builtin default when the URL has no sort and the DB has no saved default. */
export const BUILTIN_DEFAULT_SORT: readonly SortLevel[] = [
  { field: "date_added", direction: "desc" },
];

const FIELD_SET = new Set<string>(SORT_FIELDS);

function isSortField(s: string): s is SortField {
  return FIELD_SET.has(s);
}

function isSortDirection(s: string): s is SortDirection {
  return s === "asc" || s === "desc";
}

/**
 * Parse a serialized sort spec. Returns null for any invalid input —
 * callers should treat that as "no sort specified" and fall back.
 */
export function parseSortSpec(s: string): SortLevel[] | null {
  if (!s) return null;
  if (/\s/.test(s)) return null;

  const parts = s.split(",");
  if (parts.length > MAX_SORT_LEVELS) return null;

  const levels: SortLevel[] = [];
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
    if (!isSortField(field)) return null;
    if (!isSortDirection(direction)) return null;
    if (seen.has(field)) return null;
    seen.add(field);
    levels.push({ field, direction });
  }

  return levels;
}

/** Serialize a spec back into the URL-param form. */
export function serializeSortSpec(levels: readonly SortLevel[]): string {
  return levels.map((l) => `${l.field}:${l.direction}`).join(",");
}

/** Deep equality for two specs (order matters). */
export function sortSpecsEqual(
  a: readonly SortLevel[] | null | undefined,
  b: readonly SortLevel[] | null | undefined,
): boolean {
  if (!a || !b) return !a && !b;
  if (a.length !== b.length) return false;
  return a.every(
    (l, i) => l.field === b[i].field && l.direction === b[i].direction,
  );
}
