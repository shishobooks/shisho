import { describe, expect, it } from "vitest";

import { formatSeriesNumber } from "./seriesNumber";

describe("formatSeriesNumber", () => {
  it("renders volume for CBZ with volume unit", () => {
    expect(formatSeriesNumber(5, "volume", "cbz")).toBe("Vol. 5");
  });
  it("renders chapter for CBZ with chapter unit", () => {
    expect(formatSeriesNumber(42, "chapter", "cbz")).toBe("Ch. 42");
  });
  it("defaults null unit to volume for CBZ", () => {
    expect(formatSeriesNumber(5, null, "cbz")).toBe("Vol. 5");
  });
  it("uses bare number for non-CBZ", () => {
    expect(formatSeriesNumber(3, null, "epub")).toBe("3");
  });
  it("returns empty for null number", () => {
    expect(formatSeriesNumber(null, null, "cbz")).toBe("");
  });
  it("formats fractional", () => {
    expect(formatSeriesNumber(7.5, "chapter", "cbz")).toBe("Ch. 7.5");
  });
});
