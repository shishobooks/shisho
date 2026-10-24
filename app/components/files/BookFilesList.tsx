import { useId } from "react";
import { Link } from "react-router-dom";

import FileCoverThumbnail from "@/components/library/FileCoverThumbnail";
import TruncatedText from "@/components/library/TruncatedText";
import { cn } from "@/libraries/utils";
import { FileRoleSupplement, FileTypeM4B, type Book, type File } from "@/types";
import { fileLabel, fileTypeLabel, formatCount } from "@/utils/format";

/**
 * Every file of the book, this one marked rather than linked: the way
 * across to the book's other editions and supplements.
 */
const BookFilesList = ({ book, file }: { book: Book; file: File }) => {
  // The page mounts the list twice, one copy for each width.
  const headingId = useId();
  const files = book.files ?? [];
  return (
    <section aria-labelledby={headingId}>
      <h2 className="mb-3 text-sm text-muted-foreground" id={headingId}>
        {formatCount(files.length, "file")} in this book
      </h2>
      <ul className="space-y-1">
        {files.map((f) => {
          const current = f.id === file.id;
          const row = (
            <span
              className={cn(
                "flex items-center gap-3 rounded-md px-1.5 py-1",
                current ? "bg-muted" : "hover:bg-muted/60",
              )}
            >
              <FileCoverThumbnail
                className={f.file_type === FileTypeM4B ? "w-10" : "w-8"}
                file={f}
                interactive={false}
              />
              <span className="min-w-0">
                <span className="block text-xs text-muted-foreground">
                  {[
                    fileTypeLabel(f.file_type),
                    f.file_role === FileRoleSupplement && "Supplement",
                    current && "This file",
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </span>
                <TruncatedText className="line-clamp-2 break-words text-sm leading-snug">
                  {fileLabel(f)}
                </TruncatedText>
              </span>
            </span>
          );
          return (
            <li key={f.id}>
              {current ? (
                <div aria-current="page">{row}</div>
              ) : (
                // Another file of the same book reads as the same page, so
                // the place on it holds.
                <Link
                  className="block"
                  preventScrollReset
                  to={`/libraries/${f.library_id}/books/${f.book_id}/files/${f.id}`}
                >
                  {row}
                </Link>
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
};

export default BookFilesList;
