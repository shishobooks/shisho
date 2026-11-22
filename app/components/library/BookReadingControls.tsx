import { ChevronDown, X } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import LastRead from "@/components/library/LastRead";
import ReadingStatusIcon from "@/components/library/ReadingStatusIcon";
import StarRating from "@/components/library/StarRating";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  useBookReading,
  useDeleteReadThrough,
  useReadingTransition,
  useSetOnToRead,
  useSetRating,
  useSetReference,
} from "@/hooks/queries/reading";
import { useAuth } from "@/hooks/useAuth";
import { DEMO_MODE_MESSAGE, toastRequestError } from "@/libraries/api";
import type { ReadingStatus } from "@/types";
import {
  canQueueToRead,
  readingStatusLabel,
  readingTransitions,
  type ReadingTransition,
} from "@/utils/readingStatus";

const transitionLabel = (
  transition: ReadingTransition,
  status: ReadingStatus | null | undefined,
  readBefore: boolean,
): string => {
  switch (transition) {
    case "start":
      return status === "read" ||
        status === "did_not_finish" ||
        (status === "reference" && readBefore)
        ? "Read again"
        : "Start reading";
    case "pause":
      return "Pause";
    case "resume":
      return "Resume";
    case "finish":
      return "Mark as read";
    case "abandon":
      return "Mark as Did Not Finish";
  }
};

const transitionFailure: Record<ReadingTransition, string> = {
  start: "Failed to start reading",
  pause: "Failed to pause reading",
  resume: "Failed to resume reading",
  finish: "Failed to mark as read",
  abandon: "Failed to mark as Did Not Finish",
};

interface BookReadingControlsProps {
  bookId: number;
}

// The signed-in user's Reading Status and Rating for a Book. The status
// menu offers only the transitions valid from the current status, plus
// adding to or removing from To Read while the Book isn't being read, and
// marking it Reference or removing the mark. In Demo Mode both render
// disabled with the demo's explanation.
const BookReadingControls = ({ bookId }: BookReadingControlsProps) => {
  const { demoMode } = useAuth();
  const readingQuery = useBookReading(bookId);
  const transitionMutation = useReadingTransition();
  const ratingMutation = useSetRating();
  const toReadMutation = useSetOnToRead();
  const deleteMutation = useDeleteReadThrough();
  const referenceMutation = useSetReference();
  const [confirmingNotStarted, setConfirmingNotStarted] = useState(false);
  const [confirmingReference, setConfirmingReference] = useState(false);

  // A failed load leaves the controls out; the history section reports it.
  const reading = readingQuery.data;
  if (!reading) return null;

  const status = reading.status;
  const rating = reading.rating ?? null;
  const statusLabel = status ? readingStatusLabel(status) : "Track reading";

  const handleTransition = async (transition: ReadingTransition) => {
    try {
      await transitionMutation.mutateAsync({ bookId, transition });
    } catch (error) {
      toastRequestError(error, transitionFailure[transition]);
    }
  };

  const handleToRead = async (onToRead: boolean) => {
    try {
      await toReadMutation.mutateAsync({ bookId, onToRead });
    } catch (error) {
      toastRequestError(
        error,
        onToRead ? "Failed to add to To Read" : "Failed to remove from To Read",
      );
    }
  };

  // Taking back a start deletes the Read-through underway, which the server
  // undoes completely: positions, history, and the place on To Read.
  const underway = (reading.read_throughs ?? []).find(
    (rt) => rt.outcome === "underway",
  );
  const handleNotStarted = async () => {
    if (!underway) return;
    try {
      await deleteMutation.mutateAsync(underway.id);
      toast.success("Marked as not started");
      setConfirmingNotStarted(false);
    } catch (error) {
      toastRequestError(error, "Failed to mark as not started");
    }
  };

  const handleReference = async (reference: boolean) => {
    try {
      await referenceMutation.mutateAsync({ bookId, reference });
      setConfirmingReference(false);
    } catch (error) {
      toastRequestError(
        error,
        reference
          ? "Failed to mark as Reference"
          : "Failed to remove from Reference",
      );
    }
  };

  // Marking Reference deletes the Read-through underway, so it asks first;
  // with nothing underway it just applies.
  const handleMarkReference = () => {
    if (underway) {
      setConfirmingReference(true);
      return;
    }
    handleReference(true);
  };
  const readBefore = (reading.read_throughs ?? []).some(
    (rt) => rt.outcome !== "underway",
  );

  const handleRating = async (stars: number | null) => {
    try {
      await ratingMutation.mutateAsync({ bookId, payload: { rating: stars } });
    } catch (error) {
      toastRequestError(error, "Failed to save rating");
    }
  };

  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              aria-label={`Reading status: ${statusLabel}`}
              disabled={demoMode}
              size="sm"
              variant="outline"
            >
              {/* The status's own icon, as on its cover bookmark. */}
              <ReadingStatusIcon
                aria-hidden
                className="h-4 w-4"
                status={status}
              />
              {statusLabel}
              <ChevronDown aria-hidden className="h-3.5 w-3.5 opacity-60" />
            </Button>
          </DropdownMenuTrigger>
          {/* The confirmation takes focus from the closing menu. */}
          <DropdownMenuContent
            align="start"
            onCloseAutoFocus={(e) => e.preventDefault()}
          >
            {/* Queueing a book usually comes before reading it. */}
            {canQueueToRead(status) && (
              <>
                <DropdownMenuItem
                  onSelect={() => handleToRead(!reading.on_to_read)}
                >
                  {reading.on_to_read
                    ? "Remove from To Read"
                    : "Add to To Read"}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
              </>
            )}
            {readingTransitions(status).map((transition) => (
              <DropdownMenuItem
                key={transition}
                onSelect={() => handleTransition(transition)}
              >
                {transitionLabel(transition, status, readBefore)}
              </DropdownMenuItem>
            ))}
            <DropdownMenuSeparator />
            {status === "reference" ? (
              <DropdownMenuItem onSelect={() => handleReference(false)}>
                Remove from Reference
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem onSelect={handleMarkReference}>
                Mark as Reference
              </DropdownMenuItem>
            )}
            {underway && (
              <DropdownMenuItem onSelect={() => setConfirmingNotStarted(true)}>
                Mark as not started
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>

        {reading.to_read_place != null && (
          <Link
            className="text-sm text-muted-foreground hover:text-foreground hover:underline"
            to="/to-read"
          >
            #{reading.to_read_place.toLocaleString()} on To Read
          </Link>
        )}

        <div className="flex items-center gap-1">
          {/* Not disabled while a save is pending: a disabled slider
              drops keyboard focus after every arrow key. */}
          <StarRating
            disabled={demoMode}
            onChange={handleRating}
            value={rating}
          />
          {rating !== null && (
            <Button
              aria-label="Clear rating"
              disabled={demoMode}
              onClick={() => handleRating(null)}
              size="icon-sm"
              variant="ghost"
            >
              <X className="h-3.5 w-3.5" />
            </Button>
          )}
        </div>
      </div>
      {reading.last_read_at && (
        <p className="text-xs text-muted-foreground">
          <LastRead lastReadAt={reading.last_read_at} />
        </p>
      )}
      {demoMode && (
        <p className="text-xs text-muted-foreground">{DEMO_MODE_MESSAGE}</p>
      )}
      <ConfirmDialog
        confirmLabel="Mark as not started"
        description="Your current Read-through is deleted along with the reading time and places recorded during it, and every main file opens at the start again. If starting it took the book off To Read, it goes back to its place there."
        isPending={deleteMutation.isPending}
        onConfirm={handleNotStarted}
        onOpenChange={setConfirmingNotStarted}
        open={confirmingNotStarted}
        title="Mark as not started?"
      />
      <ConfirmDialog
        confirmLabel="Mark as Reference"
        description="Your current Read-through of this book is deleted from your reading history, and this can't be undone. The reading time recorded during it still counts toward your Reading Stats, and every file keeps its place. While the book is Reference, opening it never starts or finishes a Read-through."
        isPending={referenceMutation.isPending}
        onConfirm={() => handleReference(true)}
        onOpenChange={setConfirmingReference}
        open={confirmingReference}
        title="Mark as Reference?"
      />
    </div>
  );
};

export default BookReadingControls;
