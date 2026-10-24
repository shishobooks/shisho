import { AlertTriangle } from "lucide-react";
import type { ReactNode } from "react";

import { fileHeaderFacts } from "@/components/files/fileFacts";
import FileCoverThumbnail from "@/components/library/FileCoverThumbnail";
import LabelGrid, { type LabelGridRow } from "@/components/library/LabelGrid";
import TextLink from "@/components/library/TextLink";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/libraries/utils";
import { FileRoleSupplement, FileTypeM4B, type Book, type File } from "@/types";
import { fileLabel, fileTypeLabel } from "@/utils/format";

// A comic can credit one person as writer and artist; name them once.
const uniqueAuthors = (book: Book) =>
  (book.authors ?? []).filter(
    (author, index, all) =>
      all.findIndex((a) => a.person_id === author.person_id) === index,
  );

interface FileDetailHeaderProps {
  book: Book;
  file: File;
  /** Drawn over the cover's bottom-left corner, as on the book page. */
  coverBadge?: ReactNode;
  actions?: ReactNode;
  /** The edition and identifier facts, as label rows. */
  rows: LabelGridRow[];
  /** Beside the header from 1280px; the caller places it otherwise. */
  sidePanel?: ReactNode;
  linkPeople: boolean;
}

/**
 * The top of the file page, shaped like the book page's header: the file's
 * cover (or first page, or type tile), its name, the way back to its book,
 * its narrators and facts, the actions, and what edition it is.
 */
const FileDetailHeader = ({
  book,
  file,
  coverBadge,
  actions,
  rows,
  sidePanel,
  linkPeople,
}: FileDetailHeaderProps) => {
  const supplement = file.file_role === FileRoleSupplement;
  const audio = file.file_type === FileTypeM4B;
  const bookLink = (
    <TextLink to={`/libraries/${book.library_id}/books/${book.id}`}>
      {book.title}
    </TextLink>
  );
  const authors = uniqueAuthors(book);
  const personLink = (personId: number) =>
    linkPeople ? `/libraries/${book.library_id}/people/${personId}` : null;

  return (
    <div className="flex flex-col gap-6 md:flex-row">
      <div
        className={cn(
          "relative mx-auto shrink-0 self-start md:mx-0",
          // A supplement is the lesser file, so its picture is smaller.
          supplement
            ? "w-32 md:w-[150px]"
            : audio
              ? "w-48 md:w-[220px]"
              : "w-40 md:w-[200px] lg:w-[220px]",
        )}
      >
        <FileCoverThumbnail
          className="w-full rounded-md"
          file={file}
          interactive={false}
        />
        {coverBadge && (
          <div className="absolute bottom-2 left-2">{coverBadge}</div>
        )}
      </div>

      <div className="min-w-0 flex-1 space-y-4">
        <div>
          <h1 className="break-words text-3xl font-bold leading-tight">
            {fileLabel(file)}
          </h1>
          <p className="mt-1.5 break-words text-base">
            <span className="text-muted-foreground">
              {supplement ? "Supplement to " : "From "}
            </span>
            {bookLink}
            {!supplement && authors.length > 0 && (
              <>
                <span className="text-muted-foreground"> by </span>
                {authors.map((author, index) => (
                  <span key={author.id}>
                    <TextLink
                      to={author.person ? personLink(author.person_id) : null}
                    >
                      {author.person?.name ?? "Unknown"}
                    </TextLink>
                    {index < authors.length - 1 && ", "}
                  </span>
                ))}
              </>
            )}
          </p>
          {file.narrators && file.narrators.length > 0 && (
            <p className="mt-0.5 break-words text-base">
              <span className="text-muted-foreground">Narrated by </span>
              {file.narrators.map((narrator, index) => (
                <span key={narrator.id}>
                  <TextLink
                    to={narrator.person ? personLink(narrator.person_id) : null}
                  >
                    {narrator.person?.name ?? "Unknown"}
                  </TextLink>
                  {index < file.narrators!.length - 1 && ", "}
                </span>
              ))}
            </p>
          )}
          <p className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-muted-foreground">
            <Badge className="px-1.5 py-0 text-[10px]" variant="outline">
              {fileTypeLabel(file.file_type)}
            </Badge>
            {fileHeaderFacts(file).join(" · ")}
          </p>
        </div>
        {actions}
        {file.scan_error && (
          <Alert variant="destructive">
            <AlertTriangle className="h-4 w-4" />
            <AlertTitle>This file could not be read</AlertTitle>
            <AlertDescription>
              <p>
                The last scan failed to parse the file, so its metadata has not
                been refreshed. Replace or repair the file and rescan it.
              </p>
              <p className="mt-2 break-all font-mono text-xs">
                {file.scan_error}
              </p>
            </AlertDescription>
          </Alert>
        )}
        <LabelGrid rows={rows} />
      </div>

      {sidePanel && (
        <aside className="hidden w-80 shrink-0 xl:block">{sidePanel}</aside>
      )}
    </div>
  );
};

export default FileDetailHeader;
