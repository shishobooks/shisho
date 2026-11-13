import { describe, expect, it } from "vitest";

import type { ReadingStatsCalendarDay, ReadingStatsDaySession } from "@/types";
import {
  assignBookColors,
  BOOK_COLORS,
  bookColor,
  dayIn,
  dayLanes,
  heatLevels,
  heatmapWeeks,
  layoutWeek,
  monthWeeks,
  shiftDay,
  textOn,
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

describe("textOn", () => {
  // WCAG relative luminance and contrast ratio of two #rrggbb colours.
  const luminance = (hex: string) => {
    const [r, g, b] = [1, 3, 5].map((i) => {
      const c = parseInt(hex.slice(i, i + 2), 16) / 255;
      return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
    });
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  };
  const contrast = (a: string, b: string) => {
    const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
    return (hi + 0.05) / (lo + 0.05);
  };

  it("gives every book colour text that meets 4.5:1", () => {
    for (const color of BOOK_COLORS) {
      const text = textOn(color);
      const hex = text === "white" ? "#ffffff" : text;
      expect(contrast(color, hex), color).toBeGreaterThanOrEqual(4.5);
    }
  });
});

describe("assignBookColors", () => {
  it("gives the books shown together different colours", () => {
    const keys = Array.from(
      { length: BOOK_COLORS.length },
      (_, i) => `book-${i}`,
    );
    const colors = assignBookColors(keys);
    expect(new Set(keys.map((key) => colors.get(key))).size).toBe(keys.length);
  });

  it("keeps a book's own colour unless another book here has it", () => {
    // Find two keys that hash to the same colour, and one that doesn't.
    const keys = Array.from({ length: 200 }, (_, i) => `book-${i}`);
    const [first, clash] = keys.filter(
      (key) => bookColor(key) === bookColor(keys[0]),
    );
    const other = keys.find((key) => bookColor(key) !== bookColor(keys[0]))!;

    expect(assignBookColors([first, other]).get(first)).toBe(bookColor(first));
    expect(assignBookColors([clash, other]).get(clash)).toBe(bookColor(clash));
    const together = assignBookColors([clash, other, first]);
    expect(together.get(clash)).not.toBe(together.get(first));
    expect(together.get(other)).toBe(bookColor(other));
  });

  it("doesn't depend on the order the books come in", () => {
    const keys = Array.from({ length: 12 }, (_, i) => `book-${i * 7}`);
    expect([...assignBookColors(keys)].sort()).toEqual(
      [...assignBookColors([...keys].reverse())].sort(),
    );
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
  it("covers the days from one to another in Sunday-first columns", () => {
    // 2025-11-14 is a Friday and 2026-11-13 a Friday.
    const weeks = heatmapWeeks("2025-11-14", "2026-11-13");
    expect(weeks[0]).toEqual([
      null,
      null,
      null,
      null,
      null,
      "2025-11-14",
      "2025-11-15",
    ]);
    expect(weeks.flat().filter(Boolean)).toHaveLength(365);
    expect(weeks[weeks.length - 1]).toEqual([
      "2026-11-08",
      "2026-11-09",
      "2026-11-10",
      "2026-11-11",
      "2026-11-12",
      "2026-11-13",
      null,
    ]);
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

describe("dayLanes", () => {
  const session = (
    id: number,
    key: string,
    started_at: string,
    ended_at: string,
    seconds: number,
  ): ReadingStatsDaySession => ({
    id,
    book: { key, title: key, removed: false, book: null },
    started_at,
    ended_at,
    seconds,
    start_percentage: 0,
    end_percentage: 0,
    kind: "reading",
    source: "web_reader",
    measured: true,
  });

  it("gives each book a lane, in the order first read, with its sessions in minutes of the day", () => {
    // Chicago is UTC-6 in November.
    const lanes = dayLanes(
      [
        session(
          1,
          "book-1",
          "2026-11-08T13:15:00Z",
          "2026-11-08T13:40:00Z",
          1500,
        ),
        session(
          2,
          "book-2",
          "2026-11-08T14:15:00Z",
          "2026-11-08T15:12:00Z",
          3420,
        ),
        session(
          3,
          "book-1",
          "2026-11-08T21:30:00Z",
          "2026-11-08T22:20:00Z",
          3000,
        ),
      ],
      "2026-11-08",
      "America/Chicago",
    );
    expect(lanes).toEqual([
      {
        book: expect.objectContaining({ key: "book-1" }),
        seconds: 4500,
        sessions: [
          expect.objectContaining({
            id: 1,
            from: 7 * 60 + 15,
            to: 7 * 60 + 40,
          }),
          expect.objectContaining({
            id: 3,
            from: 15 * 60 + 30,
            to: 16 * 60 + 20,
          }),
        ],
      },
      {
        book: expect.objectContaining({ key: "book-2" }),
        seconds: 3420,
        sessions: [
          expect.objectContaining({
            id: 2,
            from: 8 * 60 + 15,
            to: 9 * 60 + 12,
          }),
        ],
      },
    ]);
  });

  // November 1, 2026 is 25 hours long in Chicago: 1 AM to 2 AM happens
  // twice. A session across the change still has its real length, scaled
  // to the day's 24-hour strip.
  it("places a session across a daylight-saving change by the time it took", () => {
    const fallBack = session(
      1,
      "book-1",
      "2026-11-01T06:50:00Z", // 1:50 AM CDT
      "2026-11-01T07:10:00Z", // 1:10 AM CST
      1200,
    );
    const [placed] = dayLanes([fallBack], "2026-11-01", "America/Chicago")[0]
      .sessions;
    const scale = 1440 / (25 * 60);
    expect(placed.from).toBeCloseTo(110 * scale);
    expect(placed.to).toBeCloseTo(130 * scale);
  });

  it("cuts a session past midnight at the edge of the day", () => {
    const night = session(
      1,
      "book-1",
      "2026-11-11T05:40:00Z",
      "2026-11-11T06:20:00Z",
      1200,
    );
    expect(
      dayLanes([night], "2026-11-10", "America/Chicago")[0].sessions[0],
    ).toMatchObject({
      from: 23 * 60 + 40,
      to: 24 * 60,
    });
    expect(
      dayLanes([night], "2026-11-11", "America/Chicago")[0].sessions[0],
    ).toMatchObject({
      from: 0,
      to: 20,
    });
  });
});

describe("dayIn", () => {
  it("gives the day an instant falls on in a time zone", () => {
    expect(dayIn("2026-06-01T04:30:00Z", "America/Chicago")).toBe("2026-05-31");
    expect(dayIn("2026-06-01T04:30:00Z", "UTC")).toBe("2026-06-01");
  });
});
