import {
  differenceInCalendarDays,
  differenceInMinutes,
  formatDistanceStrict,
} from "date-fns";

import { formatDaysAgo } from "@/utils/format";

// "Last read 4 hours ago" until 24 hours have passed, rounded down, then "Last
// read yesterday" and on by the day, or null with no reading activity. A clock
// running ahead of the browser's reads as just now.
export const formatLastRead = (
  lastReadAt: string | null | undefined,
  now = new Date(),
): string | null => {
  if (!lastReadAt) return null;
  const instant = new Date(lastReadAt);
  const minutes = differenceInMinutes(now, instant);
  if (minutes < 1) return "Last read just now";
  if (minutes < 24 * 60) {
    const ago = formatDistanceStrict(instant, now, {
      addSuffix: true,
      roundingMethod: "floor",
    });
    return `Last read ${ago}`;
  }
  // A day that daylight saving made longer can still be today on the calendar.
  if (differenceInCalendarDays(now, instant) < 1) return "Last read yesterday";
  return `Last read ${formatDaysAgo(lastReadAt, now)}`;
};
