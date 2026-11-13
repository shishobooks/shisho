import { Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/libraries/utils";
import type { ReadingKind, ReadingSource } from "@/types";
import { formatDuration } from "@/utils/format";
import { sessionSourceLabel } from "@/utils/readingStats";
import { formatProgress } from "@/utils/readingStatus";

/** What a session's line shows, from either of the APIs that list them. */
export interface SessionLine {
  seconds: number;
  start_percentage: number;
  end_percentage: number;
  kind: ReadingKind;
  source: ReadingSource;
  measured: boolean;
}

/**
 * One Reading Session as a line of a list: `when` it ran, how long, how far
 * it took the reader, and where, with a delete. `title` names its Book for
 * the delete's accessible name, which `when` keeps apart from the Book's
 * other sessions.
 */
const SessionRow = ({
  session,
  when,
  title,
  onDelete,
  className,
}: {
  session: SessionLine;
  when: string;
  title: string;
  onDelete: () => void;
  className?: string;
}) => (
  <li className={cn("flex items-center gap-2 py-1 pr-1.5", className)}>
    <p className="min-w-0 flex-1 text-xs text-muted-foreground">
      {[
        when,
        formatDuration(session.seconds),
        `${formatProgress(session.start_percentage)} to ${formatProgress(session.end_percentage)}`,
        sessionSourceLabel(session),
      ].join(" · ")}
    </p>
    <Button
      aria-label={`Delete session of ${title}, ${when}`}
      className="shrink-0 text-muted-foreground hover:text-destructive"
      onClick={onDelete}
      size="icon-sm"
      variant="ghost"
    >
      <Trash2 />
    </Button>
  </li>
);

export default SessionRow;
