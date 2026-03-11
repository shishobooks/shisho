import { formatDate, formatDateTime } from "./format";
import { describe, expect, it } from "vitest";

describe("formatDate", () => {
  it("preserves the UTC date regardless of local timezone", () => {
    // A date stored as midnight UTC - should display as Jan 6, not Jan 5
    // even when the local timezone is west of UTC
    const result = formatDate("2021-01-06T00:00:00Z");
    expect(result).toContain("6");
    expect(result).not.toContain("5");
  });

  it("formats a date with time component correctly", () => {
    const result = formatDate("2024-03-15T00:00:00Z");
    expect(result).toContain("15");
  });
});

describe("formatDateTime", () => {
  it("includes a timezone indicator", () => {
    const result = formatDateTime("2024-01-15T18:30:00Z");
    // Should contain some timezone abbreviation (e.g., CST, EST, PST, UTC)
    expect(result).toMatch(/[A-Z]{2,5}/);
  });

  it("displays in local time, not UTC", () => {
    // In America/Chicago (UTC-6), 18:30 UTC = 12:30 PM local
    const result = formatDateTime("2024-01-15T18:30:00Z");
    expect(result).toContain("12:30");
  });
});
