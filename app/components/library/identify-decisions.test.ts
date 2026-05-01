import { describe, expect, it } from "vitest";

import { aggregateDecisions, defaultDecision } from "./identify-decisions";

describe("defaultDecision", () => {
  it("returns true for new book-level fields regardless of primary status", () => {
    expect(
      defaultDecision({ scope: "book", status: "new", isPrimaryFile: false }),
    ).toBe(true);
    expect(
      defaultDecision({ scope: "book", status: "new", isPrimaryFile: true }),
    ).toBe(true);
  });

  it("returns true for changed book-level fields on primary file", () => {
    expect(
      defaultDecision({
        scope: "book",
        status: "changed",
        isPrimaryFile: true,
      }),
    ).toBe(true);
  });

  it("returns false for changed book-level fields on non-primary file", () => {
    expect(
      defaultDecision({
        scope: "book",
        status: "changed",
        isPrimaryFile: false,
      }),
    ).toBe(false);
  });

  it("returns true for any non-unchanged file-level field", () => {
    expect(
      defaultDecision({ scope: "file", status: "new", isPrimaryFile: false }),
    ).toBe(true);
    expect(
      defaultDecision({
        scope: "file",
        status: "changed",
        isPrimaryFile: false,
      }),
    ).toBe(true);
    expect(
      defaultDecision({ scope: "file", status: "new", isPrimaryFile: true }),
    ).toBe(true);
  });

  it("returns false for unchanged fields regardless of scope or primary", () => {
    expect(
      defaultDecision({
        scope: "book",
        status: "unchanged",
        isPrimaryFile: true,
      }),
    ).toBe(false);
    expect(
      defaultDecision({
        scope: "file",
        status: "unchanged",
        isPrimaryFile: false,
      }),
    ).toBe(false);
  });
});

describe("aggregateDecisions", () => {
  it("returns false for empty list", () => {
    expect(aggregateDecisions([])).toBe(false);
  });

  it("returns false when all decisions are false", () => {
    expect(aggregateDecisions([false, false, false])).toBe(false);
  });

  it("returns true when all decisions are true", () => {
    expect(aggregateDecisions([true, true])).toBe(true);
  });

  it("returns indeterminate for mixed decisions", () => {
    expect(aggregateDecisions([true, false])).toBe("indeterminate");
    expect(aggregateDecisions([false, true, false])).toBe("indeterminate");
  });
});
