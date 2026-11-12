import type { ReadingStatsCalendarDay } from "@/types";

/**
 * Colours a Book's bars take on the Reading Stats calendar. Each is dark
 * enough for white text on it, on either theme.
 */
export const BOOK_COLORS = [
  "oklch(0.5 0.15 25)",
  "oklch(0.5 0.12 60)",
  "oklch(0.5 0.12 145)",
  "oklch(0.5 0.1 185)",
  "oklch(0.5 0.13 230)",
  "oklch(0.5 0.17 265)",
  "oklch(0.5 0.18 300)",
  "oklch(0.5 0.17 340)",
  "oklch(0.45 0.08 100)",
  "oklch(0.45 0.05 250)",
] as const;

/**
 * A Book's colour, from its calendar key (FNV-1a), so it's the same in
 * every month.
 */
export const bookColor = (key: string): string => {
  let hash = 0x811c9dc5;
  for (let i = 0; i < key.length; i++) {
    hash ^= key.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }
  return BOOK_COLORS[(hash >>> 0) % BOOK_COLORS.length];
};

const toDay = (date: Date) => date.toISOString().slice(0, 10);

const parseDay = (day: string) => {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d));
};

/** The calendar day `by` days after `day` (YYYY-MM-DD). */
export const shiftDay = (day: string, by: number): string => {
  const date = parseDay(day);
  date.setUTCDate(date.getUTCDate() + by);
  return toDay(date);
};

/** A week, Sunday first, with null for days outside the month or year. */
export type Week = (string | null)[];

// The weeks from the Sunday on or before `first` to the Saturday on or
// after `last`, blank outside them.
const weeksBetween = (first: Date, last: Date): Week[] => {
  const weeks: Week[] = [];
  const cursor = new Date(first);
  cursor.setUTCDate(cursor.getUTCDate() - cursor.getUTCDay());
  while (cursor <= last) {
    const week: Week = [];
    for (let i = 0; i < 7; i++) {
      week.push(cursor >= first && cursor <= last ? toDay(cursor) : null);
      cursor.setUTCDate(cursor.getUTCDate() + 1);
    }
    weeks.push(week);
  }
  return weeks;
};

/** The weeks of a month (1 to 12), Sunday first. */
export const monthWeeks = (year: number, month: number): Week[] =>
  weeksBetween(
    new Date(Date.UTC(year, month - 1, 1)),
    new Date(Date.UTC(year, month, 0)),
  );

/** The weeks of a year, Sunday first, as the heatmap's columns. */
export const heatmapWeeks = (year: number): Week[] =>
  weeksBetween(
    new Date(Date.UTC(year, 0, 1)),
    new Date(Date.UTC(year, 11, 31)),
  );

/** One bar, or a lone finish marker, in a week of the calendar. */
export interface WeekItem {
  key: string;
  /** The first and last columns it covers, 0 (Sunday) to 6. */
  start: number;
  end: number;
  lane: number;
  /** The run of reading carries on from the week before, or into the next. */
  continuesBefore: boolean;
  continuesAfter: boolean;
  /** Columns where a Read-through of the Book finished. */
  finishes: number[];
  /** A finish with no reading that day, which shows only its marker. */
  markerOnly: boolean;
}

const readOn = (
  days: Map<string, ReadingStatsCalendarDay>,
  day: string | null,
  key: string,
) =>
  day !== null &&
  (days.get(day)?.books.some((b) => b.key === key && b.seconds > 0) ?? false);

/**
 * Lays out a week of the calendar: a bar for each run of consecutive days a
 * Book was read, and a marker for a finish on a day it wasn't, each in the
 * lowest lane free across its days. Items past `maxLanes` are left out and
 * counted under each day they'd cover, in `hidden`; an item with a finish is
 * placed first, so finishes are only hidden when they alone fill the lanes.
 */
export const layoutWeek = (
  week: Week,
  days: Map<string, ReadingStatsCalendarDay>,
  maxLanes = Infinity,
): { items: WeekItem[]; hidden: number[] } => {
  const keys: string[] = [];
  for (const day of week) {
    for (const book of (day && days.get(day)?.books) || []) {
      if (!keys.includes(book.key)) keys.push(book.key);
    }
  }

  const items: Omit<WeekItem, "lane">[] = [];
  for (const key of keys) {
    const entry = (col: number) =>
      week[col] ? days.get(week[col]!)?.books.find((b) => b.key === key) : null;
    let col = 0;
    while (col < 7) {
      const book = entry(col);
      if (!book) {
        col++;
        continue;
      }
      if (book.seconds === 0) {
        // Only a finish: a marker on its own.
        if (book.finished) {
          items.push({
            key,
            start: col,
            end: col,
            continuesBefore: false,
            continuesAfter: false,
            finishes: [col],
            markerOnly: true,
          });
        }
        col++;
        continue;
      }
      const start = col;
      while (col + 1 < 7 && (entry(col + 1)?.seconds ?? 0) > 0) col++;
      const end = col;
      const finishes: number[] = [];
      for (let c = start; c <= end; c++) {
        if (entry(c)?.finished) finishes.push(c);
      }
      items.push({
        key,
        start,
        end,
        continuesBefore:
          start === 0 &&
          week[0] !== null &&
          readOn(days, shiftDay(week[0], -1), key),
        continuesAfter:
          end === 6 &&
          week[6] !== null &&
          readOn(days, shiftDay(week[6], 1), key),
        finishes,
        markerOnly: false,
      });
      col++;
    }
  }

  // Finishes first, so a marker is never what spills past the last lane;
  // then earliest, then longest, so long runs settle in the top lanes.
  items.sort(
    (a, b) =>
      Number(b.finishes.length > 0) - Number(a.finishes.length > 0) ||
      a.start - b.start ||
      b.end - a.end ||
      a.key.localeCompare(b.key),
  );
  const lanes: boolean[][] = [];
  const placed: WeekItem[] = [];
  const hidden = [0, 0, 0, 0, 0, 0, 0];
  for (const item of items) {
    const cols = Array.from(
      { length: item.end - item.start + 1 },
      (_, i) => item.start + i,
    );
    let lane = lanes.findIndex((taken) => cols.every((c) => !taken[c]));
    if (lane === -1) lane = lanes.push([]) - 1;
    for (const c of cols) lanes[lane][c] = true;
    if (lane < maxLanes) {
      placed.push({ ...item, lane });
    } else {
      for (const c of cols) hidden[c]++;
    }
  }
  placed.sort((a, b) => a.start - b.start || a.lane - b.lane);
  return { items: placed, hidden };
};

/**
 * Sorts a day's time into one of five shades: 0 for none, 4 for the most,
 * and 1 to 3 by the quartiles of the days with any.
 */
export const heatLevels = (values: number[]): ((value: number) => number) => {
  const sorted = values.filter((v) => v > 0).sort((a, b) => a - b);
  const at = (q: number) =>
    sorted[Math.max(0, Math.ceil(q * sorted.length) - 1)] ?? 0;
  const [q1, q2, q3] = [at(0.25), at(0.5), at(0.75)];
  const max = sorted[sorted.length - 1] ?? 0;
  return (value) => {
    if (value <= 0) return 0;
    if (value >= max) return 4;
    if (value <= q1) return 1;
    if (value <= q2) return 2;
    if (value <= q3) return 3;
    return 4;
  };
};
