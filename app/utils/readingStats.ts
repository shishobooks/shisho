import type {
  ReadingStatsCount,
  ReadingStatsKind,
  ReadingStatsKindCount,
  ReadingStatsRatingCount,
} from "@/types";
import { formatCount } from "@/utils/format";

// The rows of the Reading Stats breakdowns, from the API's counts.

/** One row of a breakdown: a name and how many finished books it counts. */
export interface CountRow {
  /** What selecting the row filters the finished books by. */
  value: string;
  label: string;
  books: number;
}

export const countRows = (counts: ReadingStatsCount[]): CountRow[] =>
  counts.map((count) => ({
    value: count.name,
    label: count.name,
    books: count.books,
  }));

export const KIND_LABELS: Record<ReadingStatsKind, string> = {
  ebook: "Ebooks",
  comic: "Comics",
  audiobook: "Audiobooks",
  unknown: "Unknown",
};

export const kindRows = (kinds: ReadingStatsKindCount[]): CountRow[] =>
  kinds.map((kind) => ({
    value: kind.kind,
    label: KIND_LABELS[kind.kind],
    books: kind.books,
  }));

export const ratingLabel = (stars: number) => formatCount(stars, "star");

// Highest first, as a reader scans a rating breakdown.
export const ratingRows = (ratings: ReadingStatsRatingCount[]): CountRow[] =>
  [...ratings]
    .sort((a, b) => b.rating - a.rating)
    .map((rating) => ({
      value: String(rating.rating),
      label: ratingLabel(rating.rating),
      books: rating.books,
    }));
