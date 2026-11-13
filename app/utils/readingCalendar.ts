import type {
  ReadingStatsBookRef,
  ReadingStatsCalendarDay,
  ReadingStatsDaySession,
} from "@/types";

/**
 * Colours a Book's bars take on the Reading Stats calendar: twelve bright
 * ones that differ in lightness as well as hue, so even neighbours on the
 * colour wheel tell apart. The light ones carry dark text (`textOn`).
 */
export const BOOK_COLORS = [
  "#1f6fb2", // blue
  "#c2570c", // orange
  "#23822b", // green
  "#c22b2b", // red
  "#7445b0", // purple
  "#facc15", // yellow
  "#b8337f", // pink
  "#566570", // slate
  "#84cc16", // lime
  "#11808f", // teal
  "#3445a6", // indigo
  "#38bdf8", // sky
];

// Book colours too light for white text, which take this instead.
const DARK_TEXT = "#1a1a1a";
const TAKES_DARK_TEXT = new Set(["#facc15", "#84cc16", "#38bdf8"]);

/**
 * The text colour that reads on a Book's colour, each pairing at least
 * 4.5:1: white, or near-black on the light ones.
 */
export const textOn = (background: string): string =>
  TAKES_DARK_TEXT.has(background) ? DARK_TEXT : "white";

const colorIndex = (key: string): number => {
  let hash = 0x811c9dc5;
  for (let i = 0; i < key.length; i++) {
    hash ^= key.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }
  return (hash >>> 0) % BOOK_COLORS.length;
};

/**
 * A Book's own colour, from its calendar key (FNV-1a), which it keeps in
 * every month where no other Book shown has it.
 */
export const bookColor = (key: string): string => BOOK_COLORS[colorIndex(key)];

/**
 * Colours for the Books shown together, by key: each keeps its own colour,
 * except that when two share one, all but the first by key take the next
 * free colour, so no two Books in view look alike (up to the palette's
 * size). The result doesn't depend on the order of `keys`.
 */
export const assignBookColors = (keys: string[]): Map<string, string> => {
  const sorted = [...new Set(keys)].sort();
  const taken = new Map<number, string>();
  const displaced: string[] = [];
  for (const key of sorted) {
    const index = colorIndex(key);
    if (taken.has(index)) displaced.push(key);
    else taken.set(index, key);
  }
  const colors = new Map<string, string>();
  for (const [index, key] of taken) colors.set(key, BOOK_COLORS[index]);
  for (const key of displaced) {
    let index = colorIndex(key);
    for (let step = 0; step < BOOK_COLORS.length && taken.has(index); step++) {
      index = (index + 1) % BOOK_COLORS.length;
    }
    taken.set(index, key);
    colors.set(key, BOOK_COLORS[index]);
  }
  return colors;
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

/** The weeks from `from` to `to` (YYYY-MM-DD), Sunday first, as the heatmap's columns. */
export const heatmapWeeks = (from: string, to: string): Week[] =>
  weeksBetween(parseDay(from), parseDay(to));

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

const MINUTES_IN_DAY = 24 * 60;

/** The calendar day (YYYY-MM-DD) an instant falls on in timeZone. */
export const dayIn = (instant: string, timeZone: string): string =>
  // en-CA formats a date as YYYY-MM-DD.
  new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date(instant));

// How far timeZone is ahead of UTC at an instant, in milliseconds.
const zoneOffset = (ms: number, timeZone: string) => {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat("en-US", {
      timeZone,
      hourCycle: "h23",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    })
      .formatToParts(new Date(ms))
      .map((part) => [part.type, Number(part.value)]),
  );
  const local = Date.UTC(
    parts.year,
    parts.month - 1,
    parts.day,
    parts.hour,
    parts.minute,
    parts.second,
  );
  return local - Math.floor(ms / 1000) * 1000;
};

// The instant `day` begins in timeZone: its local midnight, checked again
// at the offset it lands on, so a day after a change starts on time.
const startOfDay = (day: string, timeZone: string) => {
  const utcMidnight = parseDay(day).getTime();
  const guess = utcMidnight - zoneOffset(utcMidnight, timeZone);
  return utcMidnight - zoneOffset(guess, timeZone);
};

// An instant's place on `day`'s 24-hour strip in timeZone, in minutes: the
// time since local midnight, scaled so a 23- or 25-hour day still fills the
// strip, and held to the day's edges.
const minutesInto = (instant: string, day: string, timeZone: string) => {
  const start = startOfDay(day, timeZone);
  const dayMinutes = (startOfDay(shiftDay(day, 1), timeZone) - start) / 60000;
  const minutes = (Date.parse(instant) - start) / 60000;
  return (
    Math.min(Math.max(minutes, 0), dayMinutes) * (MINUTES_IN_DAY / dayMinutes)
  );
};

/** A Session placed on the day's timeline, in minutes from midnight. */
export type TimelineSession = ReadingStatsDaySession & {
  from: number;
  to: number;
};

/** One Book's line on a day's timeline, and its time that day. */
export interface DayLane {
  book: ReadingStatsBookRef;
  seconds: number;
  sessions: TimelineSession[];
}

/**
 * A day's Sessions as a line per Book, in the order each was first read,
 * each Session placed in minutes from local midnight in timeZone and cut at
 * the day's edges.
 */
export const dayLanes = (
  sessions: ReadingStatsDaySession[],
  day: string,
  timeZone: string,
): DayLane[] => {
  const lanes = new Map<string, DayLane>();
  const sorted = [...sessions].sort(
    (a, b) => Date.parse(a.started_at) - Date.parse(b.started_at),
  );
  for (const session of sorted) {
    let lane = lanes.get(session.book.key);
    if (!lane) {
      lane = { book: session.book, seconds: 0, sessions: [] };
      lanes.set(session.book.key, lane);
    }
    lane.seconds += session.seconds;
    lane.sessions.push({
      ...session,
      from: minutesInto(session.started_at, day, timeZone),
      to: minutesInto(session.ended_at, day, timeZone),
    });
  }
  return [...lanes.values()];
};
