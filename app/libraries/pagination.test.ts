import { describe, expect, it } from "vitest";

import {
  parsePageParam,
  parseStartParam,
  resetPaging,
  startOffset,
} from "./pagination";

describe("parsePageParam", () => {
  it("parses a valid page number", () => {
    expect(parsePageParam("3")).toBe(3);
  });

  it("defaults to 1 when the param is missing", () => {
    expect(parsePageParam(null)).toBe(1);
  });

  it("defaults to 1 for non-numeric values", () => {
    expect(parsePageParam("garbage")).toBe(1);
  });

  it("defaults to 1 for zero", () => {
    expect(parsePageParam("0")).toBe(1);
  });

  it("defaults to 1 for negative values", () => {
    expect(parsePageParam("-2")).toBe(1);
  });

  it("truncates trailing junk like parseInt does", () => {
    expect(parsePageParam("2abc")).toBe(2);
  });
});

describe("startOffset", () => {
  it("starts the page on the row-aligned page holding the start item", () => {
    expect(startOffset(48, 24)).toBe(48);
    expect(startOffset(50, 24)).toBe(48);
    expect(startOffset(48, 33)).toBe(33);
  });

  it("treats a missing or invalid start as the first item", () => {
    expect(startOffset(0, 24)).toBe(0);
    expect(startOffset(-5, 24)).toBe(0);
    expect(startOffset(Number.NaN, 24)).toBe(0);
  });
});

describe("parseStartParam", () => {
  it("reads a whole number of items", () => {
    expect(parseStartParam("48")).toBe(48);
  });

  it("returns 0 for missing or invalid values", () => {
    expect(parseStartParam(null)).toBe(0);
    expect(parseStartParam("abc")).toBe(0);
    expect(parseStartParam("-3")).toBe(0);
  });
});

describe("resetPaging", () => {
  it("drops both the item index and an old page number", () => {
    const params = new URLSearchParams("page=3&start=48&search=jo");

    resetPaging(params);

    expect(params.toString()).toBe("search=jo");
  });
});
