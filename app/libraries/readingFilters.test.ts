import { describe, expect, it } from "vitest";

import {
  hasReadingFilters,
  parseReadingFilters,
  ratingRangeLabel,
  readingFilterQuery,
  setRatingRangeParams,
  toggleReadingStatusParam,
  writeReadingFilters,
} from "./readingFilters";

describe("parseReadingFilters", () => {
  it("reads the statuses and rating range from the URL", () => {
    const filters = parseReadingFilters(
      new URLSearchParams(
        "reading_status=read,none&rating_min=4&rating_max=4.5",
      ),
    );
    expect(filters).toEqual({
      statuses: ["read", "none"],
      ratingMin: 4,
      ratingMax: 4.5,
    });
    expect(hasReadingFilters(filters)).toBe(true);
  });

  it("drops values the server would refuse", () => {
    const filters = parseReadingFilters(
      new URLSearchParams(
        "reading_status=read,finished&rating_min=0&rating_max=nope",
      ),
    );
    expect(filters).toEqual({ statuses: ["read"] });
  });

  it("drops a range whose minimum is above its maximum", () => {
    expect(
      parseReadingFilters(new URLSearchParams("rating_min=4&rating_max=3")),
    ).toEqual({ statuses: [], ratingMin: 4 });
  });

  it("has no filters on an empty URL", () => {
    const filters = parseReadingFilters(new URLSearchParams());
    expect(filters).toEqual({ statuses: [] });
    expect(hasReadingFilters(filters)).toBe(false);
    expect(readingFilterQuery(filters)).toEqual({});
  });
});

describe("writeReadingFilters", () => {
  it("round-trips through the URL and clears what's empty", () => {
    const params = new URLSearchParams("q=x&rating_max=2");
    writeReadingFilters(params, {
      statuses: ["paused", "to_read"],
      ratingMin: 3.5,
    });
    expect(params.toString()).toBe(
      "q=x&reading_status=paused%2Cto_read&rating_min=3.5",
    );
    writeReadingFilters(params, { statuses: [] });
    expect(params.toString()).toBe("q=x");
  });
});

describe("readingFilterQuery", () => {
  it("sends only what's set", () => {
    expect(readingFilterQuery({ statuses: ["read"], ratingMin: 4 })).toEqual({
      reading_status: ["read"],
      rating_min: 4,
    });
  });
});

describe("ratingRangeLabel", () => {
  it("names the range", () => {
    expect(ratingRangeLabel(4, undefined)).toBe("4+ stars");
    expect(ratingRangeLabel(undefined, 2.5)).toBe("Up to 2.5 stars");
    expect(ratingRangeLabel(3, 4)).toBe("3 to 4 stars");
    expect(ratingRangeLabel(5, 5)).toBe("5 stars");
    expect(ratingRangeLabel(1, 1)).toBe("1 star");
  });
});

describe("toggleReadingStatusParam", () => {
  it("adds or removes a status in the URL, keeping the others", () => {
    const params = new URLSearchParams("reading_status=read&rating_min=4");
    toggleReadingStatusParam(params, "paused");
    expect(params.get("reading_status")).toBe("read,paused");
    toggleReadingStatusParam(params, "read");
    toggleReadingStatusParam(params, "paused");
    expect(params.toString()).toBe("rating_min=4");
  });
});

describe("setRatingRangeParams", () => {
  it("sets the range in the URL, keeping the statuses", () => {
    const params = new URLSearchParams("reading_status=read&rating_max=2");
    setRatingRangeParams(params, 4, undefined);
    expect(params.toString()).toBe("reading_status=read&rating_min=4");
  });
});
