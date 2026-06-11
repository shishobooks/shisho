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
