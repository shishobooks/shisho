import { describe, expect, it } from "vitest";

import type { ReadingStatsCalendarDay } from "@/types";
import {
  BOOK_COLORS,
  bookColor,
  heatLevels,
  heatmapWeeks,
  layoutWeek,
  monthWeeks,
  shiftDay,
} from "@/utils/readingCalendar";

const day = (
  date: string,
  books: [key: string, seconds: number, finished?: boolean][],
): ReadingStatsCalendarDay => ({
  day: date,
  books: books.map(([key, seconds, finished = false]) => ({
    key,
    seconds,
    finished,
  })),
});

describe("bookColor", () => {
  it("gives a key the same colour every time", () => {
    expect(bookColor("book-12")).toBe(bookColor("book-12"));
    expect(BOOK_COLORS).toContain(bookColor("book-12"));
  });

  it("spreads keys over the palette", () => {
    const colors = new Set(
      Array.from({ length: 40 }, (_, i) => bookColor(`book-${i}`)),
    );
    expect(colors.size).toBeGreaterThan(BOOK_COLORS.length / 2);
  });
});

describe("shiftDay", () => {
  it("steps across month and year ends", () => {
    expect(shiftDay("2026-02-28", 1)).toBe("2026-03-01");
    expect(shiftDay("2026-01-01", -1)).toBe("2025-12-31");
  });
});

describe("monthWeeks", () => {
  it("lays a month out in Sunday-first weeks with blanks outside it", () => {
    // April 2026 starts on a Wednesday and ends on a Thursday.
    const weeks = monthWeeks(2026, 4);
    expect(weeks).toHaveLength(5);
    expect(weeks[0]).toEqual([
      null,
      null,
      null,
      "2026-04-01",
      "2026-04-02",
      "2026-04-03",
      "2026-04-04",
    ]);
    expect(weeks[4]).toEqual([
      "2026-04-26",
      "2026-04-27",
      "2026-04-28",
      "2026-04-29",
      "2026-04-30",
      null,
      null,
    ]);
  });
});

describe("layoutWeek", () => {
  const week = monthWeeks(2026, 4)[1]; // April 5 to 11

  it("joins a book read on consecutive days into one bar", () => {
    const days = new Map(
      [
        day("2026-04-06", [["book-1", 600]]),
        day("2026-04-07", [["book-1", 900]]),
        day("2026-04-08", [["book-1", 300]]),
        day("2026-04-10", [["book-1", 300]]),
      ].map((d) => [d.day, d]),
    );
    const { items } = layoutWeek(week, days);
    expect(
      items.map(({ key, start, end, lane }) => ({ key, start, end, lane })),
    ).toEqual([
      { key: "book-1", start: 1, end: 3, lane: 0 },
      { key: "book-1", start: 5, end: 5, lane: 0 },
    ]);
  });

  it("leaves a bar's edge open where the run carries on into another week", () => {
    const days = new Map(
      [
        day("2026-04-04", [["book-1", 600]]),
        day("2026-04-05", [["book-1", 600]]),
        day("2026-04-11", [["book-1", 600]]),
        day("2026-04-12", [["book-1", 600]]),
      ].map((d) => [d.day, d]),
    );
    const { items } = layoutWeek(week, days);
    expect(items[0]).toMatchObject({ start: 0, continuesBefore: true });
    expect(items[0].continuesAfter).toBe(false);
    expect(items[1]).toMatchObject({ start: 6, continuesAfter: true });
    expect(items[1].continuesBefore).toBe(false);
  });

  it("stacks books that overlap and marks finishes", () => {
    const days = new Map(
      [
        day("2026-04-06", [
          ["book-1", 600],
          ["book-2", 600],
        ]),
        day("2026-04-07", [
          ["book-1", 600, true],
          ["book-2", 600],
        ]),
        day("2026-04-08", [["book-3", 0, true]]),
      ].map((d) => [d.day, d]),
    );
    const { items } = layoutWeek(week, days);
    expect(
      items.map(({ key, lane, finishes, markerOnly }) => ({
        key,
        lane,
        finishes,
        markerOnly,
      })),
    ).toEqual([
      { key: "book-1", lane: 0, finishes: [2], markerOnly: false },
      { key: "book-2", lane: 1, finishes: [], markerOnly: false },
      // A finish with no reading is a marker on its own.
      { key: "book-3", lane: 0, finishes: [3], markerOnly: true },
    ]);
  });

  it("counts what doesn't fit in the lanes under each day", () => {
    const books = ["book-1", "book-2", "book-3", "book-4", "book-5"];
    const days = new Map(
      [
        day(
          "2026-04-06",
          books.map((key) => [key, 60]),
        ),
      ].map((d) => [d.day, d]),
    );
    const { items, hidden } = layoutWeek(week, days, 3);
    expect(items).toHaveLength(3);
    expect(hidden).toEqual([0, 2, 0, 0, 0, 0, 0]);
  });
});

describe("layoutWeek with more books than lanes", () => {
  it("never hides a finish in the overflow", () => {
    const week = monthWeeks(2026, 4)[1];
    const books = ["book-1", "book-2", "book-3", "book-4"];
    const days = new Map(
      [
        day("2026-04-07", [["book-9", 0, true]]),
        day("2026-04-06", [
          ...books.map((key): [string, number] => [key, 60]),
          ["book-5", 60, true],
        ]),
      ].map((d) => [d.day, d]),
    );
    const { items, hidden } = layoutWeek(week, days, 3);
    expect(items.some((item) => item.key === "book-5")).toBe(true);
    expect(items.some((item) => item.key === "book-9")).toBe(true);
    expect(hidden[1]).toBe(2);
  });
});

describe("heatmapWeeks", () => {
  it("covers the year in Sunday-first columns", () => {
    // 2026 starts on a Thursday.
    const weeks = heatmapWeeks(2026);
    expect(weeks[0].slice(0, 5)).toEqual([
      null,
      null,
      null,
      null,
      "2026-01-01",
    ]);
    expect(weeks.flat().filter(Boolean)).toHaveLength(365);
    expect(weeks[weeks.length - 1]).toContain("2026-12-31");
  });
});

describe("heatLevels", () => {
  it("shades the days read most darkest", () => {
    const levels = heatLevels([0, 60, 600, 1200, 3600, 7200]);
    expect(levels(0)).toBe(0);
    expect(levels(60)).toBe(1);
    expect(levels(7200)).toBe(4);
    expect(levels(3600)).toBeGreaterThan(levels(600));
  });

  it("puts every day at the top level when they're all the same", () => {
    const levels = heatLevels([600, 600]);
    expect(levels(600)).toBe(4);
  });
});
