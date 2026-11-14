import { Link as LinkIcon, Loader2, Search } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import DeviceSection from "@/components/devices/DeviceSection";
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
const CandidateList = ({
  candidates,
  label,
  linking,
  onLink,
}: {
  candidates: LinkCandidate[];
  label: string;
  linking: boolean;
  onLink: (candidate: LinkCandidate) => void;
}) => (
  <ul aria-label={label} className="space-y-2">
    {candidates.map((candidate) => (
      <li
        className="flex items-center justify-between gap-3 rounded-md bg-muted/50 px-3 py-2"
        key={candidate.file_id}
      >
        <div className="min-w-0 text-sm">
          <p className="flex flex-wrap items-center gap-2 font-medium">
            <span className="break-words">{candidate.title}</span>
            {candidate.match === LinkMatchIdentifier && (
              <Badge variant="secondary">Same identifier</Badge>
            )}
          </p>
          <p className="break-words text-muted-foreground">
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

// ToLink is a copy still to link: its suggestions, a search, and Dismiss.
const ToLink = ({
  deviceId,
  book,
}: {
  deviceId: string;
  book: UnrecognizedBook;
}) => {
  const name = copyName(book);
  const link = useLinkKOReaderUnrecognized();
  const dismiss = useDismissKOReaderUnrecognized();
  const [confirmOpen, setConfirmOpen] = useState(false);

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
    <>
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
      <ConfirmDialog
        confirmLabel="Dismiss"
        description={`The reading held for "${name}" is deleted, it stops appearing here, and later reading in it is dropped. KOReader keeps its own record on the e-reader.`}
        isPending={dismiss.isPending}
        onConfirm={handleDismiss}
        onOpenChange={setConfirmOpen}
        open={confirmOpen}
        title="Dismiss Book"
      />
    </>
  );
};

// Linked is a copy already linked: the Book it's linked to, why downloading
// the Book again syncs the exact place (ADR 0017), and Unlink for a link
// made to the wrong Book.
const Linked = ({
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
    <>
      {book.linked ? (
        <div className="space-y-1 text-sm">
          <p>
            Linked to{" "}
            <Link
              className="font-medium underline underline-offset-2 hover:text-foreground"
              to={bookPath(book.linked)}
            >
              {book.linked.title}
            </Link>
            <span className="text-muted-foreground">
              {" "}
              · {fileTypeLabel(book.linked.file_type)} ·{" "}
              {book.linked.library_name}
            </span>
          </p>
          <p className="text-muted-foreground">
            This copy syncs your place as a percentage only, so it can land a
            little off. Download the book again from Shisho to sync your exact
            place.
          </p>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">
          Linked to a file that has since been deleted.
        </p>
      )}
      <Button
        aria-label={`Unlink ${name}`}
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
    </>
  );
};

const UnrecognizedBookRow = ({
  deviceId,
  book,
}: {
  deviceId: string;
  book: UnrecognizedBook;
}) => {
  const name = copyName(book);
  const lastRead = formatLastRead(book.last_read_at);
  const facts = [
    book.reading_seconds > 0
      ? `${formatDuration(book.reading_seconds)} read in KOReader`
      : null,
    lastRead,
  ].filter(Boolean);

  return (
    <li
      aria-label={name}
      className="space-y-3 border-t border-border pt-4 first:border-t-0 first:pt-0"
    >
      <div className="min-w-0">
        <p className="break-words font-medium">{name}</p>
        {book.authors.length > 0 && (
          <p className="break-words text-sm text-muted-foreground">
            {book.authors.join(", ")}
          </p>
        )}
        {facts.length > 0 && (
          <p className="text-sm text-muted-foreground">{facts.join(" · ")}</p>
        )}
      </div>
      {book.linked_at ? (
        <Linked book={book} deviceId={deviceId} />
      ) : (
        <ToLink book={book} deviceId={deviceId} />
      )}
    </li>
  );
};

/**
 * KOReaderUnrecognizedBooks lists the copies on a KOReader Device that no
 * server the KOReader plugin signed in to recognizes, such as ones downloaded
 * before Shisho recorded what it sent, so the user can link each to its Book
 * or dismiss it (ADR 0017). It shows nothing while there are none.
 */
const KOReaderUnrecognizedBooks = ({ device }: { device: DeviceResponse }) => {
  const query = useKOReaderUnrecognizedBooks(device.id);
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

  return (
    <DeviceSection
      description="Books KOReader has reading data for that Shisho doesn't recognize, such as copies downloaded before Shisho v0.7.0 or from somewhere else. Link each to its book to bring its reading history in, or dismiss it."
      id="unrecognized-heading"
      title="Unrecognized Books"
    >
      <ul aria-label="Unrecognized books" className="space-y-4">
        {books.map((book) => (
          <UnrecognizedBookRow
            book={book}
            deviceId={device.id}
            key={book.document}
          />
        ))}
      </ul>
    </DeviceSection>
  );
};

export default KOReaderUnrecognizedBooks;
