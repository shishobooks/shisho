import PageReader from "@/components/pages/PageReader";
import { useFilePageUrl } from "@/hooks/useFilePageUrl";
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
  const filePageUrl = useFilePageUrl();
  return (
    <PageReader
      bookId={file.book_id}
      fileId={file.id}
      getPageUrl={(page) => filePageUrl(file, page)}
      libraryId={libraryId}
      title={bookTitle}
      totalPages={file.page_count || 0}
    />
  );
}
