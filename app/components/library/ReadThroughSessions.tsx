import { Trash2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  useDeleteReadingSession,
  useReadThroughSessions,
} from "@/hooks/queries/reading";
import { isDemoModeError, toastRequestError } from "@/libraries/api";
import type { ReadThroughSession } from "@/types";
import { formatDayTime, formatDuration, formatShortDay } from "@/utils/format";
import { dayIn } from "@/utils/readingCalendar";
import { sessionSourceLabel } from "@/utils/readingStats";

const percent = (value: number) => `${Math.round(value * 100)}%`;

// When a Session ran, from the day it started: "Jun 1, 7:00 AM to 8:00 AM".
const sessionWhen = (session: ReadThroughSession, timeZone: string) => {
  const day = dayIn(session.started_at, timeZone);
  return `${formatShortDay(day)}, ${formatDayTime(session.started_at, day, timeZone)} to ${formatDayTime(session.ended_at, day, timeZone)}`;
};

/**
 * Every Reading Session of one Read-through, oldest first, each with a
 * delete that asks first. `title` names the Book in the confirmation.
 * `disabled` hides nothing but turns the deletes off, as Demo Mode does.
 */
const ReadThroughSessions = ({
  readThroughId,
  title,
  disabled = false,
}: {
  readThroughId: number;
  title: string;
  disabled?: boolean;
}) => {
  const sessionsQuery = useReadThroughSessions(readThroughId);
  const data = sessionsQuery.data;
  const deleteMutation = useDeleteReadingSession();
  const [deleting, setDeleting] = useState<ReadThroughSession | null>(null);

  const handleDelete = async () => {
    if (!deleting) return;
    try {
      await deleteMutation.mutateAsync(deleting.id);
      toast.success("Reading session deleted");
      setDeleting(null);
    } catch (error) {
      // Demo Mode's refusal has been toasted and won't change on a retry.
      if (isDemoModeError(error)) setDeleting(null);
      toastRequestError(error, "Failed to delete reading session");
    }
  };

  if (sessionsQuery.error && !data) {
    return (
      <QueryError
        fallback="Failed to load reading sessions"
        query={sessionsQuery}
      />
    );
  }
  if (!data) {
    return <LoadingSpinner className="py-3" />;
  }
  if (data.items.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">No reading sessions.</p>
    );
  }
  return (
    <>
      <ul
        aria-label={`Sessions of ${title}`}
        className="divide-y divide-border rounded-md border border-border"
      >
        {data.items.map((session) => (
          <li
            className="flex items-center gap-2 py-1 pl-3 pr-1.5"
            key={session.id}
          >
            <p className="min-w-0 flex-1 text-xs text-muted-foreground">
              {[
                sessionWhen(session, data.time_zone),
                formatDuration(session.seconds),
                `${percent(session.start_percentage)} to ${percent(session.end_percentage)}`,
                sessionSourceLabel(session),
              ]
                .filter((part): part is string => part !== null)
                .join(" · ")}
            </p>
            <Button
              aria-label={`Delete session of ${title} on ${sessionWhen(session, data.time_zone)}`}
              className="shrink-0 text-muted-foreground hover:text-destructive"
              disabled={disabled}
              onClick={() => setDeleting(session)}
              size="icon-sm"
              variant="ghost"
            >
              <Trash2 />
            </Button>
          </li>
        ))}
      </ul>
      <ConfirmDialog
        confirmLabel="Delete"
        description={
          deleting
            ? `${title}, ${sessionWhen(deleting, data.time_zone)}. Its time stops counting toward your Reading Stats, and it can't be brought back.`
            : ""
        }
        isPending={deleteMutation.isPending}
        onConfirm={handleDelete}
        onOpenChange={(open) => !open && setDeleting(null)}
        open={!!deleting}
        title="Delete this reading session?"
        variant="destructive"
      />
    </>
  );
};

export default ReadThroughSessions;
