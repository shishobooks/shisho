import type { ReadingStatus, ReadThrough } from "@/types";
import { formatMonthYear, localDay } from "@/utils/format";

// A reading transition, named by its route (pkg/reading/routes.go).
export type ReadingTransition =
  "start" | "pause" | "resume" | "finish" | "abandon";

// Today as YYYY-MM-DD in timeZone, the user's time zone from Preferences,
// or the browser's local day while it's unset. A Read-through's days are
// days in the user's time zone, which also dates automatic starts and
// finishes, so the two agree.
export const todayDay = (timeZone?: string | null): string => {
  if (!timeZone) return localDay(new Date());
  try {
    // en-CA formats a date as YYYY-MM-DD.
    return new Intl.DateTimeFormat("en-CA", {
      timeZone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    }).format(new Date());
  } catch {
    return localDay(new Date());
  }
};

const STATUS_LABELS: Record<ReadingStatus, string> = {
  currently_reading: "Currently Reading",
  paused: "Paused",
  to_read: "To Read",
  read: "Read",
  did_not_finish: "Did Not Finish",
};

export const readingStatusLabel = (status: ReadingStatus): string =>
  STATUS_LABELS[status];

// The transitions the server accepts from status, which is all a status
// control offers. Starting is "Read again" once the Book has been read.
export const readingTransitions = (
  status: ReadingStatus | null | undefined,
): ReadingTransition[] => {
  switch (status) {
    case "currently_reading":
      return ["pause", "finish", "abandon"];
    case "paused":
      return ["resume", "finish", "abandon"];
    default:
      return ["start", "finish"];
  }
};

// Whether a Book with status can join or leave To Read. A Book being read
// can't: starting takes a Book off To Read. Elsewhere a Book is on To Read
// exactly when its status is To Read.
export const canQueueToRead = (
  status: ReadingStatus | null | undefined,
): boolean => status !== "currently_reading" && status !== "paused";

const times = (count: number): string => {
  if (count === 1) return "once";
  if (count === 2) return "twice";
  return `${count.toLocaleString()} times`;
};

// A one-line history such as "Read twice, last finished Mar 2024", or null
// when nothing has ended.
export const readingSummary = (readThroughs: ReadThrough[]): string | null => {
  const finished = readThroughs.filter((rt) => rt.outcome === "finished");
  const abandoned = readThroughs.filter((rt) => rt.outcome === "abandoned");
  const parts: string[] = [];
  if (finished.length > 0) {
    let part = `Read ${times(finished.length)}`;
    const days = finished
      .map((rt) => rt.end_day)
      .filter((day): day is string => !!day)
      .sort();
    const lastDay = days[days.length - 1];
    if (lastDay) part += `, last finished ${formatMonthYear(lastDay)}`;
    parts.push(part);
  }
  if (abandoned.length > 0) {
    parts.push(`Didn't finish ${times(abandoned.length)}`);
  }
  return parts.length > 0 ? parts.join(" · ") : null;
};
