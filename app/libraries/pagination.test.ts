import { describe, expect, it } from "vitest";

import { parsePageParam } from "./pagination";

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
