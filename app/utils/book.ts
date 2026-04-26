import { FileRoleMain, type Book } from "@/types";

/**
 * Returns true when a book has at least one main file that has not been reviewed.
 * A book with no main files does not need review.
 */
export const isBookNeedsReview = (book: Book): boolean => {
  const mains = (book.files ?? []).filter((f) => f.file_role === FileRoleMain);
  if (mains.length === 0) return false;
  return mains.some((f) => f.reviewed !== true);
};
