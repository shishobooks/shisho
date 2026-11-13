import {
  BookCheck,
  BookX,
  ChevronDown,
  MoreHorizontal,
  Pencil,
  RotateCcw,
  Trash2,
} from "lucide-react";
import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import { toast } from "sonner";

import { EditReadThroughDialog } from "@/components/library/EditReadThroughDialog";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import PaginationFooter, {
  type PageChangeHandler,
} from "@/components/library/PaginationFooter";
import QueryError from "@/components/library/QueryError";
import ReadThroughCover from "@/components/library/ReadThroughCover";
import ReadThroughDetails from "@/components/library/ReadThroughDetails";
import ReadThroughSessions from "@/components/library/ReadThroughSessions";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ContentWidth } from "@/contexts/ContentWidth";
import {
  useDeleteReadThrough,
  useEndReadThrough,
  useReadingHistory,
  useReadingTransition,
} from "@/hooks/queries/reading";
import { useAuth } from "@/hooks/useAuth";
import { usePageTitle } from "@/hooks/usePageTitle";
import { DEMO_MODE_MESSAGE, toastRequestError } from "@/libraries/api";
import { parsePageParam, resetPaging } from "@/libraries/pagination";
import { cn } from "@/libraries/utils";
import type {
  ReadingHistoryEntry,
  ReadThrough,
  ReadThroughOutcome,
} from "@/types";
import { formatCount, formatShowingRange } from "@/utils/format";
import {
  readThroughDatesLabel,
  readThroughDeleteDescription,
  readThroughOutcomeLabel,
} from "@/utils/readingStatus";

const PER_PAGE = 50;

const OUTCOME_FILTERS: { value: ReadThroughOutcome | null; label: string }[] = [
  { value: null, label: "All" },
  { value: "finished", label: "Finished" },
  { value: "abandoned", label: "Did Not Finish" },
  { value: "underway", label: "Underway" },
];

const isOutcome = (value: string | null): value is ReadThroughOutcome =>
  value === "finished" || value === "abandoned" || value === "underway";

interface HistoryRowProps {
  entry: ReadingHistoryEntry;
  demoMode: boolean;
  onEdit: (rt: ReadThrough) => void;
  onDelete: (entry: ReadingHistoryEntry) => void;
}

const HistoryRow = ({ entry, demoMode, onEdit, onDelete }: HistoryRowProps) => {
  const rt = entry.read_through;
  const endMutation = useEndReadThrough();
  const transitionMutation = useReadingTransition();
  const underway = rt.outcome === "underway";
  const [showSessions, setShowSessions] = useState(false);
  const sessionsId = `read-through-${rt.id}-sessions`;

  const handleEnd = async (outcome: "finish" | "abandon") => {
    try {
      await endMutation.mutateAsync({ id: rt.id, outcome });
      toast.success(
        outcome === "finish"
          ? "Marked as finished"
          : "Marked as Did Not Finish",
      );
    } catch (error) {
      toastRequestError(error, "Failed to end Read-through");
    }
  };

  const handleReread = async () => {
    if (!entry.book) return;
    try {
      await transitionMutation.mutateAsync({
        bookId: entry.book.id,
        transition: "start",
      });
      toast.success("Reread started");
    } catch (error) {
      toastRequestError(error, "Failed to start a reread");
    }
  };

  return (
    <li className="px-3 py-2">
      <div className="flex items-center gap-3">
        <ReadThroughCover book={entry.book} />
        <ReadThroughDetails entry={entry}>
          <span className="font-medium">{readThroughOutcomeLabel(rt)}</span>
          <span className="text-muted-foreground">
            {readThroughDatesLabel(rt)}
          </span>
          {entry.session_count > 0 && (
            <Button
              aria-controls={showSessions ? sessionsId : undefined}
              aria-expanded={showSessions}
              className="gap-0.5 text-xs text-muted-foreground"
              onClick={() => setShowSessions((open) => !open)}
              variant="link"
            >
              {formatCount(entry.session_count, "session")}
              <ChevronDown
                className={cn(
                  "h-3 w-3 transition-transform",
                  showSessions && "rotate-180",
                )}
              />
            </Button>
          )}
        </ReadThroughDetails>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              aria-label={`Actions for ${entry.title}`}
              disabled={demoMode}
              size="icon-sm"
              variant="ghost"
            >
              <MoreHorizontal className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="end"
            onCloseAutoFocus={(e) => e.preventDefault()}
          >
            <DropdownMenuItem onSelect={() => onEdit(rt)}>
              <Pencil className="h-4 w-4" />
              Edit dates
            </DropdownMenuItem>
            {underway && (
              <>
                <DropdownMenuItem
                  disabled={endMutation.isPending}
                  onSelect={() => handleEnd("finish")}
                >
                  <BookCheck className="h-4 w-4" />
                  Mark as finished
                </DropdownMenuItem>
                <DropdownMenuItem
                  disabled={endMutation.isPending}
                  onSelect={() => handleEnd("abandon")}
                >
                  <BookX className="h-4 w-4" />
                  Mark as Did Not Finish
                </DropdownMenuItem>
              </>
            )}
            {entry.can_reread && (
              <DropdownMenuItem
                disabled={transitionMutation.isPending}
                onSelect={handleReread}
              >
                <RotateCcw className="h-4 w-4" />
                Start a reread
              </DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem
              className="text-destructive focus:text-destructive"
              onSelect={() => onDelete(entry)}
            >
              <Trash2 className="h-4 w-4" />
              Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {showSessions && (
        // Lined up under the title, past the cover.
        <div className="mt-2 sm:pl-14" id={sessionsId}>
          <ReadThroughSessions
            disabled={demoMode}
            readThroughId={rt.id}
            title={entry.title}
          />
        </div>
      )}
    </li>
  );
};

const deleteDescription = (entry: ReadingHistoryEntry | null): string =>
  entry
    ? readThroughDeleteDescription({
        underway: entry.read_through.outcome === "underway",
        removed: entry.removed,
      })
    : "";

// Every Read-through the signed-in user has, across every library and
// including ones whose Book was deleted, newest first. Each can have its
// dates edited or be deleted; one underway can be ended here.
const ReadingHistoryPage = () => {
  usePageTitle("Reading History");
  const { demoMode } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parsePageParam(searchParams.get("page"));
  const outcomeParam = searchParams.get("outcome");
  const outcome = isOutcome(outcomeParam) ? outcomeParam : null;
  const removed = searchParams.get("removed") === "true";
  const offset = (currentPage - 1) * PER_PAGE;

  const historyQuery = useReadingHistory({
    limit: PER_PAGE,
    offset,
    ...(outcome ? { outcome } : {}),
    ...(removed ? { removed: true } : {}),
  });
  const deleteMutation = useDeleteReadThrough();
  const [editing, setEditing] = useState<ReadThrough | null>(null);
  const [deleting, setDeleting] = useState<ReadingHistoryEntry | null>(null);

  const total = historyQuery.data?.total ?? 0;
  const items = historyQuery.data?.items ?? [];
  const filtered = outcome !== null || removed;

  const updateParams = (change: (params: URLSearchParams) => void) => {
    const params = new URLSearchParams(searchParams);
    change(params);
    resetPaging(params);
    setSearchParams(params, { preventScrollReset: true });
  };

  const setOutcome = (value: ReadThroughOutcome | null) =>
    updateParams((params) => {
      if (value) params.set("outcome", value);
      else params.delete("outcome");
    });

  const toggleRemoved = () =>
    updateParams((params) => {
      if (removed) params.delete("removed");
      else params.set("removed", "true");
    });

  const handlePageChange: PageChangeHandler = (page, options) => {
    const params = new URLSearchParams(searchParams);
    params.set("page", page.toString());
    setSearchParams(params, options);
  };

  const handleDelete = async () => {
    if (!deleting) return;
    try {
      await deleteMutation.mutateAsync(deleting.read_through.id);
      toast.success("Read-through deleted");
      setDeleting(null);
    } catch (error) {
      toastRequestError(error, "Failed to delete Read-through");
    }
  };

  const renderList = () => {
    if (historyQuery.error && !historyQuery.data) {
      return (
        <QueryError
          fallback="Failed to load Reading History"
          query={historyQuery}
        />
      );
    }
    if (!historyQuery.data) {
      return <LoadingSpinner />;
    }
    if (total === 0) {
      return (
        <div className="rounded-md border border-border p-8 text-center text-muted-foreground">
          {filtered
            ? "No Read-throughs match these filters."
            : "Nothing here yet. Books you start, finish, or stop reading show up here."}
        </div>
      );
    }
    const showing = formatShowingRange(
      offset,
      PER_PAGE,
      total,
      "Read-through",
      "Read-throughs",
    );
    return (
      <>
        {showing && (
          <div className="mb-4 text-sm text-muted-foreground">{showing}</div>
        )}
        <ul className="divide-y divide-border rounded-md border border-border">
          {items.map((entry) => (
            <HistoryRow
              demoMode={demoMode}
              entry={entry}
              key={entry.read_through.id}
              onDelete={setDeleting}
              onEdit={setEditing}
            />
          ))}
        </ul>
        <PaginationFooter
          className="mt-6"
          currentPage={currentPage}
          onPageChange={handlePageChange}
          totalIsCurrent={!historyQuery.isPlaceholderData}
          totalPages={Math.ceil(total / PER_PAGE)}
        />
      </>
    );
  };

  return (
    <div>
      <TopNav />
      <ContentWidth className="px-4 py-4 md:px-6 md:py-8">
        <div className="mb-6">
          <h1 className="mb-2 text-2xl font-semibold">Reading History</h1>
          <p className="mb-2 text-sm text-muted-foreground md:text-base">
            Every time you've read a book, including books since removed from
            the library. Fix dates here, or delete a read that shouldn't count.
          </p>
          {historyQuery.data && (
            <Badge variant="secondary">
              {formatCount(total, "Read-through")}
            </Badge>
          )}
          {demoMode && (
            <p className="mt-2 text-xs text-muted-foreground">
              {DEMO_MODE_MESSAGE}
            </p>
          )}
        </div>

        <div className="mb-4 flex flex-wrap items-center gap-2">
          <div
            aria-labelledby="reading-history-outcome-label"
            className="flex flex-wrap gap-1"
            role="group"
          >
            <span className="sr-only" id="reading-history-outcome-label">
              Outcome
            </span>
            {OUTCOME_FILTERS.map((filter) => {
              const active = outcome === filter.value;
              return (
                <Button
                  aria-pressed={active}
                  className={cn(active && "border-primary text-primary")}
                  key={filter.label}
                  onClick={() => setOutcome(filter.value)}
                  size="sm"
                  variant="outline"
                >
                  {filter.label}
                </Button>
              );
            })}
          </div>
          <Button
            aria-pressed={removed}
            className={cn(removed && "border-primary text-primary")}
            onClick={toggleRemoved}
            size="sm"
            variant="outline"
          >
            Removed from library
          </Button>
        </div>

        {renderList()}
      </ContentWidth>

      {editing && (
        <EditReadThroughDialog
          onOpenChange={(open) => !open && setEditing(null)}
          open
          readThrough={editing}
        />
      )}
      <ConfirmDialog
        confirmLabel="Delete"
        description={deleteDescription(deleting)}
        isPending={deleteMutation.isPending}
        onConfirm={handleDelete}
        onOpenChange={(open) => !open && setDeleting(null)}
        open={!!deleting}
        title="Delete this Read-through?"
        variant="destructive"
      />
    </div>
  );
};

export default ReadingHistoryPage;
