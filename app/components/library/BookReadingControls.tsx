import { BookOpen, ChevronDown, X } from "lucide-react";

import StarRating from "@/components/library/StarRating";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  useBookReading,
  useReadingTransition,
  useSetRating,
} from "@/hooks/queries/reading";
import { useAuth } from "@/hooks/useAuth";
import { DEMO_MODE_MESSAGE, toastRequestError } from "@/libraries/api";
import type { ReadingStatus } from "@/types";
import {
  readingStatusLabel,
  readingTransitions,
  type ReadingTransition,
} from "@/utils/readingStatus";

const transitionLabel = (
  transition: ReadingTransition,
  status: ReadingStatus | null | undefined,
): string => {
  switch (transition) {
    case "start":
      return status === "read" || status === "did_not_finish"
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
// menu offers only the transitions valid from the current status. In Demo
// Mode both render disabled with the demo's explanation.
const BookReadingControls = ({ bookId }: BookReadingControlsProps) => {
  const { demoMode } = useAuth();
  const readingQuery = useBookReading(bookId);
  const transitionMutation = useReadingTransition();
  const ratingMutation = useSetRating();

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
              <BookOpen aria-hidden className="h-4 w-4" />
              {statusLabel}
              <ChevronDown aria-hidden className="h-3.5 w-3.5 opacity-60" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            {readingTransitions(status).map((transition) => (
              <DropdownMenuItem
                key={transition}
                onSelect={() => handleTransition(transition)}
              >
                {transitionLabel(transition, status)}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>

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
      {demoMode && (
        <p className="text-xs text-muted-foreground">{DEMO_MODE_MESSAGE}</p>
      )}
    </div>
  );
};

export default BookReadingControls;
