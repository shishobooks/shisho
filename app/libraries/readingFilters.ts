/**
 * The Reading Status and rating filters on library and List pages, kept in
 * the URL as `reading_status` (comma-separated), `rating_min`, and
 * `rating_max`, and sent to the book listings as the same names.
 */
import {
  ReadingStatusFilterNone,
  type ReadingStatus,
  type ReadingStatusFilter,
} from "@/types";
import { formatCount, formatRating } from "@/utils/format";
import { readingStatusLabel } from "@/utils/readingStatus";

export interface ReadingFilters {
  statuses: ReadingStatusFilter[];
  ratingMin?: number;
  ratingMax?: number;
}

const STATUSES: readonly ReadingStatus[] = [
  "currently_reading",
  "paused",
  "to_read",
  "read",
  "did_not_finish",
];

/** The status filter's options, in the order the controls show them. */
export const READING_STATUS_FILTER_OPTIONS: readonly {
  value: ReadingStatusFilter;
  label: string;
}[] = [
  ...STATUSES.map((value) => ({ value, label: readingStatusLabel(value) })),
  { value: ReadingStatusFilterNone, label: "No status" },
];

export const readingStatusFilterLabel = (value: ReadingStatusFilter): string =>
  READING_STATUS_FILTER_OPTIONS.find((o) => o.value === value)?.label ?? value;

const isStatus = (value: string): value is ReadingStatusFilter =>
  READING_STATUS_FILTER_OPTIONS.some((o) => o.value === value);

// A rating the server accepts as a range end, or undefined.
const parseRating = (value: string | null): number | undefined => {
  if (value === null) return undefined;
  const stars = Number(value);
  return Number.isFinite(stars) && stars >= 0.25 && stars <= 5
    ? stars
    : undefined;
};

/**
 * Reads the filters from the URL, dropping values the server would refuse,
 * so an old or edited link still lists books.
 */
export function parseReadingFilters(params: URLSearchParams): ReadingFilters {
  const statuses = (params.get("reading_status") ?? "")
    .split(",")
    .filter(isStatus);
  const filters: ReadingFilters = { statuses };
  const ratingMin = parseRating(params.get("rating_min"));
  let ratingMax = parseRating(params.get("rating_max"));
  if (
    ratingMin !== undefined &&
    ratingMax !== undefined &&
    ratingMax < ratingMin
  ) {
    ratingMax = undefined;
  }
  if (ratingMin !== undefined) filters.ratingMin = ratingMin;
  if (ratingMax !== undefined) filters.ratingMax = ratingMax;
  return filters;
}

/** Writes filters into params, deleting the ones that are empty. */
export function writeReadingFilters(
  params: URLSearchParams,
  filters: ReadingFilters,
): void {
  if (filters.statuses.length > 0) {
    params.set("reading_status", filters.statuses.join(","));
  } else {
    params.delete("reading_status");
  }
  for (const [key, value] of [
    ["rating_min", filters.ratingMin],
    ["rating_max", filters.ratingMax],
  ] as const) {
    if (value === undefined) params.delete(key);
    else params.set(key, String(value));
  }
}

export const hasReadingFilters = (filters: ReadingFilters): boolean =>
  filters.statuses.length > 0 ||
  filters.ratingMin !== undefined ||
  filters.ratingMax !== undefined;

/** The book listing query params for filters. */
export function readingFilterQuery(filters: ReadingFilters): {
  reading_status?: ReadingStatusFilter[];
  rating_min?: number;
  rating_max?: number;
} {
  return {
    ...(filters.statuses.length > 0 && { reading_status: filters.statuses }),
    ...(filters.ratingMin !== undefined && { rating_min: filters.ratingMin }),
    ...(filters.ratingMax !== undefined && { rating_max: filters.ratingMax }),
  };
}

/** Names a rating range: "4+ stars", "Up to 3 stars", "3 to 4 stars". */
export function ratingRangeLabel(
  min: number | undefined,
  max: number | undefined,
): string {
  if (min !== undefined && max !== undefined) {
    if (min === max) return formatCount(min, "star");
    return `${formatRating(min)} to ${formatRating(max)} stars`;
  }
  if (min !== undefined) return `${formatRating(min)}+ stars`;
  if (max !== undefined) return `Up to ${formatRating(max)} stars`;
  return "Any rating";
}
