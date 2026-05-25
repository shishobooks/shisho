import { describe, expect, it } from "vitest";

import { aggregateDecisions, defaultDecision } from "./identify-decisions";

describe("defaultDecision", () => {
  it("returns true for new book-level fields regardless of source", () => {
    expect(
      defaultDecision({ scope: "book", status: "new", fieldSource: "manual" }),
    ).toBe(true);
    expect(
      defaultDecision({
        scope: "book",
        status: "new",
        fieldSource: "filepath",
      }),
    ).toBe(true);
    expect(
      defaultDecision({
        scope: "book",
        status: "new",
        fieldSource: undefined,
      }),
    ).toBe(true);
  });

  it("returns false for unchanged fields regardless of scope or source", () => {
    expect(
      defaultDecision({
        scope: "book",
        status: "unchanged",
        fieldSource: "filepath",
      }),
    ).toBe(false);
    expect(
      defaultDecision({
        scope: "file",
        status: "unchanged",
        fieldSource: undefined,
      }),
    ).toBe(false);
  });

  it("returns true for any non-unchanged file-level field regardless of source", () => {
    expect(
      defaultDecision({
        scope: "file",
        status: "new",
        fieldSource: "manual",
      }),
    ).toBe(true);
    expect(
      defaultDecision({
        scope: "file",
        status: "changed",
        fieldSource: "plugin",
      }),
    ).toBe(true);
    expect(
      defaultDecision({
        scope: "file",
        status: "changed",
        fieldSource: undefined,
      }),
    ).toBe(true);
  });

  it("returns ON for changed book-level fields with filepath source", () => {
    expect(
      defaultDecision({
        scope: "book",
        status: "changed",
        fieldSource: "filepath",
      }),
    ).toBe(true);
  });

  it("returns ON for changed book-level fields with file metadata source", () => {
    expect(
      defaultDecision({
        scope: "book",
        status: "changed",
        fieldSource: "file_metadata",
      }),
    ).toBe(true);
    expect(
      defaultDecision({
        scope: "book",
        status: "changed",
        fieldSource: "epub_metadata",
      }),
    ).toBe(true);
    expect(
      defaultDecision({
        scope: "book",
        status: "changed",
        fieldSource: "cbz_metadata",
      }),
    ).toBe(true);
    expect(
      defaultDecision({
        scope: "book",
        status: "changed",
        fieldSource: "m4b_metadata",
      }),
    ).toBe(true);
    expect(
      defaultDecision({
        scope: "book",
        status: "changed",
        fieldSource: "pdf_metadata",
      }),
    ).toBe(true);
  });

  it("returns OFF for changed book-level fields with manual source", () => {
    expect(
      defaultDecision({
        scope: "book",
        status: "changed",
        fieldSource: "manual",
      }),
    ).toBe(false);
  });

  it("returns OFF for changed book-level fields with sidecar source", () => {
    expect(
      defaultDecision({
        scope: "book",
        status: "changed",
        fieldSource: "sidecar",
      }),
    ).toBe(false);
  });

  it("returns OFF for changed book-level fields with plugin source", () => {
    expect(
      defaultDecision({
        scope: "book",
        status: "changed",
        fieldSource: "plugin",
      }),
    ).toBe(false);
  });

  it("returns OFF for changed book-level fields with plugin-specific source", () => {
    expect(
      defaultDecision({
        scope: "book",
        status: "changed",
        fieldSource: "plugin:shisho/goodreads-metadata",
      }),
    ).toBe(false);
  });

  it("returns ON for changed book-level fields with missing/unknown source", () => {
    expect(
      defaultDecision({
        scope: "book",
        status: "changed",
        fieldSource: undefined,
      }),
    ).toBe(true);
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
