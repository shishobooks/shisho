import PageReader from "@/components/pages/PageReader";
import { useFilePageUrl } from "@/hooks/useFilePageUrl";
import type { File } from "@/types";
import { fileTitle } from "@/utils/format";

interface CBZReaderProps {
  file: File;
  libraryId: string;
  bookTitle?: string;
}

export default function CBZReader({
  file,
  libraryId,
  bookTitle,
}: CBZReaderProps) {
  const filePageUrl = useFilePageUrl();
  return (
    <PageReader
      bookId={file.book_id}
      fileId={file.id}
      getPageUrl={(page, size) => filePageUrl(file, page, size)}
      libraryId={libraryId}
      title={fileTitle(file, bookTitle)}
      totalPages={file.page_count || 0}
    />
  );
}
