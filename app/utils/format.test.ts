import { describe, expect, it } from "vitest";

import { formatDate, formatDateTime, formatPlayerTime } from "./format";

describe("formatDate", () => {
  it("preserves the UTC date regardless of local timezone", () => {
    // A date stored as midnight UTC - should display as Jan 6, not Jan 5
    // even when the local timezone is west of UTC
    const result = formatDate("2021-01-06T00:00:00Z");
    expect(result).toContain("Jan");
    expect(result).toContain("6");
    expect(result).toContain("2021");
  });

  it("formats a date with time component correctly", () => {
    const result = formatDate("2024-03-15T00:00:00Z");
    expect(result).toContain("Mar");
    expect(result).toContain("15");
  });
});

describe("formatDateTime", () => {
  it("includes a timezone indicator", () => {
    const result = formatDateTime("2024-01-15T18:30:00Z");
    // Should contain some timezone abbreviation (e.g., CST, EST, PST, UTC)
    expect(result).toMatch(/[A-Z]{2,5}/);
  });

  it("includes date and time components", () => {
    const result = formatDateTime("2024-01-15T18:30:00Z");
    expect(result).toContain("Jan");
    expect(result).toContain("15");
    expect(result).toContain("2024");
    // Should contain a time with :30 minutes regardless of timezone
    expect(result).toMatch(/:30/);
  });
});

describe("formatPlayerTime", () => {
  it("formats sub-minute durations as M:SS with zero minutes", () => {
    expect(formatPlayerTime(5)).toBe("0:05");
    expect(formatPlayerTime(45)).toBe("0:45");
  });

  it("formats minute-and-second durations as M:SS", () => {
    expect(formatPlayerTime(65)).toBe("1:05");
    expect(formatPlayerTime(185)).toBe("3:05");
  });

  it("formats hour-long durations as H:MM:SS", () => {
    expect(formatPlayerTime(3661)).toBe("1:01:01");
    expect(formatPlayerTime(3600)).toBe("1:00:00");
  });

  it("truncates fractional seconds rather than rounding up", () => {
    expect(formatPlayerTime(59.9)).toBe("0:59");
    expect(formatPlayerTime(0.4)).toBe("0:00");
  });

  it("clamps NaN, negative, and non-finite inputs to 0:00", () => {
    expect(formatPlayerTime(NaN)).toBe("0:00");
    expect(formatPlayerTime(-10)).toBe("0:00");
    expect(formatPlayerTime(Infinity)).toBe("0:00");
  });
});
