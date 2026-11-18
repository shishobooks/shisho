import { describe, expect, it, vi } from "vitest";

import { chapterPositions, type MarkerBook } from "./readerChapterMarkers";

const html = (body: string) =>
  new DOMParser().parseFromString(
    `<html><body>${body}</body></html>`,
    "text/html",
  );

// A book of three sections. foliate's section fractions run from 0 to 1,
// one more than there are sections.
const FRACTIONS = [0, 0.2, 0.6, 1];

const book = (
  docs: Record<number, Document> = {},
  { async = false } = {},
): MarkerBook & { loads: ReturnType<typeof vi.fn> } => {
  const loads = vi.fn((index: number) => Promise.resolve(docs[index]));
  const files = ["one.xhtml", "two.xhtml", "three.xhtml"];
  return {
    loads,
    resolveHref: (href: string) => {
      const [path, hash] = href.split("#");
      const index = files.indexOf(path);
      if (index < 0) return null;
      const resolved = {
        index,
        anchor: (doc: Document) => (hash ? doc.getElementById(hash) : 0),
      };
      return async ? Promise.resolve(resolved) : resolved;
    },
    sections: files.map((_, index) => ({
      createDocument: () => loads(index),
    })),
  };
};

describe("chapterPositions", () => {
  it("puts a chapter that starts a section at the section's start", async () => {
    const positions = await chapterPositions(
      [{ href: "two.xhtml" }, { href: "three.xhtml" }],
      book(),
      FRACTIONS,
    );
    expect(positions).toEqual([0.2, 0.6]);
  });

  it("places a chapter partway through a section by the text before it", async () => {
    // Two chapters of equal length in the second section.
    const doc = html(
      `<h2 id="a">A</h2><p>${"x".repeat(98)}</p><h2 id="b">B</h2><p>${"x".repeat(98)}</p>`,
    );
    const positions = await chapterPositions(
      [{ href: "two.xhtml#a" }, { href: "two.xhtml#b" }],
      book({ 1: doc }),
      FRACTIONS,
    );
    // Halfway through the section that runs from 0.2 to 0.6.
    expect(positions).toEqual([0.2, 0.4]);
  });

  it("reads each section's document once, however many chapters it holds", async () => {
    const doc = html(`<h2 id="a">A</h2><p>Text</p><h2 id="b">B</h2>`);
    const b = book({ 1: doc });
    await chapterPositions(
      [{ href: "two.xhtml#a" }, { href: "two.xhtml#b" }],
      b,
      FRACTIONS,
    );
    expect(b.loads).toHaveBeenCalledTimes(1);
  });

  it("skips headings with nowhere to go, and links it can't resolve", async () => {
    const positions = await chapterPositions(
      [{ href: "" }, { href: "missing.xhtml" }, { href: "two.xhtml" }],
      book(),
      FRACTIONS,
    );
    expect(positions).toEqual([0.2]);
  });

  it("puts a chapter whose anchor is gone at its section's start", async () => {
    const positions = await chapterPositions(
      [{ href: "three.xhtml#gone" }],
      book({ 2: html("<p>Text</p>") }),
      FRACTIONS,
    );
    expect(positions).toEqual([0.6]);
  });

  it("marks a place once when several entries point at it", async () => {
    // A part and its first chapter often start together.
    const positions = await chapterPositions(
      [{ href: "two.xhtml" }, { href: "two.xhtml" }, { href: "three.xhtml" }],
      book(),
      FRACTIONS,
    );
    expect(positions).toEqual([0.2, 0.6]);
  });

  it("merges entries too close together to tell apart on the bar", async () => {
    const positions = await chapterPositions(
      [{ href: "two.xhtml" }, { href: "three.xhtml" }],
      book(),
      [0, 0.2, 0.201, 1],
    );
    expect(positions).toEqual([0.2]);
  });

  it("lists positions in book order whatever the contents' order", async () => {
    const positions = await chapterPositions(
      [{ href: "three.xhtml" }, { href: "two.xhtml" }],
      book(),
      FRACTIONS,
    );
    expect(positions).toEqual([0.2, 0.6]);
  });

  it("works with an AZW3 book, whose links resolve later", async () => {
    const positions = await chapterPositions(
      [{ href: "two.xhtml" }],
      book({}, { async: true }),
      FRACTIONS,
    );
    expect(positions).toEqual([0.2]);
  });

  it("stops when it's no longer wanted", async () => {
    const b = book({ 1: html("<p id='a'>A</p>"), 2: html("<p id='b'>B</p>") });
    let wanted = true;
    const positions = chapterPositions(
      [{ href: "two.xhtml#a" }, { href: "three.xhtml#b" }],
      b,
      FRACTIONS,
      () => {
        const still = wanted;
        wanted = false;
        return !still;
      },
    );
    await expect(positions).resolves.toBeNull();
    expect(b.loads).toHaveBeenCalledTimes(1);
  });
});
