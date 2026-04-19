import {
  BUILTIN_DEFAULT_SORT,
  parseSortSpec,
  serializeSortSpec,
  SORT_FIELDS,
  sortSpecsEqual,
  type SortLevel,
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

  it("returns null for trailing colon segments (title:asc:extra)", () => {
    // Mirrors Go's pkg/sortspec which rejects "asc:extra" as an invalid
    // direction. JS's split(_, 2) would silently truncate; we use an
    // unbounded split + length check to match Go.
    expect(parseSortSpec("title:asc:extra")).toBeNull();
  });

  it("returns null for empty middle segment (title::asc)", () => {
    expect(parseSortSpec("title::asc")).toBeNull();
  });

  it("returns null for trailing colon (title:asc:)", () => {
    expect(parseSortSpec("title:asc:")).toBeNull();
  });

  it("returns null for input containing whitespace", () => {
    expect(parseSortSpec("title:asc, title:desc")).toBeNull();
    expect(parseSortSpec(" title:asc")).toBeNull();
    expect(parseSortSpec("title:asc\n")).toBeNull();
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

describe("sortSpecsEqual", () => {
  it("returns true for two null/undefined inputs", () => {
    expect(sortSpecsEqual(null, null)).toBe(true);
    expect(sortSpecsEqual(undefined, undefined)).toBe(true);
    expect(sortSpecsEqual(null, undefined)).toBe(true);
  });

  it("returns false when only one side is null", () => {
    expect(sortSpecsEqual(null, [])).toBe(false);
    expect(sortSpecsEqual([], null)).toBe(false);
  });

  it("returns true for two empty arrays", () => {
    expect(sortSpecsEqual([], [])).toBe(true);
  });

  it("returns true for identical specs", () => {
    const a: SortLevel[] = [
      { field: "title", direction: "asc" },
      { field: "author", direction: "desc" },
    ];
    const b: SortLevel[] = [
      { field: "title", direction: "asc" },
      { field: "author", direction: "desc" },
    ];
    expect(sortSpecsEqual(a, b)).toBe(true);
  });

  it("returns false for different order (order matters)", () => {
    const a: SortLevel[] = [
      { field: "title", direction: "asc" },
      { field: "author", direction: "asc" },
    ];
    const b: SortLevel[] = [
      { field: "author", direction: "asc" },
      { field: "title", direction: "asc" },
    ];
    expect(sortSpecsEqual(a, b)).toBe(false);
  });

  it("returns false for different directions", () => {
    expect(
      sortSpecsEqual(
        [{ field: "title", direction: "asc" }],
        [{ field: "title", direction: "desc" }],
      ),
    ).toBe(false);
  });

  it("returns false for different lengths", () => {
    expect(
      sortSpecsEqual(
        [{ field: "title", direction: "asc" }],
        [
          { field: "title", direction: "asc" },
          { field: "author", direction: "asc" },
        ],
      ),
    ).toBe(false);
  });
});
