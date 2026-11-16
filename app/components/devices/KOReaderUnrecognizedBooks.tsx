import {
  ChevronDown,
  Link as LinkIcon,
  Loader2,
  Search,
  Trash2,
} from "lucide-react";
import { useId, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import DeviceSection from "@/components/devices/DeviceSection";
import Disclosure from "@/components/devices/Disclosure";
import QueryError from "@/components/library/QueryError";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  useDeleteKOReaderUnrecognizedReading,
  useDismissAllKOReaderUnrecognized,
  useDismissKOReaderUnrecognized,
  useKOReaderLinkCandidates,
  useKOReaderUnrecognizedBooks,
  useLinkKOReaderUnrecognized,
  useRestoreKOReaderUnrecognized,
  useUnlinkKOReaderUnrecognized,
} from "@/hooks/queries/devices";
import { useDebounce } from "@/hooks/useDebounce";
import { isDemoModeError, toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import type { DeviceResponse } from "@/types/generated/devices";
import {
  LinkMatchIdentifier,
  type LinkCandidate,
  type UnrecognizedBook,
} from "@/types/generated/koreaderplugin";
import { fileTypeLabel, formatCount, formatDuration } from "@/utils/format";
import { formatLastRead } from "@/utils/readingProgress";

// The name a copy goes by: KOReader's title, or the start of its hash when
// KOReader knows none.
const copyName = (book: UnrecognizedBook) =>
  book.title?.trim() || `Untitled (${book.document.slice(0, 8)})`;

const bookPath = (candidate: LinkCandidate) =>
  `/libraries/${candidate.library_id}/books/${candidate.book_id}`;

// What tells two candidates apart: authors, File Type, and library.
const candidateDetails = (candidate: LinkCandidate) =>
  [
    candidate.authors.join(", "),
    fileTypeLabel(candidate.file_type),
    candidate.library_name,
  ]
    .filter(Boolean)
    .join(" · ");

// CandidateList offers Files to link a copy to, each with its own Link.
// Compact puts each on one line, as a row's best match shows.
const CandidateList = ({
  candidates,
  label,
  linking,
  onLink,
  compact = false,
}: {
  candidates: LinkCandidate[];
  label: string;
  linking: boolean;
  onLink: (candidate: LinkCandidate) => void;
  compact?: boolean;
}) => (
  <ul aria-label={label} className="space-y-2">
    {candidates.map((candidate) => (
      <li
        className={cn(
          "flex items-center justify-between gap-3 rounded-md bg-muted/50 px-3",
          compact ? "py-1.5" : "py-2",
        )}
        key={candidate.file_id}
      >
        <div
          className={cn(
            "min-w-0 text-sm",
            compact && "flex items-center gap-2",
          )}
        >
          <p
            className={cn(
              "flex items-center gap-2 font-medium",
              // Compact, the title gives way before the Link button does.
              compact ? "min-w-0" : "flex-wrap",
            )}
          >
            <span className={compact ? "truncate" : "break-words"}>
              {candidate.title}
            </span>
            {candidate.match === LinkMatchIdentifier && (
              <Badge className="shrink-0" variant="secondary">
                Same identifier
              </Badge>
            )}
          </p>
          <p
            className={cn(
              "text-muted-foreground",
              compact ? "min-w-0 truncate" : "break-words",
            )}
          >
            {candidateDetails(candidate)}
          </p>
        </div>
        <Button
          aria-label={`Link to ${candidate.title}, ${fileTypeLabel(candidate.file_type)} in ${candidate.library_name}`}
          className="shrink-0"
          disabled={linking}
          onClick={() => onLink(candidate)}
          size="sm"
          variant="outline"
        >
          <LinkIcon className="h-4 w-4 sm:mr-2" />
          <span className="hidden sm:inline">Link</span>
        </Button>
      </li>
    ))}
  </ul>
);

// CandidateSearch finds a File to link a copy to when no suggestion fits.
const CandidateSearch = ({
  deviceId,
  name,
  linking,
  onLink,
}: {
  deviceId: string;
  name: string;
  linking: boolean;
  onLink: (candidate: LinkCandidate) => void;
}) => {
  const [search, setSearch] = useState("");
  const debounced = useDebounce(search.trim(), 300);
  const query = useKOReaderLinkCandidates(deviceId, debounced);

  return (
    <div className="space-y-2">
      <div className="relative">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          aria-label={`Search for ${name}`}
          className="pl-9"
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search books..."
          value={search}
        />
        {query.isFetching && (
          <Loader2 className="absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 animate-spin text-muted-foreground" />
        )}
      </div>
      {debounced !== "" && query.error && !query.data && (
        <QueryError fallback="Failed to search books" query={query} />
      )}
      {debounced !== "" &&
        query.data &&
        (query.data.length > 0 ? (
          <CandidateList
            candidates={query.data}
            label={`Search results for ${name}`}
            linking={linking}
            onLink={onLink}
          />
        ) : (
          <p className="text-sm text-muted-foreground">
            No books match your search.
          </p>
        ))}
    </div>
  );
};

// How many books still to link show before the rest are asked for.
const FIRST_SHOWN = 10;

// details is what KOReader knows of a copy besides its title, on one line.
const details = (book: UnrecognizedBook) =>
  [
    book.authors.join(", "),
    book.reading_seconds > 0
      ? `${formatDuration(book.reading_seconds)} read in KOReader`
      : null,
    formatLastRead(book.last_read_at),
  ]
    .filter(Boolean)
    .join(" · ");

// ToLinkRow is a copy still to link, kept to a few lines: its best match,
// if it has one, linked in one click, Dismiss, and More for the other
// suggestions and a search. Dismissing only sets the copy aside, so it asks
// nothing first and offers Undo instead.
const ToLinkRow = ({
  deviceId,
  book,
}: {
  deviceId: string;
  book: UnrecognizedBook;
}) => {
  const name = copyName(book);
  const panelId = useId();
  const [open, setOpen] = useState(false);
  const link = useLinkKOReaderUnrecognized();
  const dismiss = useDismissKOReaderUnrecognized();
  const restore = useRestoreKOReaderUnrecognized();
  const best = book.suggestions[0];
  const about = details(book);

  const handleLink = async (candidate: LinkCandidate) => {
    try {
      await link.mutateAsync({
        deviceId,
        document: book.document,
        fileId: candidate.file_id,
      });
      toast.success(`Linked to ${candidate.title}`);
    } catch (error) {
      toastRequestError(error, "Failed to link the book");
    }
  };

  const handleDismiss = async () => {
    const copy = { deviceId, document: book.document };
    try {
      await dismiss.mutateAsync(copy);
    } catch (error) {
      toastRequestError(error, "Failed to dismiss the book");
      return;
    }
    const id = `koreader-dismissed-${book.document}`;
    // Undo is the app's own button: the toast library's action button
    // doesn't follow the app's styles.
    toast.success(`Dismissed ${name}`, {
      id,
      description: (
        <div className="mt-1 flex justify-end">
          <Button
            onClick={() => {
              toast.dismiss(id);
              restore
                .mutateAsync(copy)
                .catch((error) =>
                  toastRequestError(error, "Failed to restore the book"),
                );
            }}
            size="sm"
            variant="outline"
          >
            Undo
          </Button>
        </div>
      ),
    });
  };

  return (
    <li aria-label={name} className="space-y-2 py-3 first:pt-0 last:pb-0">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate font-medium" title={name}>
            {name}
          </p>
          {about && (
            <p
              className="line-clamp-2 text-sm text-muted-foreground sm:line-clamp-1"
              title={about}
            >
              {about}
            </p>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Button
            aria-label={`Dismiss ${name}`}
            disabled={dismiss.isPending}
            onClick={handleDismiss}
            size="sm"
            variant="ghost"
          >
            Dismiss
          </Button>
          <Button
            aria-controls={panelId}
            aria-expanded={open}
            aria-label={`More options for ${name}`}
            onClick={() => setOpen((value) => !value)}
            size="sm"
            variant="ghost"
          >
            More
            <ChevronDown
              className={cn(
                "ml-1 h-4 w-4 transition-transform",
                open && "rotate-180",
              )}
            />
          </Button>
        </div>
      </div>
      {open ? (
        <div className="space-y-3" id={panelId}>
          {book.suggestions.length > 0 ? (
            <CandidateList
              candidates={book.suggestions}
              label={`Suggestions for ${name}`}
              linking={link.isPending}
              onLink={handleLink}
            />
          ) : (
            <p className="text-sm text-muted-foreground">
              No suggestions. Search for the book to link it.
            </p>
          )}
          <CandidateSearch
            deviceId={deviceId}
            linking={link.isPending}
            name={name}
            onLink={handleLink}
          />
        </div>
      ) : (
        best && (
          <CandidateList
            candidates={[best]}
            compact
            label={`Best match for ${name}`}
            linking={link.isPending}
            onLink={handleLink}
          />
        )
      )}
    </li>
  );
};

// SKIP_DELETE_CONFIRM_KEY remembers, in this browser, that the user chose
// not to be asked before deleting a dismissed book's reading history, so
// deleting several takes one click each. The book stays listed to restore,
// so not asking loses nothing the plugin can't send again.
const SKIP_DELETE_CONFIRM_KEY = "shisho-koreader-skip-delete-history-confirm";

const skipsDeleteConfirm = () => {
  try {
    return localStorage.getItem(SKIP_DELETE_CONFIRM_KEY) === "true";
  } catch {
    return false;
  }
};

const skipDeleteConfirm = () => {
  try {
    localStorage.setItem(SKIP_DELETE_CONFIRM_KEY, "true");
  } catch {
    // Without storage, it asks again next time.
  }
};

// DismissedRow is a copy set aside, on one line: Restore puts it back to
// link, and Delete history drops the reading history held for it, as for a
// book from another server's library this one should never hold. One whose
// history was deleted can only be restored.
const DismissedRow = ({
  deviceId,
  book,
}: {
  deviceId: string;
  book: UnrecognizedBook;
}) => {
  const name = copyName(book);
  const restore = useRestoreKOReaderUnrecognized();
  const deleteReading = useDeleteKOReaderUnrecognizedReading();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [dontAsk, setDontAsk] = useState(false);
  const dontAskId = useId();
  const about = book.reading_deleted
    ? "Reading history deleted. Reading sessions KOReader sends for it are dropped."
    : details(book);

  const handleRestore = async () => {
    try {
      await restore.mutateAsync({ deviceId, document: book.document });
    } catch (error) {
      toastRequestError(error, "Failed to restore the book");
      return;
    }
    // Its history comes back only from the e-reader, and before a link, so
    // its status and stars come too.
    if (book.reading_deleted) {
      toast.success(`Restored ${name}`, {
        description:
          "To bring its reading history back, choose Send reading history again in the KOReader plugin's menu before you link it.",
      });
    }
  };

  const handleDelete = async () => {
    try {
      await deleteReading.mutateAsync({ deviceId, document: book.document });
      if (dontAsk) skipDeleteConfirm();
      setConfirmOpen(false);
    } catch (error) {
      // Demo Mode already toasted the rejection; nothing will change, so
      // there is nothing left to confirm.
      if (isDemoModeError(error)) setConfirmOpen(false);
      toastRequestError(error, "Failed to delete the reading history");
    }
  };

  return (
    <li
      aria-label={name}
      className="flex items-center justify-between gap-3 py-2 first:pt-0 last:pb-0"
    >
      <div className="min-w-0 text-sm">
        <p className="truncate font-medium" title={name}>
          {name}
        </p>
        {about && (
          <p className="truncate text-muted-foreground" title={about}>
            {about}
          </p>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-1">
        <Button
          aria-label={`Restore ${name}`}
          disabled={restore.isPending}
          onClick={handleRestore}
          size="sm"
          variant="outline"
        >
          Restore
        </Button>
        {!book.reading_deleted && (
          <Button
            aria-label={`Delete the reading history of ${name}`}
            disabled={deleteReading.isPending}
            onClick={() => {
              if (skipsDeleteConfirm()) {
                void handleDelete();
              } else {
                setConfirmOpen(true);
              }
            }}
            size="sm"
            variant="ghost"
          >
            <Trash2 className="h-4 w-4 text-destructive sm:mr-2" />
            <span className="hidden sm:inline">Delete history</span>
          </Button>
        )}
      </div>
      <ConfirmDialog
        confirmLabel="Delete"
        description={`The reading history held for "${name}", its reading sessions, status, and Rating, is deleted from this server, and reading sessions KOReader sends for it later are dropped. Use this for a book from another server's library. KOReader keeps its own record on the e-reader: restore the book and choose Send reading history again in the plugin's menu to bring the history back.`}
        isPending={deleteReading.isPending}
        onConfirm={handleDelete}
        onOpenChange={setConfirmOpen}
        open={confirmOpen}
        title="Delete Reading History"
        variant="destructive"
      >
        <div className="mt-4 flex items-center gap-2">
          <Checkbox
            checked={dontAsk}
            id={dontAskId}
            onCheckedChange={(checked) => setDontAsk(checked === true)}
          />
          <Label className="font-normal" htmlFor={dontAskId}>
            Don't ask again
          </Label>
        </div>
      </ConfirmDialog>
    </li>
  );
};

// LinkedRow is a copy already linked, on one line: the Book it's linked to,
// and Unlink for a link made to the wrong Book.
const LinkedRow = ({
  deviceId,
  book,
}: {
  deviceId: string;
  book: UnrecognizedBook;
}) => {
  const name = copyName(book);
  const unlink = useUnlinkKOReaderUnrecognized();
  const [confirmOpen, setConfirmOpen] = useState(false);

  const handleUnlink = async () => {
    try {
      await unlink.mutateAsync({ deviceId, document: book.document });
      setConfirmOpen(false);
    } catch (error) {
      // Demo Mode already toasted the rejection; nothing will change, so
      // there is nothing left to confirm.
      if (isDemoModeError(error)) setConfirmOpen(false);
      toastRequestError(error, "Failed to unlink the book");
    }
  };

  return (
    <li
      aria-label={name}
      className="flex items-center justify-between gap-3 py-2 first:pt-0 last:pb-0"
    >
      <div className="min-w-0 text-sm">
        <p className="truncate font-medium" title={name}>
          {name}
        </p>
        {book.linked ? (
          <p className="truncate text-muted-foreground">
            Linked to{" "}
            <Link
              className="text-foreground underline underline-offset-2"
              to={bookPath(book.linked)}
            >
              {book.linked.title}
            </Link>{" "}
            · {fileTypeLabel(book.linked.file_type)} ·{" "}
            {book.linked.library_name}
          </p>
        ) : (
          <p className="text-muted-foreground">
            Linked to a file that has since been deleted.
          </p>
        )}
      </div>
      <Button
        aria-label={`Unlink ${name}`}
        className="shrink-0"
        onClick={() => setConfirmOpen(true)}
        size="sm"
        variant="outline"
      >
        Unlink
      </Button>
      <ConfirmDialog
        confirmLabel="Unlink"
        description={`"${name}" stops syncing with this book and is listed to link again, on every one of your Devices. Reading sessions KOReader sends for it from then on are held until you link it. The reading sessions, status, and Rating the link added stay; delete any that are wrong from the book's page.`}
        isPending={unlink.isPending}
        onConfirm={handleUnlink}
        onOpenChange={setConfirmOpen}
        open={confirmOpen}
        title="Unlink Book"
      />
    </li>
  );
};

/**
 * KOReaderUnrecognizedBooks lists the copies on a KOReader Device that no
 * server the KOReader plugin signed in to recognizes, such as ones downloaded
 * before Shisho recorded what it sent, so the user can link each to its Book
 * or dismiss it to link later (ADR 0017). A Device can hold many, so each
 * row is a few lines, the first FIRST_SHOWN show until the rest are asked
 * for, and the linked and dismissed ones fold away. It shows nothing while
 * there are none.
 */
const KOReaderUnrecognizedBooks = ({ device }: { device: DeviceResponse }) => {
  const query = useKOReaderUnrecognizedBooks(device.id);
  const [showAll, setShowAll] = useState(false);
  const dismissAll = useDismissAllKOReaderUnrecognized();
  const [confirmDismissAll, setConfirmDismissAll] = useState(false);
  const books = query.data ?? [];
  const toLink = books.filter((book) => !book.linked_at && !book.dismissed_at);
  const linked = books.filter((book) => book.linked_at);
  const dismissed = books.filter((book) => book.dismissed_at);

  // Linked books fold away while there are books to link, and open when
  // none are left, as after Dismiss all or linking the last one. Opening or
  // closing them by hand stays until that changes.
  const nothingToLink = query.data ? toLink.length === 0 : null;
  const [linkedOpen, setLinkedOpen] = useState(false);
  const [wasNothingToLink, setWasNothingToLink] = useState(nothingToLink);
  if (nothingToLink !== wasNothingToLink) {
    setWasNothingToLink(nothingToLink);
    if (nothingToLink) setLinkedOpen(true);
  }

  const handleDismissAll = async () => {
    try {
      // Only the books listed, so one that arrived since isn't dismissed
      // unseen.
      await dismissAll.mutateAsync({
        deviceId: device.id,
        documents: toLink.map((book) => book.document),
      });
      setConfirmDismissAll(false);
    } catch (error) {
      // Demo Mode already toasted the rejection; nothing will change, so
      // there is nothing left to confirm.
      if (isDemoModeError(error)) setConfirmDismissAll(false);
      toastRequestError(error, "Failed to dismiss the books");
    }
  };

  if (query.error && !query.data) {
    return (
      <DeviceSection id="unrecognized-heading" title="Unrecognized Books">
        <QueryError
          fallback="Failed to load unrecognized books"
          query={query}
        />
      </DeviceSection>
    );
  }
  if (books.length === 0) return null;

  const shown = showAll ? toLink : toLink.slice(0, FIRST_SHOWN);

  return (
    <DeviceSection
      description="Books KOReader has reading data for that Shisho doesn't recognize, such as copies downloaded before Shisho v0.7.0 or from somewhere else. Link each to its book to bring its reading history in, or dismiss it to link later."
      id="unrecognized-heading"
      title="Unrecognized Books"
    >
      {toLink.length > 0 && (
        <div className="space-y-2">
          <div className="flex items-center justify-between gap-3">
            <p className="text-sm font-medium">
              {toLink.length.toLocaleString()} to link
            </p>
            {toLink.length > 1 && (
              <Button
                onClick={() => setConfirmDismissAll(true)}
                size="sm"
                variant="ghost"
              >
                Dismiss all
              </Button>
            )}
          </div>
          <ConfirmDialog
            confirmLabel="Dismiss all"
            description={`All ${formatCount(toLink.length, "book")} still to link move to Dismissed. Their reading history stays held, so you can restore and link any of them later, such as once you add the book to your library. Linked books stay.`}
            isPending={dismissAll.isPending}
            onConfirm={handleDismissAll}
            onOpenChange={setConfirmDismissAll}
            open={confirmDismissAll}
            title="Dismiss All Books"
          />
          <ul
            aria-label="Unrecognized books"
            className="divide-y divide-border"
          >
            {shown.map((book) => (
              <ToLinkRow book={book} deviceId={device.id} key={book.document} />
            ))}
          </ul>
          {shown.length < toLink.length && (
            <Button
              onClick={() => setShowAll(true)}
              size="sm"
              variant="outline"
            >
              Show {(toLink.length - shown.length).toLocaleString()} more
            </Button>
          )}
        </div>
      )}
      {linked.length > 0 && (
        <Disclosure
          onOpenChange={setLinkedOpen}
          open={linkedOpen}
          size="sm"
          title={`Linked (${linked.length.toLocaleString()})`}
        >
          <p className="text-sm text-muted-foreground">
            A linked copy syncs your place as a percentage only, so it can land
            a little off. Download the book again from Shisho to sync your exact
            place.
          </p>
          <ul aria-label="Linked books" className="divide-y divide-border">
            {linked.map((book) => (
              <LinkedRow book={book} deviceId={device.id} key={book.document} />
            ))}
          </ul>
        </Disclosure>
      )}
      {dismissed.length > 0 && (
        <Disclosure
          size="sm"
          title={`Dismissed (${dismissed.length.toLocaleString()})`}
        >
          <p className="text-sm text-muted-foreground">
            A dismissed book keeps its reading history, so you can restore it
            and link it once the book is in your library. Delete the history of
            a book that belongs to another server.
          </p>
          <ul aria-label="Dismissed books" className="divide-y divide-border">
            {dismissed.map((book) => (
              <DismissedRow
                book={book}
                deviceId={device.id}
                key={book.document}
              />
            ))}
          </ul>
        </Disclosure>
      )}
    </DeviceSection>
  );
};

export default KOReaderUnrecognizedBooks;
