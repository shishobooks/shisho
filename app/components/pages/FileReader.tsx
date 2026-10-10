import { useParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import CBZReader from "@/components/pages/CBZReader";
import M4BReader from "@/components/pages/M4BReader";
import PDFReader from "@/components/pages/PDFReader";
import ReflowableReader from "@/components/pages/ReflowableReader";
import { useBook } from "@/hooks/queries/books";
import { isLoadFailure } from "@/libraries/api";
import {
  FileTypeAZW3,
  FileTypeCBZ,
  FileTypeEPUB,
  FileTypeM4B,
  FileTypeMOBI,
  FileTypePDF,
} from "@/types";

// Covers the page below the Demo Mode banner, since readers render outside
// the library layout.
const READER_OVERLAY =
  "fixed inset-x-0 bottom-0 top-[var(--demo-banner-height,0px)] bg-background flex items-center justify-center";

export default function FileReader() {
  const { libraryId, bookId, fileId } = useParams<{
    libraryId: string;
    bookId: string;
    fileId: string;
  }>();

  const bookQuery = useBook(bookId);
  const book = bookQuery.data;
  const file = book?.files?.find((f) => f.id === Number(fileId));

  // isPending, not isLoading, so a query paused while offline keeps the
  // spinner instead of claiming the file is missing.
  if (bookQuery.isPending) {
    return (
      <div className={READER_OVERLAY}>
        <LoadingSpinner />
      </div>
    );
  }

  if (isLoadFailure(bookQuery)) {
    return (
      <div className={READER_OVERLAY}>
        <QueryError
          className="mx-4 max-w-md flex-1"
          fallback="Failed to load book"
          query={bookQuery}
        />
      </div>
    );
  }

  if (!file) {
    return (
      <div className={READER_OVERLAY}>
        <div className="text-center">
          <h1 className="text-2xl font-semibold mb-4">File Not Found</h1>
          <p className="text-muted-foreground">
            The file you're looking for doesn't exist or may have been removed.
          </p>
        </div>
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
    case FileTypeAZW3:
    case FileTypeMOBI:
      return <ReflowableReader bookTitle={book?.title} file={file} />;
    case FileTypeM4B:
      return <M4BReader book={book} file={file} libraryId={libraryId!} />;
    default:
      return (
        <div className={READER_OVERLAY}>
          <p className="text-muted-foreground">
            Reading is not supported for this file type.
          </p>
        </div>
      );
  }
}
