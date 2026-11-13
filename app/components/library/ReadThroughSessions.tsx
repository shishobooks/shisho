import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import SessionRow from "@/components/library/SessionRow";
import { useReadThroughSessions } from "@/hooks/queries/reading";
import { useSessionDelete } from "@/hooks/useSessionDelete";
import type { ReadThroughSession } from "@/types";
import { formatDayTime, formatShortDay } from "@/utils/format";
import { dayIn } from "@/utils/readingCalendar";

// When a Session ran, from the day it started: "Jun 1, 7:00 AM to 8:00 AM".
const sessionWhen = (session: ReadThroughSession, timeZone: string) => {
  const day = dayIn(session.started_at, timeZone);
  return `${formatShortDay(day)}, ${formatDayTime(session.started_at, day, timeZone)} to ${formatDayTime(session.ended_at, day, timeZone)}`;
};

/**
 * Every Reading Session of one Read-through, oldest first, each with a
 * delete that asks first. `title` names the Book in the confirmation.
 */
const ReadThroughSessions = ({
  readThroughId,
  title,
}: {
  readThroughId: number;
  title: string;
}) => {
  const sessionsQuery = useReadThroughSessions(readThroughId);
  const data = sessionsQuery.data;
  const sessionDelete = useSessionDelete();

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
        {data.items.map((session) => {
          const when = sessionWhen(session, data.time_zone);
          return (
            <SessionRow
              className="pl-3"
              key={session.id}
              onDelete={() =>
                sessionDelete.requestDelete(session.id, `${title}, ${when}`)
              }
              session={session}
              title={title}
              when={when}
            />
          );
        })}
      </ul>
      {sessionDelete.dialog}
    </>
  );
};

export default ReadThroughSessions;
