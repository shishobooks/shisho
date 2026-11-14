import { Trash2 } from "lucide-react";
import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { useAuth } from "@/hooks/useAuth";
import { cn } from "@/libraries/utils";
import type { ReadingStatsDaySession, ReadThroughSession } from "@/types";
import { formatDuration } from "@/utils/format";
import { sessionSourceLabel, withEstimate } from "@/utils/readingStats";
import { formatProgress } from "@/utils/readingStatus";

type SessionFields =
  | "seconds"
  | "start_percentage"
  | "end_percentage"
  | "kind"
  | "source"
  | "measured"
  | "device_id"
  | "device_name";

/** What a session's line shows, from either of the APIs that list them. */
export type SessionLine = Pick<ReadThroughSession, SessionFields> &
  Pick<ReadingStatsDaySession, SessionFields>;

// progressLine is how far a session took the reader, or where it stayed
// when it went nowhere, as opening a book and closing it again does.
const progressLine = (start: number, end: number): string => {
  const from = formatProgress(start);
  const to = formatProgress(end);
  return from === to ? `at ${from}` : `${from} to ${to}`;
};

// SessionSource names where a session was read: the Device it came from,
// linking to its page, or the reader when it came from none.
const SessionSource = ({ session }: { session: SessionLine }) => {
  if (!session.device_id || !session.device_name) {
    return <>{sessionSourceLabel(session)}</>;
  }
  return (
    <>
      <Link
        className="underline underline-offset-2 hover:text-foreground"
        to={`/user/devices/${session.device_id}`}
      >
        {session.device_name}
      </Link>
      {withEstimate("", session.measured)}
    </>
  );
};

/**
 * One Reading Session as a line of a list: `when` it ran, how long, how far
 * it took the reader, and where, with a delete. `title` names its Book for
 * the delete's accessible name, which `when` keeps apart from the Book's
 * other sessions. Demo Mode disables the delete, as it does the other
 * reading controls.
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
}) => {
  const { demoMode } = useAuth();
  return (
    <li className={cn("flex items-center gap-2 py-1 pr-1.5", className)}>
      <p className="min-w-0 flex-1 text-xs text-muted-foreground">
        {[
          when,
          formatDuration(session.seconds),
          progressLine(session.start_percentage, session.end_percentage),
        ].join(" · ")}
        {" · "}
        <SessionSource session={session} />
      </p>
      <Button
        aria-label={`Delete session of ${title}, ${when}`}
        className="shrink-0 text-muted-foreground hover:text-destructive"
        disabled={demoMode}
        onClick={onDelete}
        size="icon-sm"
        variant="ghost"
      >
        <Trash2 />
      </Button>
    </li>
  );
};

export default SessionRow;
