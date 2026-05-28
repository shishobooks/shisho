import type { File } from "@/types";

const isMainFile = (f: File): boolean => f.file_role !== "supplement";

const isBookFile = (f: File): boolean =>
  f.file_type === "epub" || f.file_type === "cbz" || f.file_type === "pdf";

const isAudiobookFile = (f: File): boolean => f.file_type === "m4b";

const hasCover = (f: File): boolean => Boolean(f.cover_image_filename);

/** Pick the preferred file from a bucket, falling back to the first entry. */
const pickFirst = (bucket: File[]): File =>
  bucket.find((f) => f.is_preferred_cover) ?? bucket[0];

/**
 * Picks which file's cover should represent a book based on the library's
 * preferred cover_aspect_ratio setting. Mirrors the backend's
 * pkg/covers.SelectFile logic. Supplements are excluded — they don't
 * represent the book. Within each bucket (ebook / audiobook), a file with
 * is_preferred_cover takes priority.
 */
export const selectCoverFile = (
  files: File[] | undefined,
  coverAspectRatio: string,
): File | null => {
  if (!files) return null;

  const candidates = files.filter(isMainFile);
  const bookFiles = candidates.filter((f) => isBookFile(f) && hasCover(f));
  const audiobookFiles = candidates.filter(
    (f) => isAudiobookFile(f) && hasCover(f),
  );

  switch (coverAspectRatio) {
    case "audiobook":
    case "audiobook_fallback_book":
      if (audiobookFiles.length > 0) return pickFirst(audiobookFiles);
      if (bookFiles.length > 0) return pickFirst(bookFiles);
      break;
    default:
      if (bookFiles.length > 0) return pickFirst(bookFiles);
      if (audiobookFiles.length > 0) return pickFirst(audiobookFiles);
  }
  return null;
};

/**
 * Determines which file type would provide the cover based on library
 * preference. Used for placeholder variant selection and aspect-ratio frame
 * sizing when no cover image is available. Mirrors selectCoverFile's
 * priority logic but doesn't require cover_image_filename. Supplements are
 * excluded.
 */
export const getCoverFileType = (
  files: File[] | undefined,
  coverAspectRatio: string,
): "book" | "audiobook" => {
  if (!files || files.length === 0) return "book";

  const candidates = files.filter(isMainFile);
  const hasBookFiles = candidates.some(isBookFile);
  const hasAudiobookFiles = candidates.some(isAudiobookFile);

  switch (coverAspectRatio) {
    case "audiobook":
    case "audiobook_fallback_book":
      if (hasAudiobookFiles) return "audiobook";
      if (hasBookFiles) return "book";
      break;
    default:
      if (hasBookFiles) return "book";
      if (hasAudiobookFiles) return "audiobook";
  }
  return "book";
};
