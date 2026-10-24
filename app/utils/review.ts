import {
  FileTypeM4B,
  type Book,
  type File,
  type ReviewOverride,
} from "@/types";

/**
 * The review fields that belong to the book rather than to one of its
 * files: a book missing one needs the book editor, not a file editor.
 */
export const BOOK_REVIEW_FIELDS: ReadonlySet<string> = new Set([
  "authors",
  "description",
  "genres",
  "tags",
  "series",
  "subtitle",
]);

const REVIEW_FIELD_LABELS: Record<string, string> = {
  release_date: "release date",
  url: "URL",
};

/** How a review field is written in a sentence: "release date", "URL". */
export const reviewFieldLabel = (field: string): string =>
  REVIEW_FIELD_LABELS[field] ?? field;

/**
 * Determine which fields are missing for a given file + book pair, given the
 * active review criteria.
 *
 * Book-level fields: authors, description, genres, tags, series, subtitle
 * File-level fields: cover, publisher, identifiers, release_date, language, url
 * Audio-only fields: narrators, chapters, abridged
 */
export function missingReviewFields(
  file: File,
  book: Book,
  bookFields: string[],
  audioFields: string[],
): string[] {
  const isAudio = file.file_type === FileTypeM4B;
  const missing: string[] = [];

  for (const field of bookFields) {
    switch (field) {
      case "authors":
        if (!book.authors || book.authors.length === 0) missing.push(field);
        break;
      case "description":
        if (!book.description) missing.push(field);
        break;
      case "genres":
        if (!book.book_genres || book.book_genres.length === 0)
          missing.push(field);
        break;
      case "tags":
        if (!book.book_tags || book.book_tags.length === 0) missing.push(field);
        break;
      case "series":
        if (!book.book_series || book.book_series.length === 0)
          missing.push(field);
        break;
      case "subtitle":
        if (!book.subtitle) missing.push(field);
        break;
      case "cover":
        if (!file.cover_image_filename && !file.cover_mime_type)
          missing.push(field);
        break;
      case "publisher":
        if (!file.publisher) missing.push(field);
        break;
      case "identifiers":
        if (!file.identifiers || file.identifiers.length === 0)
          missing.push(field);
        break;
      case "release_date":
        if (!file.release_date) missing.push(field);
        break;
      case "language":
        if (!file.language) missing.push(field);
        break;
      case "url":
        if (!file.url) missing.push(field);
        break;
    }
  }

  if (isAudio) {
    for (const field of audioFields) {
      switch (field) {
        case "narrators":
          if (!file.narrators || file.narrators.length === 0)
            missing.push(field);
          break;
        case "chapters":
          if (!file.chapters || file.chapters.length === 0) missing.push(field);
          break;
        case "abridged":
          if (file.abridged == null) missing.push(field);
          break;
      }
    }
  }

  return missing;
}

/**
 * Whether saving an edit dialog's review draft changes anything. The draft is
 * `undefined` while untouched and `null` after Use automatic; `saved` holds
 * the override of each main file it applies to (`null` for none).
 */
export function reviewDraftChanged(
  draft: ReviewOverride | null | undefined,
  saved: Array<ReviewOverride | null>,
): boolean {
  return draft !== undefined && saved.some((override) => override !== draft);
}
