import {
  BookOpen,
  Headphones,
  MoreHorizontal,
  MoveRight,
  Pencil,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import BookFilesList from "@/components/files/BookFilesList";
import type { ChaptersActionState } from "@/components/files/FileChaptersEditor";
import FileChaptersSection from "@/components/files/FileChaptersSection";
import FileDetailHeader from "@/components/files/FileDetailHeader";
import {
  fileDiskRows,
  fileEditionRows,
  fileIdentifierRows,
} from "@/components/files/fileFacts";
import FilePageGrid from "@/components/files/FilePageGrid";
import { BookEditDialog } from "@/components/library/BookEditDialog";
import BookReviewBadge from "@/components/library/BookReviewBadge";
import { DeleteConfirmationDialog } from "@/components/library/DeleteConfirmationDialog";
import DownloadErrorDialog from "@/components/library/DownloadErrorDialog";
import FileDownloadControl from "@/components/library/FileDownloadControl";
import { FileEditDialog } from "@/components/library/FileEditDialog";
import LabelGrid from "@/components/library/LabelGrid";
import LibraryBreadcrumbs from "@/components/library/LibraryBreadcrumbs";
import LibraryLayout from "@/components/library/LibraryLayout";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { MoveFilesDialog } from "@/components/library/MoveFilesDialog";
import QueryError from "@/components/library/QueryError";
import { RescanDialog } from "@/components/library/RescanDialog";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";
import { useBook, useDeleteFile, useResyncFile } from "@/hooks/queries/books";
import { useDeliveryRules } from "@/hooks/queries/delivery";
import { useUserLibrary } from "@/hooks/queries/libraries";
import { usePluginIdentifierTypes } from "@/hooks/queries/plugins";
import { useCan } from "@/hooks/useCan";
import { useFileDownload } from "@/hooks/useFileDownload";
import { useLeaveDeletedBook } from "@/hooks/useLeaveDeletedBook";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useUnsavedChanges } from "@/hooks/useUnsavedChanges";
import { isLoadFailure, toastRequestError } from "@/libraries/api";
import {
  FileRoleMain,
  FileTypeCBZ,
  FileTypePDF,
  type ResyncMode,
} from "@/types";
import { defaultDeliveredFileType } from "@/utils/delivery";
import { fileLabel } from "@/utils/format";
import { getReadingAction } from "@/utils/readingAction";

const NO_CHANGES: ChaptersActionState = {
  isSaving: false,
  canSave: false,
  hasChanges: false,
};

/**
 * A file's page, shaped like its book's: the header with the file's cover,
 * name, facts, and actions, the book's other files beside it, then the
 * pages of a PDF or CBZ, the chapters, and the file's on-disk facts.
 */
const FileDetail = () => {
  const { fileId, bookId, libraryId, tab } = useParams<{
    fileId: string;
    bookId: string;
    libraryId: string;
    tab?: string;
  }>();
  const navigate = useNavigate();
  const leaveDeletedBook = useLeaveDeletedBook();
  // File metadata, chapters, rescan, move, and delete all require Books
  // Write. People pages need People Read; without it names are plain text.
  const canWriteBooks = useCan("books:write");
  const linkPeople = useCan("people:read");

  const bookQuery = useBook(bookId);
  const libraryQuery = useUserLibrary(libraryId);
  const { data: pluginIdentifierTypes } = usePluginIdentifierTypes();
  // The main download button follows the user's default Delivery Rules.
  const deliveryRulesQuery = useDeliveryRules();
  const downloads = useFileDownload();
  const resyncFileMutation = useResyncFile();
  const deleteFileMutation = useDeleteFile();

  const [editFileOpen, setEditFileOpen] = useState(false);
  const [editBookOpen, setEditBookOpen] = useState(false);
  const [rescanOpen, setRescanOpen] = useState(false);
  const [moveOpen, setMoveOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [chaptersState, setChaptersState] =
    useState<ChaptersActionState>(NO_CHANGES);
  // The page had Details and Chapters tabs. An old link to either lands
  // here under the plain URL, scrolled to the Chapters section for
  // `/chapters`.
  const [scrollToChapters, setScrollToChapters] = useState(tab === "chapters");

  // Leaving with unsaved chapter edits asks first.
  const { showBlockerDialog, proceedNavigation, cancelNavigation } =
    useUnsavedChanges(chaptersState.hasChanges);

  const file = bookQuery.data?.files?.find(
    (f) => f.id === parseInt(fileId || "0"),
  );
  usePageTitle(file ? fileLabel(file) : undefined);

  useEffect(() => {
    if (!tab) return;
    navigate(`/libraries/${libraryId}/books/${bookId}/files/${fileId}`, {
      replace: true,
      preventScrollReset: true,
    });
  }, [tab, libraryId, bookId, fileId, navigate]);

  if (bookQuery.isLoading) {
    return (
      <LibraryLayout>
        <LoadingSpinner />
      </LibraryLayout>
    );
  }

  if (isLoadFailure(bookQuery)) {
    return (
      <LibraryLayout>
        <QueryError fallback="Failed to load book" query={bookQuery} />
      </LibraryLayout>
    );
  }

  if (!bookQuery.data || !file) {
    const noun = bookQuery.data ? "File" : "Book";
    return (
      <LibraryLayout>
        <div className="text-center">
          <h1 className="mb-4 text-2xl font-semibold">{noun} Not Found</h1>
          <p className="text-muted-foreground">
            The {noun.toLowerCase()} you're looking for doesn't exist or may
            have been removed.
          </p>
        </div>
      </LibraryLayout>
    );
  }

  const book = bookQuery.data;
  const library = libraryQuery.data;
  const bookUrl = `/libraries/${libraryId}/books/${bookId}`;
  const isMain = file.file_role === FileRoleMain;
  const files = book.files ?? [];
  const readingAction = getReadingAction(file.file_type);
  const deliveredFileType = deliveryRulesQuery.isPending
    ? undefined
    : defaultDeliveredFileType(file, deliveryRulesQuery.data?.rules);

  const handleRescan = async (mode: ResyncMode) => {
    try {
      const result = await resyncFileMutation.mutateAsync({
        fileId: file.id,
        payload: { mode },
      });
      setRescanOpen(false);
      if ("book_deleted" in result && result.book_deleted) {
        toast.success("Book removed (no files remain)");
        leaveDeletedBook(book, result.removed);
      } else if ("file_deleted" in result && result.file_deleted) {
        toast.success("File removed (no longer on disk, or DRM-protected)");
        navigate(bookUrl);
      } else {
        toast.success("File rescanned");
      }
    } catch (error) {
      toastRequestError(error, "Failed to rescan file");
    }
  };

  const handleDelete = async () => {
    try {
      const result = await deleteFileMutation.mutateAsync(file.id);
      setDeleteOpen(false);
      if (result.book_deleted) {
        toast.success("Book deleted");
        leaveDeletedBook(book, result.removed);
      } else {
        toast.success("File deleted");
        navigate(bookUrl);
      }
    } catch (error) {
      toastRequestError(error, "Failed to delete file");
    }
  };

  const actions = (
    <div className="flex flex-wrap items-center gap-2">
      {readingAction && (
        <Button asChild size="sm">
          <Link to={`${bookUrl}/files/${file.id}/read`}>
            {readingAction === "listen" ? (
              <Headphones aria-hidden="true" className="h-4 w-4" />
            ) : (
              <BookOpen aria-hidden="true" className="h-4 w-4" />
            )}
            {readingAction === "listen" ? "Listen" : "Read"}
          </Link>
        </Button>
      )}
      <FileDownloadControl
        bookTitle={book.title}
        deliveredFileType={deliveredFileType}
        file={file}
        isDownloading={downloads.downloadingFileId === file.id}
        isSupplement={!isMain}
        onCancelDownload={downloads.cancel}
        onDownload={() =>
          deliveredFileType && downloads.download(file, deliveredFileType)
        }
        onDownloadAs={(fileType) => downloads.downloadAs(file, fileType)}
        onDownloadOriginal={() => downloads.downloadOriginal(file)}
      />
      {canWriteBooks && (
        <>
          <Button
            aria-label="Edit"
            onClick={() => setEditFileOpen(true)}
            size="sm"
            variant="outline"
          >
            <Pencil aria-hidden="true" className="h-4 w-4" />
            <span className="hidden sm:inline">Edit</span>
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button aria-label="File actions" size="icon-sm" variant="ghost">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              align="start"
              onCloseAutoFocus={(e) => e.preventDefault()}
            >
              <DropdownMenuItem
                disabled={resyncFileMutation.isPending}
                onSelect={() => setRescanOpen(true)}
              >
                <RefreshCw aria-hidden="true" className="h-4 w-4" />
                Rescan file
              </DropdownMenuItem>
              {/* A supplement belongs with its book, as on the book page. */}
              {isMain && library && (
                <DropdownMenuItem onSelect={() => setMoveOpen(true)}>
                  <MoveRight aria-hidden="true" className="h-4 w-4" />
                  Move to another book
                </DropdownMenuItem>
              )}
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="text-destructive focus:text-destructive"
                onSelect={() => setDeleteOpen(true)}
              >
                <Trash2 aria-hidden="true" className="h-4 w-4" />
                Delete file
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </>
      )}
    </div>
  );

  const filesList = files.length > 1 && (
    <BookFilesList book={book} file={file} />
  );

  return (
    <LibraryLayout>
      <LibraryBreadcrumbs
        items={[{ label: book.title, to: bookUrl }, { label: fileLabel(file) }]}
        libraryId={libraryId!}
        libraryName={library?.name ?? book.library?.name}
      />

      {/* Keyed by the file, so moving to another of the book's files starts
          its sections fresh: pages collapsed, chapters out of edit mode. */}
      <div className="space-y-10" key={file.id}>
        <FileDetailHeader
          actions={actions}
          book={book}
          coverBadge={
            isMain && (
              <BookReviewBadge
                book={book}
                files={[file]}
                onEditBook={
                  canWriteBooks ? () => setEditBookOpen(true) : undefined
                }
                onEditFile={
                  canWriteBooks ? () => setEditFileOpen(true) : undefined
                }
              />
            )
          }
          file={file}
          linkPeople={linkPeople}
          rows={[
            ...fileEditionRows(file),
            ...fileIdentifierRows(file, pluginIdentifierTypes),
          ]}
          sidePanel={filesList}
        />

        {/* Under 1280px the book's files come before the long lists below. */}
        {filesList && <div className="xl:hidden">{filesList}</div>}

        {(file.file_type === FileTypeCBZ || file.file_type === FileTypePDF) && (
          <FilePageGrid file={file} />
        )}

        <FileChaptersSection
          canEdit={canWriteBooks}
          file={file}
          onActionStateChange={setChaptersState}
          onScrolledIntoView={() => setScrollToChapters(false)}
          scrollIntoViewOnLoad={scrollToChapters}
        />

        <section aria-labelledby="file-disk-heading">
          <h2
            className="mb-3 text-sm text-muted-foreground"
            id="file-disk-heading"
          >
            On disk
          </h2>
          <LabelGrid rows={fileDiskRows(file)} />
        </section>
      </div>

      <DownloadErrorDialog
        failure={downloads.failure}
        onClose={downloads.clearFailure}
        onDownloadOriginal={(failure) =>
          downloads.downloadOriginal(failure.file)
        }
      />

      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />

      {canWriteBooks && (
        <>
          {editFileOpen && (
            <FileEditDialog
              book={book}
              file={file}
              onOpenChange={setEditFileOpen}
              open={editFileOpen}
            />
          )}
          {/* For the review badge, when a book field is what is missing. */}
          <BookEditDialog
            book={book}
            onOpenChange={setEditBookOpen}
            open={editBookOpen}
          />
          <RescanDialog
            entityName={fileLabel(file)}
            entityType="file"
            isPending={resyncFileMutation.isPending}
            onConfirm={handleRescan}
            onOpenChange={setRescanOpen}
            open={rescanOpen}
          />
          {library && isMain && (
            <MoveFilesDialog
              library={library}
              onOpenChange={setMoveOpen}
              onSuccess={(targetBook) => {
                setMoveOpen(false);
                // The file now lives under another book; follow it there.
                // Replace history so Back does not return to the old URL.
                navigate(
                  `/libraries/${libraryId}/books/${targetBook.id}/files/${file.id}`,
                  { replace: true },
                );
              }}
              open={moveOpen}
              selectedFiles={[file]}
              sourceBook={book}
            />
          )}
          <DeleteConfirmationDialog
            isPending={deleteFileMutation.isPending}
            onConfirm={handleDelete}
            onOpenChange={setDeleteOpen}
            open={deleteOpen}
            title={fileLabel(file)}
            variant="file"
          />
        </>
      )}
    </LibraryLayout>
  );
};

export default FileDetail;
