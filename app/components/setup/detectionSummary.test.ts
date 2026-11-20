import { describe, expect, it } from "vitest";

import { detectionSummary } from "./detectionSummary";

const found = { audiobooks: 3, comics: 0, ebooks: 1200, stopped_early: false };

describe("detectionSummary", () => {
  it("speaks of the folders Setup looked through", () => {
    expect(detectionSummary(found)).toBe(
      "We found about 1,200 ebooks and 3 audiobooks.",
    );
  });

  it("speaks of the libraries when the plugin page looked through them", () => {
    expect(detectionSummary(found, "libraries")).toBe(
      "We found about 1,200 ebooks and 3 audiobooks in your libraries.",
    );
    expect(
      detectionSummary({ ...found, stopped_early: true }, "libraries"),
    ).toBe(
      "Your libraries are large, so we looked through part of them and found about 1,200 ebooks and 3 audiobooks.",
    );
    expect(
      detectionSummary(
        { audiobooks: 0, comics: 0, ebooks: 0, stopped_early: false },
        "libraries",
      ),
    ).toBe(
      "We didn't find any books in your libraries yet, so here are the main plugins.",
    );
    expect(detectionSummary(undefined, "libraries")).toBe(
      "We couldn't look through your libraries, so here are the main plugins.",
    );
  });
});
