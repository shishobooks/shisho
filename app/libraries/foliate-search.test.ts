import { describe, expect, it } from "vitest";

import { SEARCH_OPTIONS } from "@/hooks/useBookSearch";
import { searchMatcher } from "@/libraries/foliate/search.js";
import { textWalker } from "@/libraries/foliate/text-walker.js";

// The options the reader passes to foliate's search, run through foliate's
// own matcher, as view.search() does for each section.
const find = (html: string, query: string) => {
  const doc = new DOMParser().parseFromString(
    `<html lang="en"><body>${html}</body></html>`,
    "text/html",
  );
  const matcher = searchMatcher(textWalker, {
    defaultLocale: "en",
    ...SEARCH_OPTIONS,
  });
  return Array.from(matcher(doc, query), ({ excerpt }) => excerpt.match);
};

describe("book search options", () => {
  it("ignores case", () => {
    expect(find("<p>The Lantern and the LANTERN.</p>", "lantern")).toEqual([
      "Lantern",
      "LANTERN",
    ]);
  });

  it("ignores accents", () => {
    expect(find("<p>A café, then a cafe.</p>", "cafe")).toEqual([
      "café",
      "cafe",
    ]);
    expect(find("<p>A cafe.</p>", "café")).toEqual(["cafe"]);
  });

  it("matches a phrase across the book's markup", () => {
    expect(find("<p>an <em>old</em> lantern</p>", "old lantern")).toEqual([
      "old lantern",
    ]);
  });

  it("shows the whole phrase when it spans several pieces of markup", () => {
    expect(
      find(
        "<p>an <em>old</em> <strong>brass</strong> lantern</p>",
        "an old brass lantern",
      ),
    ).toEqual(["an old brass lantern"]);
  });

  it("matches inside words", () => {
    expect(find("<p>Lanterns glow.</p>", "lantern")).toEqual(["Lantern"]);
  });
});
