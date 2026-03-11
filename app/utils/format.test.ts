import { formatDate } from "./format";
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
