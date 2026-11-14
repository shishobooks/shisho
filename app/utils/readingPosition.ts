import type { ReadingPosition } from "@/types";

// Where each web reader opens a saved Reading Position. A position can lack
// the reader's own locator: one from a copy a user linked is a percentage
// only (ADR 0017), and one another reader saved may not translate. Rather
// than the start, the reader then opens at the position's percentage.

type Saved = Pick<ReadingPosition, "cfi" | "page" | "seconds" | "percentage">;

const fractionOf = (saved: Saved): number | null =>
  saved.percentage > 0 ? Math.min(saved.percentage, 1) : null;

/**
 * Where the reflowable reader opens: the saved CFI, or how far through the
 * book as foliate's fraction, or undefined for the start.
 */
export const reflowableOpenTarget = (
  saved: Saved | null,
): string | { fraction: number } | undefined => {
  if (!saved) return undefined;
  if (saved.cfi) return saved.cfi;
  const fraction = fractionOf(saved);
  return fraction === null ? undefined : { fraction };
};

/**
 * The 0-based page a page-based reader opens at, or null for its default.
 * The page for a percentage is the inverse of how the reader saves one,
 * page / (totalPages - 1).
 */
export const savedPage = (
  saved: Saved | null,
  totalPages: number,
): number | null => {
  if (!saved) return null;
  if (saved.page != null) return saved.page;
  const fraction = fractionOf(saved);
  if (fraction === null || totalPages < 1) return null;
  return Math.round(fraction * (totalPages - 1));
};

/**
 * The second the audiobook player opens at, 0 for the start. With no seconds
 * saved, it's the percentage of duration, once the duration is known.
 */
export const savedSeconds = (saved: Saved | null, duration: number): number => {
  if (!saved) return 0;
  if (saved.seconds != null) return saved.seconds;
  const fraction = fractionOf(saved);
  if (fraction === null || !Number.isFinite(duration) || duration <= 0) {
    return 0;
  }
  return fraction * duration;
};
