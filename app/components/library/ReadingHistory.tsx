import { Pencil, Trash2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { EditReadThroughDialog } from "@/components/library/EditReadThroughDialog";
import QueryError from "@/components/library/QueryError";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { useBookReading, useDeleteReadThrough } from "@/hooks/queries/reading";
import { useAuth } from "@/hooks/useAuth";
import { toastRequestError } from "@/libraries/api";
import type { ReadThrough } from "@/types";
import { formatDate } from "@/utils/format";
import { readingSummary } from "@/utils/readingStatus";

const outcomeLabel = (rt: ReadThrough): string => {
  switch (rt.outcome) {
    case "underway":
      return rt.paused ? "Paused" : "Reading";
    case "finished":
      return "Finished";
    case "abandoned":
      return "Did Not Finish";
  }
};

const datesLabel = (rt: ReadThrough): string => {
  const begin = rt.begin_day ? formatDate(rt.begin_day) : null;
  const end = rt.end_day ? formatDate(rt.end_day) : null;
  if (rt.outcome === "underway") {
    return begin ? `Started ${begin}` : "Start date unknown";
  }
  if (begin && end) return `${begin} to ${end}`;
  if (end) return `Ended ${end}, start date unknown`;
  if (begin) return `Started ${begin}, end date unknown`;
  return "Dates unknown";
};

interface ReadingHistoryProps {
  bookId: number;
}

// The signed-in user's Read-throughs of a Book, newest first, with a
// one-line summary. Each can have its dates edited or be deleted.
const ReadingHistory = ({ bookId }: ReadingHistoryProps) => {
  const { demoMode } = useAuth();
  const readingQuery = useBookReading(bookId);
  const deleteMutation = useDeleteReadThrough();
  const [editing, setEditing] = useState<ReadThrough | null>(null);
  const [deleting, setDeleting] = useState<ReadThrough | null>(null);

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
        {readThroughs.map((rt) => (
          <li
            className="flex items-center justify-between gap-3 px-3 py-2"
            key={rt.id}
          >
            <div className="min-w-0">
              <p className="text-sm font-medium">{outcomeLabel(rt)}</p>
              <p className="text-xs text-muted-foreground">{datesLabel(rt)}</p>
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
          </li>
        ))}
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
        description={
          deleting?.outcome === "underway"
            ? "It won't count toward this book's history or its Reading Status. Every file opens at the start again, and if starting it took the book off To Read, it goes back to its place there."
            : "It won't count toward this book's history or its Reading Status."
        }
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
