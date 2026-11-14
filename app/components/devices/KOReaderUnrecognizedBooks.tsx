import { ChevronDown, Link as LinkIcon, Loader2, Search } from "lucide-react";
import { useId, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import DeviceSection from "@/components/devices/DeviceSection";
import Disclosure from "@/components/devices/Disclosure";
import QueryError from "@/components/library/QueryError";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Input } from "@/components/ui/input";
import {
  useDismissKOReaderUnrecognized,
  useKOReaderLinkCandidates,
  useKOReaderUnrecognizedBooks,
  useLinkKOReaderUnrecognized,
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
import { fileTypeLabel, formatDuration } from "@/utils/format";
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
              compact ? "min-w-0 shrink-0" : "flex-wrap",
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
// if it has one, linked in one click, and More for the other suggestions, a
// search, and Dismiss.
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
  const [confirmOpen, setConfirmOpen] = useState(false);
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
    try {
      await dismiss.mutateAsync({ deviceId, document: book.document });
      setConfirmOpen(false);
    } catch (error) {
      // Demo Mode already toasted the rejection; nothing will change, so
      // there is nothing left to confirm.
      if (isDemoModeError(error)) setConfirmOpen(false);
      toastRequestError(error, "Failed to dismiss the book");
    }
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
        <Button
          aria-controls={panelId}
          aria-expanded={open}
          aria-label={`${open ? "Fewer" : "More"} options for ${name}`}
          className="shrink-0"
          onClick={() => setOpen((value) => !value)}
          size="sm"
          variant="ghost"
        >
          {open ? "Less" : "More"}
          <ChevronDown
            className={cn(
              "ml-1 h-4 w-4 transition-transform",
              open && "rotate-180",
            )}
          />
        </Button>
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
          <Button
            aria-label={`Dismiss ${name}`}
            onClick={() => setConfirmOpen(true)}
            size="sm"
            variant="outline"
          >
            Dismiss
          </Button>
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
      <ConfirmDialog
        confirmLabel="Dismiss"
        description={`The reading held for "${name}" is deleted, it stops appearing here, and later reading in it is dropped. KOReader keeps its own record on the e-reader.`}
        isPending={dismiss.isPending}
        onConfirm={handleDismiss}
        onOpenChange={setConfirmOpen}
        open={confirmOpen}
        title="Dismiss Book"
      />
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
        description={`"${name}" stops syncing with this book and is listed to link again, on every one of your Devices. Its reading from then on is held until you link it. The reading sessions, status, and Rating the link added stay; delete any that are wrong from the book's page.`}
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
 * or dismiss it (ADR 0017). A Device can hold many, so each row is a few
 * lines, the first FIRST_SHOWN show until the rest are asked for, and the
 * linked ones fold away. It shows nothing while there are none.
 */
const KOReaderUnrecognizedBooks = ({ device }: { device: DeviceResponse }) => {
  const query = useKOReaderUnrecognizedBooks(device.id);
  const [showAll, setShowAll] = useState(false);
  const books = query.data ?? [];

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

  const toLink = books.filter((book) => !book.linked_at);
  const linked = books.filter((book) => book.linked_at);
  const shown = showAll ? toLink : toLink.slice(0, FIRST_SHOWN);

  return (
    <DeviceSection
      description="Books KOReader has reading data for that Shisho doesn't recognize, such as copies downloaded before Shisho v0.7.0 or from somewhere else. Link each to its book to bring its reading history in, or dismiss it."
      id="unrecognized-heading"
      title="Unrecognized Books"
    >
      {toLink.length > 0 && (
        <div className="space-y-2">
          <p className="text-sm font-medium">{toLink.length} to link</p>
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
              Show {toLink.length - shown.length} more
            </Button>
          )}
        </div>
      )}
      {linked.length > 0 && (
        <Disclosure
          defaultOpen={toLink.length === 0}
          size="sm"
          title={`Linked (${linked.length})`}
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
    </DeviceSection>
  );
};

export default KOReaderUnrecognizedBooks;
