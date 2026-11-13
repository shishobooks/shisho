import { ChevronDown, Pencil, Trash2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { EditReadThroughDialog } from "@/components/library/EditReadThroughDialog";
import QueryError from "@/components/library/QueryError";
import ReadThroughSessions from "@/components/library/ReadThroughSessions";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { useBookReading, useDeleteReadThrough } from "@/hooks/queries/reading";
import { useAuth } from "@/hooks/useAuth";
import { toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import type { ReadThrough } from "@/types";
import { formatCount } from "@/utils/format";
import {
  readingSummary,
  readThroughDatesLabel,
  readThroughDeleteDescription,
  readThroughOutcomeLabel,
} from "@/utils/readingStatus";

interface ReadingHistoryProps {
  bookId: number;
  /** The Book's title, which names it when deleting a session. */
  title: string;
}

// The signed-in user's Read-throughs of a Book, newest first, with a
// one-line summary. Each can have its dates edited or be deleted, and its
// Reading Sessions listed and deleted.
const ReadingHistory = ({ bookId, title }: ReadingHistoryProps) => {
  const { demoMode } = useAuth();
  const readingQuery = useBookReading(bookId);
  const deleteMutation = useDeleteReadThrough();
  const [editing, setEditing] = useState<ReadThrough | null>(null);
  const [deleting, setDeleting] = useState<ReadThrough | null>(null);
  const [showingSessions, setShowingSessions] = useState<number | null>(null);

  if (readingQuery.error && !readingQuery.data) {
    return (
      <section className="mt-10">
        <QueryError
          fallback="Failed to load reading history"
          query={readingQuery}
        />
      </section>
    );
  }
  const readThroughs = readingQuery.data?.read_throughs ?? [];
  if (readThroughs.length === 0) return null;

  const summary = readingSummary(readThroughs);

  const handleDelete = async () => {
    if (!deleting) return;
    try {
      await deleteMutation.mutateAsync(deleting.id);
      toast.success("Read-through deleted");
      setDeleting(null);
    } catch (error) {
      toastRequestError(error, "Failed to delete Read-through");
    }
  };

  return (
    <section aria-labelledby="book-reading-heading" className="mt-10">
      <h2
        className="mb-1 text-sm text-muted-foreground"
        id="book-reading-heading"
      >
        Reading history
      </h2>
      {summary && <p className="mb-3 text-sm">{summary}</p>}
      <ul className="divide-y divide-border rounded-md border border-border">
        {readThroughs.map((rt) => {
          const sessionCount = readingQuery.data?.session_counts[rt.id] ?? 0;
          const open = showingSessions === rt.id;
          const sessionsId = `book-read-through-${rt.id}-sessions`;
          return (
            <li className="px-3 py-2" key={rt.id}>
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <p className="text-sm font-medium">
                    {readThroughOutcomeLabel(rt)}
                  </p>
                  <p className="flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
                    {readThroughDatesLabel(rt)}
                    {sessionCount > 0 && (
                      <Button
                        aria-controls={open ? sessionsId : undefined}
                        aria-expanded={open}
                        className="gap-0.5 text-xs text-muted-foreground"
                        onClick={() => setShowingSessions(open ? null : rt.id)}
                        variant="link"
                      >
                        {formatCount(sessionCount, "session")}
                        <ChevronDown
                          className={cn(
                            "h-3 w-3 transition-transform",
                            open && "rotate-180",
                          )}
                        />
                      </Button>
                    )}
                  </p>
                </div>
                <div className="flex shrink-0 gap-1">
                  <Button
                    aria-label="Edit dates"
                    disabled={demoMode}
                    onClick={() => setEditing(rt)}
                    size="icon-sm"
                    variant="ghost"
                  >
                    <Pencil className="h-4 w-4" />
                  </Button>
                  <Button
                    aria-label="Delete Read-through"
                    disabled={demoMode}
                    onClick={() => setDeleting(rt)}
                    size="icon-sm"
                    variant="ghost"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </div>
              {open && (
                <div className="mt-2" id={sessionsId}>
                  <ReadThroughSessions
                    disabled={demoMode}
                    readThroughId={rt.id}
                    title={title}
                  />
                </div>
              )}
            </li>
          );
        })}
      </ul>

      {editing && (
        <EditReadThroughDialog
          onOpenChange={(open) => !open && setEditing(null)}
          open
          readThrough={editing}
        />
      )}
      <ConfirmDialog
        confirmLabel="Delete"
        description={readThroughDeleteDescription({
          underway: deleting?.outcome === "underway",
          removed: false,
        })}
        isPending={deleteMutation.isPending}
        onConfirm={handleDelete}
        onOpenChange={(open) => !open && setDeleting(null)}
        open={!!deleting}
        title="Delete this Read-through?"
        variant="destructive"
      />
    </section>
  );
};

export default ReadingHistory;
