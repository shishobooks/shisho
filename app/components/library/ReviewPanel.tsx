import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useReviewCriteria } from "@/hooks/queries/review";
import { cn } from "@/libraries/utils";
import {
  FileRoleMain,
  FileTypeM4B,
  ReviewOverrideReviewed,
  ReviewOverrideUnreviewed,
  type Book,
  type File,
  type ReviewOverride,
} from "@/types";
import { formatDate } from "@/utils/format";

interface ReviewPanelProps {
  book: Book;
  files: File[];
  onChange: (override: ReviewOverride) => void;
  isPending?: boolean;
}

/**
 * Determine which fields are missing for a given file + book pair, given the
 * active review criteria.
 *
 * Book-level fields: authors, description, genres, tags, series, subtitle
 * File-level fields: cover, publisher, imprint, identifiers, release_date, language, url
 * Audio-only fields: narrators, chapters, abridged
 */
function getMissingFields(
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
      case "imprint":
        if (!file.imprint) missing.push(field);
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
 * Build the "Missing: ..." hint string aggregated across all main files that
 * need review.
 *
 * - If a field is missing on all files → list once without qualifier
 * - If different files miss different fields → qualify with file type
 *   e.g. "cover (EPUB), narrators (M4B)"
 */
function buildMissingHint(
  needsReviewFiles: File[],
  book: Book,
  bookFields: string[],
  audioFields: string[],
): string | null {
  if (needsReviewFiles.length === 0) return null;

  // Collect missing fields per file
  const missingPerFile: Map<number, string[]> = new Map();
  for (const file of needsReviewFiles) {
    const missing = getMissingFields(file, book, bookFields, audioFields);
    if (missing.length > 0) {
      missingPerFile.set(file.id, missing);
    }
  }

  if (missingPerFile.size === 0) return null;

  // Gather all unique fields across all files
  const allFields = new Set<string>();
  for (const fields of missingPerFile.values()) {
    for (const f of fields) allFields.add(f);
  }

  // For each unique field, check if it's missing on ALL files
  const fieldCount = needsReviewFiles.length;
  const parts: string[] = [];

  for (const field of allFields) {
    const filesWithField = needsReviewFiles.filter((f) =>
      missingPerFile.get(f.id)?.includes(field),
    );

    if (filesWithField.length === fieldCount) {
      // Missing on all files — list without qualifier
      parts.push(field);
    } else {
      // Missing on some files — qualify with file type (uppercase)
      for (const file of filesWithField) {
        parts.push(`${field} (${file.file_type.toUpperCase()})`);
      }
    }
  }

  if (parts.length === 0) return null;
  return `Missing: ${parts.join(", ")}`;
}

export function ReviewPanel({
  book,
  files,
  onChange,
  isPending = false,
}: ReviewPanelProps) {
  const { data: criteria } = useReviewCriteria();

  // Only consider main files
  const mainFiles = files.filter((f) => f.file_role === FileRoleMain);

  if (mainFiles.length === 0) return null;

  // Aggregate reviewed state: true iff ALL main files are reviewed
  const allReviewed =
    mainFiles.length > 0 && mainFiles.every((f) => f.reviewed === true);

  // Determine override indicator
  const allOverrideReviewed = mainFiles.every(
    (f) => f.review_override === ReviewOverrideReviewed,
  );
  const allOverrideUnreviewed = mainFiles.every(
    (f) => f.review_override === ReviewOverrideUnreviewed,
  );
  const hasAnyOverride = mainFiles.some((f) => f.review_override != null);
  const allNoOverride = mainFiles.every((f) => f.review_override == null);

  // Find most recent override date
  const sortedDates = mainFiles
    .filter((f) => f.review_overridden_at != null)
    .map((f) => f.review_overridden_at!)
    .sort();
  const mostRecentDate = sortedDates[sortedDates.length - 1];

  let indicatorText: string;
  let indicatorVariant: "secondary" | "success" | "outline" = "secondary";

  if (allNoOverride) {
    indicatorText = "Auto";
  } else if (allOverrideReviewed && mostRecentDate) {
    indicatorText = `Manually marked reviewed on ${formatDate(mostRecentDate)}`;
    indicatorVariant = "success";
  } else if (allOverrideUnreviewed && mostRecentDate) {
    indicatorText = `Manually marked needs review on ${formatDate(mostRecentDate)}`;
  } else if (hasAnyOverride) {
    indicatorText = "Manually set on multiple files";
  } else {
    indicatorText = "Auto";
  }

  // Build missing-fields hint (only when book needs review)
  const needsReview = !allReviewed;
  let missingHint: string | null = null;

  if (needsReview && criteria) {
    const needsReviewFiles = mainFiles.filter((f) => f.reviewed === false);
    missingHint = buildMissingHint(
      needsReviewFiles,
      book,
      criteria.book_fields,
      criteria.audio_fields,
    );
  }

  const handleToggle = (checked: boolean) => {
    onChange(checked ? ReviewOverrideReviewed : ReviewOverrideUnreviewed);
  };

  return (
    <div className="border rounded-md p-4 space-y-2 bg-muted/30">
      <div className="flex items-center gap-3">
        <Switch
          checked={allReviewed}
          className="cursor-pointer"
          disabled={isPending}
          id="review-toggle"
          onCheckedChange={handleToggle}
        />
        <Label
          className={cn(
            "cursor-pointer font-medium",
            isPending && "opacity-50",
          )}
          htmlFor="review-toggle"
        >
          Reviewed
        </Label>
        <Badge className="ml-1 text-xs" variant={indicatorVariant}>
          {indicatorText}
        </Badge>
      </div>

      {missingHint && (
        <p className="text-xs text-muted-foreground pl-0">{missingHint}</p>
      )}
    </div>
  );
}
