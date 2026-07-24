import { describe, expect, it } from "vitest";

import { formatSeriesNumber } from "./seriesNumber";

describe("formatSeriesNumber", () => {
  it("renders a bare range", () => {
    expect(formatSeriesNumber(1, 3, null, "epub")).toBe("1-3");
  });

  it("renders volume for CBZ with volume unit", () => {
    expect(formatSeriesNumber(5, null, "volume", "cbz")).toBe("Vol. 5");
  });

  it("renders a volume range for CBZ", () => {
    expect(formatSeriesNumber(1, 3, "volume", "cbz")).toBe("Vol. 1-3");
  });

  it("renders chapter for CBZ with chapter unit", () => {
    expect(formatSeriesNumber(42, null, "chapter", "cbz")).toBe("Ch. 42");
  });

  it("renders a chapter range for CBZ", () => {
    expect(formatSeriesNumber(5, 8, "chapter", "cbz")).toBe("Ch. 5-8");
  });

  it("defaults null unit to volume for CBZ", () => {
    expect(formatSeriesNumber(5, null, null, "cbz")).toBe("Vol. 5");
  });

  it("uses bare number for non-CBZ", () => {
    expect(formatSeriesNumber(3, null, null, "epub")).toBe("3");
  });

  it("returns empty for null number", () => {
    expect(formatSeriesNumber(null, 3, null, "cbz")).toBe("");
  });

  it("formats fractional", () => {
    expect(formatSeriesNumber(7.5, null, "chapter", "cbz")).toBe("Ch. 7.5");
  });
});
