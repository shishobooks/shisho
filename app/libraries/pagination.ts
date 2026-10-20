/**
 * Parse a 1-based page number from a `?page=` URL search param.
 *
 * Returns 1 for missing, non-numeric, zero, or negative values so that
 * garbage URLs never produce a NaN page (which would propagate into a
 * NaN offset in list requests).
 */
export function parsePageParam(value: string | null): number {
  const parsed = parseInt(value ?? "1", 10);
  return Number.isNaN(parsed) || parsed < 1 ? 1 : parsed;
}

/**
 * The offset of the page holding the item at `start`. Pages are aligned to
 * the page size, so the item a URL names stays on screen when the page size
 * changes with the window or the cover size.
 */
export function startOffset(start: number, pageSize: number): number {
  if (!Number.isFinite(start) || start <= 0) return 0;
  return Math.floor(start / pageSize) * pageSize;
}

/** Reads the `?start=` item index, 0 when missing or invalid. */
export function parseStartParam(value: string | null): number {
  const parsed = parseInt(value ?? "0", 10);
  return Number.isNaN(parsed) || parsed < 0 ? 0 : parsed;
}

/**
 * Returns a paged list to its first page, as a new filter, sort, or search
 * does. Clears the item index and an old `?page=` link's page number, which
 * a list reads when no index is set.
 */
export function resetPaging(params: URLSearchParams): void {
  params.delete("start");
  params.delete("page");
}
