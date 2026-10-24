import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useReviewCriteria } from "@/hooks/queries/review";
import { FileRoleMain, type Book, type File } from "@/types";
import { isBookNeedsReview } from "@/utils/book";
import { fileLabel } from "@/utils/format";
import {
  BOOK_REVIEW_FIELDS,
  missingReviewFields,
  reviewFieldLabel,
} from "@/utils/review";

interface BookReviewBadgeProps {
  book: Book;
  /**
   * Opens the editor that can fix what is missing. Absent for a role that
   * cannot edit, which gets the badge and its explanation but no action.
   */
  onEditBook?: () => void;
  onEditFile?: (file: File) => void;
}

/**
 * The gallery's "Needs review" badge, on the book page's cover where the
 * gallery puts it. Hovering says what the review criteria find missing, on
 * the book and on each file; a click opens the editor that can fix it: the
 * book's when a book field is missing, else the first file's that is.
 */
const BookReviewBadge = ({
  book,
  onEditBook,
  onEditFile,
}: BookReviewBadgeProps) => {
  const { data: criteria } = useReviewCriteria();
  if (!isBookNeedsReview(book)) return null;

  const unreviewed = book.files.filter(
    (f) => f.file_role === FileRoleMain && f.reviewed !== true,
  );
  const bookMissing = new Set<string>();
  const fileMissing: { file: File; fields: string[] }[] = [];
  for (const file of unreviewed) {
    const missing = missingReviewFields(
      file,
      book,
      criteria?.book_fields ?? [],
      criteria?.audio_fields ?? [],
    );
    for (const field of missing) {
      if (BOOK_REVIEW_FIELDS.has(field)) bookMissing.add(field);
    }
    const own = missing.filter((field) => !BOOK_REVIEW_FIELDS.has(field));
    if (own.length > 0) fileMissing.push({ file, fields: own });
  }

  // With the criteria loaded, nothing missing means someone marked the book
  // as needing review. Until they load, the badge says only what it is.
  const manual =
    criteria !== undefined &&
    bookMissing.size === 0 &&
    fileMissing.length === 0;
  const firstFile = fileMissing[0]?.file;
  const onClick =
    bookMissing.size > 0 || !firstFile
      ? onEditBook
      : onEditFile && (() => onEditFile(firstFile));

  // The gallery's badge (BookItem), so the two read as the same thing.
  const badge = (
    <Badge className="px-1.5 py-0 text-[10px]" variant="secondary">
      Needs review
    </Badge>
  );

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        {/* A button, so the keyboard can open the tooltip even for a role
            that cannot edit, where it does nothing else. */}
        <Button
          aria-label="Needs review"
          className="flex rounded-md"
          onClick={onClick}
          variant="unstyled"
        >
          {badge}
        </Button>
      </TooltipTrigger>
      <TooltipContent
        arrowClassName="bg-popover fill-popover"
        className="max-w-xs border bg-popover text-left text-popover-foreground shadow-md"
        side="bottom"
      >
        {criteria === undefined ? (
          <p>Needs review.</p>
        ) : manual ? (
          <p>Marked as needing review.</p>
        ) : (
          <div className="space-y-1">
            {bookMissing.size > 0 && (
              <p>
                <span className="font-semibold">Book:</span> missing{" "}
                {[...bookMissing].map(reviewFieldLabel).join(", ")}
              </p>
            )}
            {fileMissing.map(({ file, fields }) => (
              <p key={file.id}>
                <span className="font-semibold">{fileLabel(file)}:</span>{" "}
                missing {fields.map(reviewFieldLabel).join(", ")}
              </p>
            ))}
          </div>
        )}
        {onClick && (
          <p className="mt-1 text-muted-foreground">
            {manual || criteria === undefined
              ? "Click to open the editor."
              : "Click to fix."}
          </p>
        )}
      </TooltipContent>
    </Tooltip>
  );
};

export default BookReviewBadge;
