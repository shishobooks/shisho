import { useParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import CBZReader from "@/components/pages/CBZReader";
import EPUBReader from "@/components/pages/EPUBReader";
import PDFReader from "@/components/pages/PDFReader";
import { useBook } from "@/hooks/queries/books";
import { FileTypeCBZ, FileTypeEPUB, FileTypePDF } from "@/types";

export default function FileReader() {
  const { libraryId, bookId, fileId } = useParams<{
    libraryId: string;
    bookId: string;
    fileId: string;
  }>();

  const { data: book, isLoading } = useBook(bookId);
  const file = book?.files?.find((f) => f.id === Number(fileId));

  if (isLoading || !file) {
    return (
      <div className="fixed inset-0 bg-background flex items-center justify-center">
        <LoadingSpinner />
      </div>
    );
  }

  switch (file.file_type) {
    case FileTypeCBZ:
      return (
        <CBZReader bookTitle={book?.title} file={file} libraryId={libraryId!} />
      );
    case FileTypePDF:
      return (
        <PDFReader bookTitle={book?.title} file={file} libraryId={libraryId!} />
      );
    case FileTypeEPUB:
      return <EPUBReader bookTitle={book?.title} file={file} />;
    default:
      return (
        <div className="fixed inset-0 bg-background flex items-center justify-center">
          <p className="text-muted-foreground">
            Reading is not supported for this file type.
          </p>
        </div>
      );
  }
}
