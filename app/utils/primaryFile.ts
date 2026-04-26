import type { Book } from "@/types";

export function getPrimaryFileType(book: Book): string | null {
  const primary =
    (book.primary_file_id != null
      ? book.files?.find((f) => f.id === book.primary_file_id)
      : null) ?? book.files?.[0];
  return primary?.file_type ?? null;
}
