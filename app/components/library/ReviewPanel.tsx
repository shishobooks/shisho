import { useId } from "react";

import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { useReviewCriteria } from "@/hooks/queries/review";
import { cn } from "@/libraries/utils";
import {
  FileRoleMain,
  ReviewOverrideReviewed,
  ReviewOverrideUnreviewed,
  type Book,
  type File,
  type ReviewOverride,
} from "@/types";
import { fileLabel, fileTypeLabel, formatDate } from "@/utils/format";
import { missingReviewFields } from "@/utils/review";

interface ReviewPanelProps {
  book: Book;
  files: File[];
  onChange: (override: ReviewOverride) => void;
  isPending?: boolean;
  /**
   * When provided, the checkbox reflects this
   * value instead of the aggregate `reviewed` state computed from `files`.
   * Use for controlled / draft mode (e.g. edit dialogs that defer to Save).
   * The committed-state-driven status text and missing-fields hint are
   * unaffected.
   */
  toggleValue?: boolean;
}

/**
 * Build the "Missing: ..." hint string aggregated across all main files that
 * need review.
 *
 * - If a field is missing on all files → list once without qualifier
 * - If different files miss different fields → qualify with the files, once
 *   per field: by type when no other main file shares it, else by name,
 *   e.g. "cover (EPUB), narrators (Read by Fry, Read by Dale)"
 */
function buildMissingHint(
  needsReviewFiles: File[],
  mainFiles: File[],
  book: Book,
  bookFields: string[],
  audioFields: string[],
): string | null {
  if (needsReviewFiles.length === 0) return null;

  // Collect missing fields per file
  const missingPerFile: Map<number, string[]> = new Map();
  for (const file of needsReviewFiles) {
    const missing = missingReviewFields(file, book, bookFields, audioFields);
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
      // Missing on some files: say which. Two files of one type would both
      // read "(M4B)", so a shared type names the file instead.
      const which = filesWithField.map((file) =>
        mainFiles.filter((f) => f.file_type === file.file_type).length > 1
          ? fileLabel(file)
          : fileTypeLabel(file.file_type),
      );
      parts.push(`${field} (${[...new Set(which)].join(", ")})`);
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
  toggleValue,
}: ReviewPanelProps) {
  const { data: criteria } = useReviewCriteria();
  // Book detail and an edit dialog over it can both render a panel.
  const toggleId = useId();

  // Only consider main files
  const mainFiles = files.filter((f) => f.file_role === FileRoleMain);

  if (mainFiles.length === 0) return null;

  // Committed aggregate reviewed state: true iff ALL main files are reviewed.
  // Used for the missing-fields hint (which always reflects saved state).
  const committedAllReviewed =
    mainFiles.length > 0 && mainFiles.every((f) => f.reviewed === true);

  // The toggle's checked state and the icon's color follow `toggleValue`
  // when caller is in controlled mode; otherwise default to committed state.
  const allReviewed = toggleValue ?? committedAllReviewed;

  // Determine override indicator
  const allOverrideReviewed = mainFiles.every(
    (f) => f.review_override === ReviewOverrideReviewed,
  );
  const allOverrideUnreviewed = mainFiles.every(
    (f) => f.review_override === ReviewOverrideUnreviewed,
  );
  const allNoOverride = mainFiles.every((f) => f.review_override == null);

  // Find most recent override date
  const sortedDates = mainFiles
    .filter((f) => f.review_overridden_at != null)
    .map((f) => f.review_overridden_at!)
    .sort();
  const mostRecentDate = sortedDates[sortedDates.length - 1];

  const isAuto = allNoOverride;

  // How the state was set, said under the checkbox rather than in a tooltip,
  // so it reads the same on a phone.
  let statusText: string;
  if (isAuto) {
    statusText =
      "Set automatically from the review criteria. Check or uncheck to set it by hand.";
  } else if (allOverrideReviewed && mostRecentDate) {
    statusText = `Marked reviewed by hand on ${formatDate(mostRecentDate)}.`;
  } else if (allOverrideUnreviewed && mostRecentDate) {
    statusText = `Marked as needing review by hand on ${formatDate(mostRecentDate)}.`;
  } else {
    statusText = "Set by hand on some files.";
  }

  // Build missing-fields hint (only when book needs review).
  // Uses committed state — the hint reflects what's actually missing on disk,
  // not the draft toggle state in controlled mode.
  const needsReview = !committedAllReviewed;
  let missingHint: string | null = null;

  if (needsReview && criteria) {
    const needsReviewFiles = mainFiles.filter((f) => f.reviewed === false);
    missingHint = buildMissingHint(
      needsReviewFiles,
      mainFiles,
      book,
      criteria.book_fields,
      criteria.audio_fields,
    );
  }

  const handleToggle = (checked: boolean) => {
    onChange(checked ? ReviewOverrideReviewed : ReviewOverrideUnreviewed);
  };

  // Laid out like the dialogs' other checkboxes (Abridged, Preferred cover):
  // a field label, the checkbox with its label, and a line of help.
  return (
    <div
      aria-labelledby={`${toggleId}-heading`}
      className="space-y-2"
      role="group"
    >
      <Label id={`${toggleId}-heading`}>Review</Label>
      <div className="flex items-center gap-2">
        <Checkbox
          aria-describedby={`${toggleId}-status`}
          checked={allReviewed}
          disabled={isPending}
          id={toggleId}
          onCheckedChange={(checked) => handleToggle(checked === true)}
        />
        <Label
          className={cn(
            "cursor-pointer font-normal text-muted-foreground",
            isPending && "opacity-50",
          )}
          htmlFor={toggleId}
        >
          Reviewed
        </Label>
      </div>
      <p className="text-xs text-muted-foreground" id={`${toggleId}-status`}>
        {statusText}
        {missingHint && ` ${missingHint}.`}
      </p>
    </div>
  );
}
