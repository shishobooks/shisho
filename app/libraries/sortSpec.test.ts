import {
  BUILTIN_DEFAULT_SORT,
  parseSortSpec,
  serializeSortSpec,
  SORT_FIELDS,
} from "./sortSpec";
import { describe, expect, it } from "vitest";

describe("parseSortSpec", () => {
  it("parses single level", () => {
    expect(parseSortSpec("title:asc")).toEqual([
      { field: "title", direction: "asc" },
    ]);
  });

  it("parses multi-level", () => {
    expect(parseSortSpec("author:asc,series:asc,title:asc")).toEqual([
      { field: "author", direction: "asc" },
      { field: "series", direction: "asc" },
      { field: "title", direction: "asc" },
    ]);
  });

  it("returns null for unknown field", () => {
    expect(parseSortSpec("bogus:asc")).toBeNull();
  });

  it("returns null for bad direction", () => {
    expect(parseSortSpec("title:sideways")).toBeNull();
  });

  it("returns null for duplicate field", () => {
    expect(parseSortSpec("title:asc,title:desc")).toBeNull();
  });

  it("returns null for empty string", () => {
    expect(parseSortSpec("")).toBeNull();
  });
});

describe("serializeSortSpec", () => {
  it("serializes a single level", () => {
    expect(serializeSortSpec([{ field: "title", direction: "asc" }])).toBe(
      "title:asc",
    );
  });

  it("returns empty string for empty array", () => {
    expect(serializeSortSpec([])).toBe("");
  });

  it("round-trips", () => {
    const input = "author:asc,date_added:desc";
    const parsed = parseSortSpec(input);
    expect(parsed).not.toBeNull();
    expect(serializeSortSpec(parsed!)).toBe(input);
  });
});

describe("SORT_FIELDS", () => {
  it("matches the Go whitelist", () => {
    // If this test fails, the Go side (pkg/sortspec/whitelist.go)
    // and the TS side have drifted. Update both in lockstep.
    expect(SORT_FIELDS).toEqual([
      "title",
      "author",
      "series",
      "date_added",
      "date_released",
      "page_count",
      "duration",
    ]);
  });
});

describe("BUILTIN_DEFAULT_SORT", () => {
  it("is date_added:desc", () => {
    expect(BUILTIN_DEFAULT_SORT).toEqual([
      { field: "date_added", direction: "desc" },
    ]);
    expect(serializeSortSpec(BUILTIN_DEFAULT_SORT)).toBe("date_added:desc");
  });
});
