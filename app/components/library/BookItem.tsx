import { uniqBy } from "lodash";
import { Link } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/libraries/utils";
import {
  AuthorRolePenciller,
  AuthorRoleWriter,
  FileTypeCBZ,
  type Book,
  type File,
} from "@/types";

interface BookItemProps {
  book: Book;
  libraryId: string;
  seriesId?: number;
  coverAspectRatio?: string;
}

// Selects the file that would be used for the cover based on cover_aspect_ratio setting
// This mirrors the backend's selectCoverFile logic
const selectCoverFile = (
  files: File[] | undefined,
  coverAspectRatio: string,
): File | null => {
  if (!files) return null;

  const bookFiles = files.filter(
    (f) =>
      (f.file_type === "epub" || f.file_type === "cbz") && f.cover_image_path,
  );
  const audiobookFiles = files.filter(
    (f) => f.file_type === "m4b" && f.cover_image_path,
  );

  switch (coverAspectRatio) {
    case "audiobook":
    case "audiobook_fallback_book":
      if (audiobookFiles.length > 0) return audiobookFiles[0];
      if (bookFiles.length > 0) return bookFiles[0];
      break;
    default: // "book", "book_fallback_audiobook"
      if (bookFiles.length > 0) return bookFiles[0];
      if (audiobookFiles.length > 0) return audiobookFiles[0];
  }
  return null;
};

const getAspectRatioClass = (
  coverAspectRatio: string,
  files?: File[],
): string => {
  // For non-fallback modes, just use the specified aspect ratio
  if (coverAspectRatio === "audiobook") return "aspect-square";
  if (coverAspectRatio === "book") return "aspect-[2/3]";

  // For fallback modes, determine which file's cover is being displayed
  const coverFile = selectCoverFile(files, coverAspectRatio);
  const isAudiobookCover = coverFile?.file_type === "m4b";

  switch (coverAspectRatio) {
    case "book_fallback_audiobook":
      return isAudiobookCover ? "aspect-square" : "aspect-[2/3]";
    case "audiobook_fallback_book":
      return isAudiobookCover ? "aspect-square" : "aspect-[2/3]";
    default:
      return "aspect-[2/3]";
  }
};

const BookItem = ({
  book,
  libraryId,
  seriesId,
  coverAspectRatio = "book",
}: BookItemProps) => {
  // Find the series number for the specific series context (if provided)
  const seriesNumber = seriesId
    ? book.book_series?.find((bs) => bs.series_id === seriesId)?.series_number
    : undefined;

  const aspectClass = getAspectRatioClass(coverAspectRatio, book.files);

  return (
    <div className="w-32" key={book.id}>
      <Link
        className="group cursor-pointer"
        to={`/libraries/${libraryId}/books/${book.id}`}
      >
        <img
          alt={`${book.title} Cover`}
          className={cn(
            "w-full object-cover rounded-sm border-neutral-300 dark:border-neutral-600 border-1",
            aspectClass,
          )}
          onError={(e) => {
            (e.target as HTMLImageElement).style.display = "none";
            (e.target as HTMLImageElement).nextElementSibling!.textContent =
              "no cover";
          }}
          src={`/api/books/${book.id}/cover`}
        />
        <div className="mt-2 group-hover:underline font-bold line-clamp-2 w-32">
          {book.title}
        </div>
      </Link>
      {book.authors &&
        book.authors.length > 0 &&
        (() => {
          const hasCBZFiles = book.files?.some(
            (f) => f.file_type === FileTypeCBZ,
          );

          // For CBZ files, only show Writer and Penciller roles, deduplicated by name
          const displayAuthors = hasCBZFiles
            ? book.authors.filter(
                (a) =>
                  a.role === AuthorRoleWriter ||
                  a.role === AuthorRolePenciller ||
                  !a.role,
              )
            : book.authors;

          // Get unique author names
          const uniqueNames = [
            ...new Set(displayAuthors.map((a) => a.person?.name ?? "Unknown")),
          ];

          if (uniqueNames.length === 0) return null;

          return (
            <div className="mt-1 text-sm line-clamp-2 text-neutral-500 dark:text-neutral-500">
              {uniqueNames.join(", ")}
            </div>
          );
        })()}
      {book.files && (
        <div className="mt-2 flex gap-2 text-sm">
          {uniqBy(book.files, "file_type").map((f) => (
            <Badge className="uppercase" key={f.id} variant="subtle">
              {f.file_type}
            </Badge>
          ))}
        </div>
      )}
      {seriesNumber && (
        <div className="mt-1">
          <Badge className="text-xs" variant="outline">
            #{seriesNumber}
          </Badge>
        </div>
      )}
    </div>
  );
};

export default BookItem;
