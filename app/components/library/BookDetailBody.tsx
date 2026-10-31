import {
  GitMerge,
  Info,
  List,
  MoreHorizontal,
  Pencil,
  RefreshCw,
  ScanSearch,
  Share2,
  Trash2,
} from "lucide-react";
import React, { useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";

import AddToListPopover from "@/components/library/AddToListPopover";
import BookDetailHeader from "@/components/library/BookDetailHeader";
import { BookEditDialog } from "@/components/library/BookEditDialog";
import BookFileRow from "@/components/library/BookFileRow";
import BookReadingBookmark from "@/components/library/BookReadingBookmark";
import BookReadingControls from "@/components/library/BookReadingControls";
import BookRelatedPanel from "@/components/library/BookRelatedPanel";
import BookReviewBadge from "@/components/library/BookReviewBadge";
import { DeleteConfirmationDialog } from "@/components/library/DeleteConfirmationDialog";
import DetailsDialog from "@/components/library/DetailsDialog";
import DownloadErrorDialog from "@/components/library/DownloadErrorDialog";
import { FileEditDialog } from "@/components/library/FileEditDialog";
import { IdentifyBookDialog } from "@/components/library/IdentifyBookDialog";
import { MergeIntoDialog } from "@/components/library/MergeIntoDialog";
import { MoveFilesDialog } from "@/components/library/MoveFilesDialog";
import ReadingHistory from "@/components/library/ReadingHistory";
import { RescanDialog } from "@/components/library/RescanDialog";
import { ShareLinkDialog } from "@/components/library/ShareLinkDialog";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  useDeleteBook,
  useDeleteFile,
  useResyncBook,
  useResyncFile,
} from "@/hooks/queries/books";
import { useDeliveryRules } from "@/hooks/queries/delivery";
import { usePluginIdentifierTypes } from "@/hooks/queries/plugins";
import { useSharingSettings } from "@/hooks/queries/sharing";
import { useBookRelatedTabs } from "@/hooks/useBookRelatedTabs";
import { useCan } from "@/hooks/useCan";
import { useFileDownload } from "@/hooks/useFileDownload";
import { useLeaveDeletedBook } from "@/hooks/useLeaveDeletedBook";
import { toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import type {
  Book,
  CoverAspectRatio,
  DeliveredFileType,
  File,
  LibrarySummary,
  ResyncMode,
} from "@/types";
import { getCoverFileType } from "@/utils/coverSelection";
import { bookCoverUrl } from "@/utils/coverUrl";
import { defaultDeliveredFileType } from "@/utils/delivery";
import { fileLabel, formatCount, formatDateTime } from "@/utils/format";
import { anyOf } from "@/utils/permissions";

/**
 * Set by a parent that renders a book for someone outside the library, such as
 * the recipient of a Share Link. Its presence switches the body into share
 * context: resource names render as plain text, and every action control
 * (book actions, file edits, Read and Listen, Add to list, the review badge,
 * the reading controls and history, and the Related panel) is hidden
 * regardless of the viewer's permissions. The builders replace the
 * authenticated download and cover endpoints.
 */
export interface ShareLinkContext {
  /** Endpoint the file's download button fetches (HEAD, then navigates). */
  downloadUrl: (file: File) => string;
  /** Endpoint for a download menu entry: the file as one File Type. */
  downloadAsUrl: (file: File, fileType: DeliveredFileType) => string;
  /** The book cover URL, or null when the book has no cover. */
  bookCoverUrl: (book: Book) => string | null;
  /** A file's cover URL, or null when the file has no cover. */
  fileCoverUrl: (file: File) => string | null;
  /**
   * The library's cover aspect ratio preference, which sizes the cover box.
   * Defaults to "book" (2:3).
   */
  coverAspectRatio?: CoverAspectRatio;
  /** The library's tag namespaces, which group the tags. */
  tagNamespaces?: string[];
}

const NO_NAMESPACES: string[] = [];

interface BookMenuEntry {
  label: string;
  icon: typeof Info;
  /**
   * Gated on the permission the entry's backend route requires. A group
   * with no visible entry drops out with its separator.
   */
  visible: boolean;
  onSelect: () => void;
  disabled?: boolean;
  destructive?: boolean;
}

interface BookDetailBodyProps {
  book: Book;
  /**
   * The book's library, for the cover aspect ratio, the tag namespaces, and
   * the merge and move dialogs. Absent in Share Link context.
   */
  library?: LibrarySummary;
  shareLink?: ShareLinkContext;
}

// The presentational body of Book Detail: the header (cover, title, actions,
// series, genres, tags, description), the files with their download and read
// controls, the Related panel, and the dialogs behind the actions. The page
// supplies the book and library from the authenticated query hooks; a share
// page supplies a share payload and a `shareLink`. Every new control here
// decides its Share Link behavior: anything that links into the app or
// mutates data is hidden or rendered as plain text when `isShareLink` is
// true, with a case in BookDetailBody.test.tsx, and every query is disabled
// there so the body makes no authenticated requests for the recipient.
const BookDetailBody = ({ book, library, shareLink }: BookDetailBodyProps) => {
  const isShareLink = !!shareLink;
  const libraryId = book.library_id;
  const navigate = useNavigate();
  const leaveDeletedBook = useLeaveDeletedBook();
  // Metadata, covers, chapters, review status, identify, rescan, merge, move,
  // and delete all require Books Write. List membership is governed by the
  // list's own permission and stays available to everyone.
  const canWriteBooks = useCan("books:write") && !isShareLink;
  // Shares Write creates links, and either shares operation lists them.
  const canWriteShares = useCan("shares:write") && !isShareLink;
  const canListShares =
    useCan(anyOf("shares:read", "shares:write")) && !isShareLink;
  // The sharing settings link in the Share dialog opens a Config Write page.
  const canManageSharing = useCan("config:write");
  // Author and series pages need People Read and Series Read. Without them
  // the names render as plain text instead of links to an error page.
  const linkPeople = useCan("people:read") && !isShareLink;
  const linkSeries = useCan("series:read") && !isShareLink;
  // The hook checks the endpoint's permissions itself.
  const { data: sharingSettings } = useSharingSettings({
    enabled: !isShareLink,
  });
  // Share stays offered while sharing is off: no link works then, but a
  // sharer can still revoke or delete one without an admin turning sharing
  // back on. The dialog explains the state and hides the new-link form, so it
  // waits for the settings rather than guessing the policy.
  const canShare = canListShares && sharingSettings !== undefined;
  const sharingEnabled = sharingSettings?.enabled === true;
  // The main download button follows the user's default Delivery Rules. A
  // Share Link offers each File in its own File Type: the user's rules are
  // not a recipient's.
  const deliveryRulesQuery = useDeliveryRules({ enabled: !isShareLink });
  const deliveryRulesLoading = !isShareLink && deliveryRulesQuery.isPending;
  const deliveryRules = deliveryRulesQuery.data?.rules;
  const { data: pluginIdentifierTypes } = usePluginIdentifierTypes({
    enabled: !isShareLink,
  });

  const resyncFileMutation = useResyncFile();
  const resyncBookMutation = useResyncBook();
  const deleteBookMutation = useDeleteBook();
  const deleteFileMutation = useDeleteFile();
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [addToListOpen, setAddToListOpen] = useState(false);
  const [shareDialogOpen, setShareDialogOpen] = useState(false);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [showBookRescanDialog, setShowBookRescanDialog] = useState(false);
  const [rescanFileId, setRescanFileId] = useState<number | null>(null);
  const [showMergeIntoDialog, setShowMergeIntoDialog] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [editingFile, setEditingFile] = useState<File | null>(null);
  const downloads = useFileDownload(shareLink);
  const [resyncingFileId, setResyncingFileId] = useState<number | null>(null);
  const [moveFileId, setMoveFileId] = useState<number | null>(null);
  const [deletingFileId, setDeletingFileId] = useState<number | null>(null);
  const [fileToDelete, setFileToDelete] = useState<File | null>(null);
  const [showIdentifyDialog, setShowIdentifyDialog] = useState(false);

  const coverUrl = shareLink
    ? shareLink.bookCoverUrl(book)
    : bookCoverUrl(book);

  // The Related panel's content, loaded once for both places it can sit. A
  // recipient gets no panel and the hook sends nothing for one.
  const related = useBookRelatedTabs(book, { enabled: !isShareLink });
  const relatedPanel =
    !isShareLink && related.tabs.length > 0 ? (
      <BookRelatedPanel book={book} related={related} />
    ) : null;

  const handleRescanFile = async (fileId: number, mode: ResyncMode) => {
    setResyncingFileId(fileId);
    try {
      const result = await resyncFileMutation.mutateAsync({
        fileId,
        payload: { mode },
      });
      if ("book_deleted" in result && result.book_deleted) {
        toast.success("Book removed (no files remain)");
        leaveDeletedBook(book, result.removed);
      } else if ("file_deleted" in result && result.file_deleted) {
        toast.success("File removed (no longer on disk, or DRM-protected)");
      } else {
        toast.success("File rescanned");
      }
    } catch (error) {
      toastRequestError(error, "Failed to rescan file");
    } finally {
      setResyncingFileId(null);
    }
  };

  const handleRescanBook = async (mode: ResyncMode) => {
    try {
      const result = await resyncBookMutation.mutateAsync({
        bookId: book.id,
        payload: { mode },
      });
      if ("book_deleted" in result && result.book_deleted) {
        toast.success("Book removed (no files remain)");
        leaveDeletedBook(book, result.removed);
      } else {
        toast.success("Book rescanned");
      }
    } catch (error) {
      toastRequestError(error, "Failed to rescan book");
    }
  };

  const handleDeleteBook = async () => {
    try {
      const result = await deleteBookMutation.mutateAsync(book.id);
      toast.success("Book deleted");
      leaveDeletedBook(book, result.removed);
    } catch (error) {
      toastRequestError(error, "Failed to delete book");
    }
  };

  const handleDeleteFile = async () => {
    if (!fileToDelete) return;
    setDeletingFileId(fileToDelete.id);
    try {
      const result = await deleteFileMutation.mutateAsync(fileToDelete.id);
      setFileToDelete(null);
      if (result.book_deleted) {
        toast.success("Book deleted");
        leaveDeletedBook(book, result.removed);
      } else {
        toast.success("File deleted");
      }
    } catch (error) {
      toastRequestError(error, "Failed to delete file");
    } finally {
      setDeletingFileId(null);
    }
  };

  // Separate main files and supplements
  const mainFiles =
    book.files?.filter((f) => f.file_role !== "supplement") ?? [];
  const supplements =
    book.files?.filter((f) => f.file_role === "supplement") ?? [];

  // The cover box takes the shape the library's gallery gives the book.
  const libraryCoverAspectRatio =
    shareLink?.coverAspectRatio ?? library?.cover_aspect_ratio ?? "book";
  const tagNamespaces =
    shareLink?.tagNamespaces ?? library?.tag_namespaces ?? NO_NAMESPACES;
  const coverVariant = getCoverFileType(book.files, libraryCoverAspectRatio);

  // The ⋮ menu's entries, grouped by separator. Each entry carries its own
  // permission. Book details and Add to list need none (the list's own
  // permission governs membership), so the menu is always there.
  const bookMenuGroups: BookMenuEntry[][] = [
    [
      {
        label: "Book details",
        icon: Info,
        visible: true,
        onSelect: () => setDetailsOpen(true),
      },
      {
        label: "Add to list",
        icon: List,
        visible: true,
        // Opens after the menu closes, so the popover anchors and keeps
        // focus instead of the closing menu taking it back.
        onSelect: () => setTimeout(() => setAddToListOpen(true), 0),
      },
    ],
    [
      {
        label: "Rescan book",
        icon: RefreshCw,
        visible: canWriteBooks,
        disabled: resyncBookMutation.isPending,
        onSelect: () => setShowBookRescanDialog(true),
      },
      {
        label: "Merge into another book",
        icon: GitMerge,
        visible: canWriteBooks,
        onSelect: () => setShowMergeIntoDialog(true),
      },
    ],
    [
      {
        label: "Delete book",
        icon: Trash2,
        visible: canWriteBooks,
        destructive: true,
        onSelect: () => setShowDeleteDialog(true),
      },
    ],
  ]
    .map((group) => group.filter((entry) => entry.visible))
    .filter((group) => group.length > 0);

  // Header buttons with text hide the label on phones and keep the icon;
  // the aria-label repeats the text so the name survives.
  const headerButton = (
    label: string,
    Icon: typeof Info,
    onClick: () => void,
  ) => (
    <Button aria-label={label} onClick={onClick} size="sm" variant="outline">
      <Icon className="h-4 w-4" />
      <span className="hidden sm:inline">{label}</span>
    </Button>
  );

  const actions = !isShareLink && (
    <div className="flex flex-wrap items-center gap-2">
      {canWriteBooks &&
        headerButton("Edit", Pencil, () => setEditDialogOpen(true))}
      {canWriteBooks &&
        headerButton("Identify", ScanSearch, () => setShowIdentifyDialog(true))}
      {canShare &&
        headerButton("Share", Share2, () => setShareDialogOpen(true))}
      {/*
        The relative wrapper exists so AddToListPopover can be opened from the
        menu's "Add to list" item: its trigger is an invisible absolute span
        that anchors the popover to the same rectangle as the ⋮ button.
      */}
      <div className="relative">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button aria-label="Book actions" size="icon-sm" variant="ghost">
              <MoreHorizontal className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="start"
            onCloseAutoFocus={(e) => e.preventDefault()}
          >
            {bookMenuGroups.map((group, groupIndex) => (
              <React.Fragment key={group[0].label}>
                {groupIndex > 0 && <DropdownMenuSeparator />}
                {group.map((entry) => (
                  <DropdownMenuItem
                    className={cn(
                      entry.destructive &&
                        "text-destructive focus:text-destructive",
                    )}
                    disabled={entry.disabled}
                    key={entry.label}
                    onSelect={entry.onSelect}
                  >
                    <entry.icon
                      className={cn(
                        "h-4 w-4 mr-2",
                        entry.destructive && "text-destructive",
                      )}
                    />
                    {entry.label}
                  </DropdownMenuItem>
                ))}
              </React.Fragment>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        <AddToListPopover
          bookId={book.id}
          onOpenChange={setAddToListOpen}
          open={addToListOpen}
          trigger={
            <span
              aria-hidden
              className="absolute inset-0 pointer-events-none"
            />
          }
        />
      </div>
    </div>
  );

  const renderFileRow = (file: File, isSupplement: boolean) => {
    // The main button waits for the rules that decide its File Type.
    const deliveredFileType = deliveryRulesLoading
      ? undefined
      : defaultDeliveredFileType(file, deliveryRules);
    return (
      <BookFileRow
        bookTitle={book.title}
        canWriteBooks={canWriteBooks}
        deliveredFileType={deliveredFileType}
        file={file}
        getCoverUrl={shareLink?.fileCoverUrl}
        isDeletingFile={deletingFileId === file.id}
        isDownloading={downloads.downloadingFileId === file.id}
        isResyncing={resyncingFileId === file.id}
        isShareLink={isShareLink}
        isSupplement={isSupplement}
        key={file.id}
        libraryId={libraryId}
        onCancelDownload={downloads.cancel}
        onDeleteFile={() => setFileToDelete(file)}
        onDownload={() =>
          deliveredFileType && downloads.download(file, deliveredFileType)
        }
        onDownloadAs={(fileType) => downloads.downloadAs(file, fileType)}
        onDownloadOriginal={() => downloads.downloadOriginal(file)}
        onEdit={() => setEditingFile(file)}
        onMoveFile={isSupplement ? undefined : () => setMoveFileId(file.id)}
        onRescan={() => setRescanFileId(file.id)}
        pluginIdentifierTypes={pluginIdentifierTypes}
      />
    );
  };

  return (
    <>
      <BookDetailHeader
        actions={actions}
        book={book}
        coverBadge={
          !isShareLink && (
            <BookReviewBadge
              book={book}
              onEditBook={
                canWriteBooks ? () => setEditDialogOpen(true) : undefined
              }
              onEditFile={canWriteBooks ? setEditingFile : undefined}
            />
          )
        }
        coverBookmark={!isShareLink && <BookReadingBookmark bookId={book.id} />}
        coverUrl={coverUrl}
        coverVariant={coverVariant}
        isShareLink={isShareLink}
        // A new book starts with its description collapsed and its cover's
        // placeholder showing until the image loads.
        key={book.id}
        linkPeople={linkPeople}
        linkSeries={linkSeries}
        reading={!isShareLink && <BookReadingControls bookId={book.id} />}
        sidePanel={relatedPanel}
        tagNamespaces={tagNamespaces}
      />

      <section aria-labelledby="book-files-heading" className="mt-10">
        <h2
          className="mb-3 text-sm text-muted-foreground"
          id="book-files-heading"
        >
          {formatCount(mainFiles.length, "file")}
        </h2>
        <div className="space-y-2">
          {mainFiles.map((file) => renderFileRow(file, false))}
        </div>
      </section>

      {supplements.length > 0 && (
        <section aria-labelledby="book-supplements-heading" className="mt-8">
          <h2
            className="mb-3 text-sm text-muted-foreground"
            id="book-supplements-heading"
          >
            {formatCount(supplements.length, "supplement")}
          </h2>
          <div className="space-y-2">
            {supplements.map((file) => renderFileRow(file, true))}
          </div>
        </section>
      )}

      {!isShareLink && <ReadingHistory bookId={book.id} />}

      {/* Under 1280px the Related panel follows the files: they matter more.
          It renders twice, one copy hidden by CSS at each width, so it can
          sit beside the header or below the files without a resize hook. */}
      {/* Keyed by the book like the header, so the next book's panel opens
          centered on it. The prefix keeps the two sibling keys apart. */}
      {relatedPanel && (
        <div className="mt-10 xl:hidden" key={`related-${book.id}`}>
          {relatedPanel}
        </div>
      )}

      {!isShareLink && (
        <DetailsDialog
          onOpenChange={setDetailsOpen}
          open={detailsOpen}
          rows={[
            {
              label: "Library",
              value: book.library?.name || `Library ${book.library_id}`,
            },
            {
              label: "Folder",
              value: book.filepath.split("/").map((segment, i, arr) => (
                <React.Fragment key={i}>
                  {segment}
                  {i < arr.length - 1 && (
                    <>
                      /
                      <wbr />
                    </>
                  )}
                </React.Fragment>
              )),
            },
            { label: "Sort title", value: book.sort_title },
            { label: "Added", value: formatDateTime(book.created_at) },
            { label: "Updated", value: formatDateTime(book.updated_at) },
          ]}
          title="Book details"
        />
      )}

      {canWriteBooks && (
        <>
          <BookEditDialog
            book={book}
            onOpenChange={setEditDialogOpen}
            open={editDialogOpen}
          />

          <IdentifyBookDialog
            book={book}
            onOpenChange={setShowIdentifyDialog}
            open={showIdentifyDialog}
          />

          <RescanDialog
            entityName={book.title}
            entityType="book"
            isPending={resyncBookMutation.isPending}
            onConfirm={handleRescanBook}
            onOpenChange={setShowBookRescanDialog}
            open={showBookRescanDialog}
          />

          {(() => {
            const rescanFile = rescanFileId
              ? book.files.find((f) => f.id === rescanFileId)
              : null;
            return (
              <RescanDialog
                entityName={rescanFile ? fileLabel(rescanFile) : ""}
                entityType="file"
                isPending={resyncFileMutation.isPending}
                onConfirm={(mode) => {
                  if (rescanFileId) handleRescanFile(rescanFileId, mode);
                }}
                onOpenChange={(open) => {
                  if (!open) setRescanFileId(null);
                }}
                open={rescanFileId !== null}
              />
            );
          })()}

          {editingFile && (
            <FileEditDialog
              book={book}
              file={editingFile}
              onOpenChange={(open) => {
                if (!open) setEditingFile(null);
              }}
              open={!!editingFile}
            />
          )}
        </>
      )}

      <DownloadErrorDialog
        failure={downloads.failure}
        onClose={downloads.clearFailure}
        // A recipient cannot have the file as it is on disk.
        onDownloadOriginal={
          isShareLink
            ? undefined
            : (failure) => downloads.downloadOriginal(failure.file)
        }
      />

      {canWriteBooks && library && (
        <MergeIntoDialog
          library={library}
          onOpenChange={setShowMergeIntoDialog}
          onSuccess={(targetBook) => {
            // Replace history so back button doesn't return to the now-deleted book
            navigate(`/libraries/${libraryId}/books/${targetBook.id}`, {
              replace: true,
            });
          }}
          open={showMergeIntoDialog}
          sourceBook={book}
        />
      )}

      {canWriteBooks && library && (
        <MoveFilesDialog
          library={library}
          onOpenChange={(open) => {
            if (!open) setMoveFileId(null);
          }}
          onSuccess={(targetBook) => {
            setMoveFileId(null);
            // Moving the last main file deletes this book, so go to the one
            // it moved to. Replace history so Back doesn't return to it.
            if (mainFiles.length === 1) {
              navigate(`/libraries/${libraryId}/books/${targetBook.id}`, {
                replace: true,
              });
            }
          }}
          open={moveFileId !== null}
          selectedFiles={mainFiles.filter((f) => f.id === moveFileId)}
          sourceBook={book}
        />
      )}

      {canWriteBooks && (
        <DeleteConfirmationDialog
          files={book.files}
          isPending={deleteBookMutation.isPending}
          onConfirm={handleDeleteBook}
          onOpenChange={setShowDeleteDialog}
          open={showDeleteDialog}
          title={book.title}
          variant="book"
        />
      )}

      {canShare && (
        <ShareLinkDialog
          bookId={book.id}
          bookTitle={book.title}
          canList={canListShares}
          canManageSharing={canManageSharing}
          canWrite={canWriteShares}
          onOpenChange={setShareDialogOpen}
          open={shareDialogOpen}
          requireExpiration={sharingSettings?.require_expiration ?? false}
          sharingEnabled={sharingEnabled}
        />
      )}

      {canWriteBooks && fileToDelete && (
        <DeleteConfirmationDialog
          isPending={deleteFileMutation.isPending}
          onConfirm={handleDeleteFile}
          onOpenChange={(open) => !open && setFileToDelete(null)}
          open={!!fileToDelete}
          title={fileToDelete.filepath.split("/").pop() ?? "File"}
          variant="file"
        />
      )}
    </>
  );
};

export default BookDetailBody;
