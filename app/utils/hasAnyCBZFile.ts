import { FileTypeCBZ, type Book } from "@/types";

/**
 * Returns true if the book has at least one file with file_type "cbz".
 * Used to determine whether series numbers should use CBZ formatting
 * (e.g., "Vol. 5", "Ch. 42") instead of bare numbers.
 */
export function hasAnyCBZFile(book: Book): boolean {
  return book.files?.some((f) => f.file_type === FileTypeCBZ) ?? false;
}
