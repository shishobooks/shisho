import { describe, expect, it } from "vitest";

import { formatDaysAgo } from "./format";
import { formatLastRead } from "./readingProgress";

const NOW = new Date(2026, 10, 20, 12, 0, 0);

const daysAgo = (days: number, hour = 12) =>
  new Date(2026, 10, 20 - days, hour, 0, 0).toISOString();

describe("formatDaysAgo", () => {
  it.each([
    [daysAgo(0, 1), "today"],
    [daysAgo(1, 23), "yesterday"],
    [daysAgo(2), "2 days ago"],
    [daysAgo(5), "5 days ago"],
    [daysAgo(29), "29 days ago"],
    [daysAgo(40), "about 1 month ago"],
    [daysAgo(120), "4 months ago"],
    [daysAgo(400), "about 1 year ago"],
  ])("%s reads as %s", (instant, expected) => {
    expect(formatDaysAgo(instant, NOW)).toBe(expected);
  });

  it("counts calendar days, not 24-hour stretches", () => {
    expect(formatDaysAgo(daysAgo(1, 23), new Date(2026, 10, 20, 0, 30))).toBe(
      "yesterday",
    );
  });

  it("treats a clock running ahead as today", () => {
    expect(formatDaysAgo(daysAgo(-1), NOW)).toBe("today");
  });
});

describe("formatLastRead", () => {
  const at = (day: number, hour: number, minute = 0) =>
    new Date(2026, 10, day, hour, minute).toISOString();

  it("says how long ago for the first 24 hours", () => {
    expect(formatLastRead(at(20, 8), NOW)).toBe("Last read 4 hours ago");
    expect(formatLastRead(at(20, 11, 58), NOW)).toBe("Last read 2 minutes ago");
    // Yesterday on the calendar, but under a day ago.
    expect(formatLastRead(at(19, 13), NOW)).toBe("Last read 23 hours ago");
  });

  it("says just now under a minute, and for a clock running ahead", () => {
    expect(
      formatLastRead(new Date(NOW.getTime() - 20_000).toISOString(), NOW),
    ).toBe("Last read just now");
    expect(formatLastRead(at(20, 12, 5), NOW)).toBe("Last read just now");
  });

  it("never rounds up to the next hour or to a whole day", () => {
    const ago = (ms: number) => new Date(NOW.getTime() - ms).toISOString();
    expect(formatLastRead(ago((23 * 60 + 59) * 60_000), NOW)).toBe(
      "Last read 23 hours ago",
    );
    expect(formatLastRead(ago(59.5 * 60_000), NOW)).toBe(
      "Last read 59 minutes ago",
    );
    expect(formatLastRead(ago(90 * 60_000), NOW)).toBe("Last read 1 hour ago");
  });

  it("says yesterday, not today, when daylight saving makes a day longer", () => {
    const zone = process.env.TZ;
    process.env.TZ = "America/Chicago";
    try {
      // Clocks fell back on Nov 1, 2026, so it ran 25 hours.
      const now = new Date(2026, 10, 1, 23, 11);
      expect(
        formatLastRead(new Date(2026, 10, 1, 0, 10).toISOString(), now),
      ).toBe("Last read yesterday");
    } finally {
      // Assigning undefined would store the string "undefined", which Node
      // reads as UTC for every test after this one.
      if (zone === undefined) delete process.env.TZ;
      else process.env.TZ = zone;
    }
  });

  it("goes by the day from 24 hours on", () => {
    expect(formatLastRead(at(19, 12), NOW)).toBe("Last read yesterday");
    expect(formatLastRead(at(19, 9), NOW)).toBe("Last read yesterday");
    expect(formatLastRead(at(15, 12), NOW)).toBe("Last read 5 days ago");
  });

  it("is null with no activity", () => {
    expect(formatLastRead(null, NOW)).toBeNull();
    expect(formatLastRead(undefined, NOW)).toBeNull();
  });
});
