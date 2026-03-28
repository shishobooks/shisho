import PageReader from "@/components/pages/PageReader";
import type { File } from "@/types";

interface PDFReaderProps {
  file: File;
  libraryId: string;
  bookTitle?: string;
}

export default function PDFReader({
  file,
  libraryId,
  bookTitle,
}: PDFReaderProps) {
  return (
    <PageReader
      bookId={file.book_id}
      fileId={file.id}
      getPageUrl={(page) => `/api/books/files/${file.id}/page/${page}`}
      libraryId={libraryId}
      title={bookTitle}
      totalPages={file.page_count || 0}
    />
  );
}
