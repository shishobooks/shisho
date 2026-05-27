import { FileRoleMain, type Book, type FileType } from "@/types";

/**
 * Returns the distinct file types present across main files in the selected books.
 * Supplements are excluded.
 */
export const getAvailableFileTypes = (
  books: Book[],
  selectedBookIds: number[],
): FileType[] => {
  const selectedIds = new Set(selectedBookIds);
  const types = new Set<FileType>();

  for (const book of books) {
    if (!selectedIds.has(book.id)) continue;
    for (const file of book.files ?? []) {
      if (file.file_role === FileRoleMain) {
        types.add(file.file_type);
      }
    }
  }

  return Array.from(types);
};

interface DownloadFileInfo {
  fileIds: number[];
  totalSize: number;
}

/**
 * Collects all main files matching the selected file types across the selected books.
 * Returns their IDs and the total size in bytes. Supplements are excluded.
 */
export const collectDownloadFiles = (
  books: Book[],
  selectedBookIds: number[],
  selectedFileTypes: string[],
): DownloadFileInfo => {
  const selectedIds = new Set(selectedBookIds);
  const typeSet = new Set(selectedFileTypes);
  const fileIds: number[] = [];
  let totalSize = 0;

  for (const book of books) {
    if (!selectedIds.has(book.id)) continue;
    for (const file of book.files ?? []) {
      if (file.file_role === FileRoleMain && typeSet.has(file.file_type)) {
        fileIds.push(file.id);
        totalSize += file.filesize_bytes ?? 0;
      }
    }
  }

  return { fileIds, totalSize };
};
