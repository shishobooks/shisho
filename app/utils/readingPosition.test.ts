import { describe, expect, it } from "vitest";

import {
  reflowableOpenTarget,
  savedPage,
  savedSeconds,
} from "./readingPosition";

// A percentage-only position, as a copy a user linked stores (ADR 0017).
const percentageOnly = (percentage: number) => ({ percentage });

describe("reflowableOpenTarget", () => {
  it("opens at the saved CFI", () => {
    expect(
      reflowableOpenTarget({ cfi: "epubcfi(/6/14!/4/2)", percentage: 0.4 }),
    ).toBe("epubcfi(/6/14!/4/2)");
  });

  it("opens a position with no CFI at its percentage", () => {
    expect(reflowableOpenTarget(percentageOnly(0.4))).toEqual({
      fraction: 0.4,
    });
  });

  it("opens at the start with nothing saved, or nothing past the start", () => {
    expect(reflowableOpenTarget(null)).toBeUndefined();
    expect(reflowableOpenTarget(percentageOnly(0))).toBeUndefined();
  });
});

describe("savedPage", () => {
  it("opens at the saved page", () => {
    expect(savedPage({ page: 12, percentage: 0.5 }, 100)).toBe(12);
    expect(savedPage({ page: 0, percentage: 0 }, 100)).toBe(0);
  });

  it("opens a position with no page at the page for its percentage", () => {
    // The reader saves page / (totalPages - 1), so this undoes it.
    expect(savedPage(percentageOnly(0.5), 101)).toBe(50);
    expect(savedPage(percentageOnly(0.333), 10)).toBe(3);
    expect(savedPage(percentageOnly(1), 10)).toBe(9);
  });

  it("leaves the reader's default with nothing saved", () => {
    expect(savedPage(null, 10)).toBeNull();
    expect(savedPage(percentageOnly(0), 10)).toBeNull();
  });
});

describe("savedSeconds", () => {
  it("opens at the saved second", () => {
    expect(savedSeconds({ seconds: 125, percentage: 0.1 }, 3600)).toBe(125);
  });

  it("opens a position with no seconds at its percentage of the duration", () => {
    expect(savedSeconds(percentageOnly(0.25), 3600)).toBe(900);
  });

  it("opens at the start when the duration isn't known", () => {
    expect(savedSeconds(percentageOnly(0.25), 0)).toBe(0);
    expect(savedSeconds(percentageOnly(0.25), NaN)).toBe(0);
    expect(savedSeconds(percentageOnly(0.25), Infinity)).toBe(0);
  });

  it("opens at the start with nothing saved", () => {
    expect(savedSeconds(null, 3600)).toBe(0);
  });
});
