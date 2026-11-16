import { describe, expect, it } from "vitest";

import { footnoteText, isFootnoteReference } from "./readerFootnotes";

const EPUB_NS = "http://www.idpf.org/2007/ops";

const xhtml = (body: string) =>
  new DOMParser().parseFromString(
    `<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="${EPUB_NS}"><body>${body}</body></html>`,
    "application/xhtml+xml",
  );

const link = (doc: Document) => doc.querySelector("a")!;

describe("isFootnoteReference", () => {
  it("finds a link the book marks as a note reference", () => {
    const doc = xhtml(`<p>Text<a epub:type="noteref" href="#n1">1</a></p>`);
    expect(isFootnoteReference(link(doc))).toBe(true);
  });

  it("finds a link with the note reference role", () => {
    const doc = xhtml(`<p>Text<a role="doc-noteref" href="#n1">1</a></p>`);
    expect(isFootnoteReference(link(doc))).toBe(true);
  });

  it("finds an unmarked link set as a superscript", () => {
    const doc = xhtml(`<p>Text<sup><a href="#n1">1</a></sup></p>`);
    expect(isFootnoteReference(link(doc))).toBe(true);
  });

  it("finds an unmarked link around a superscript", () => {
    const doc = xhtml(`<p>Text<a href="#n1"><sup>1</sup></a></p>`);
    expect(isFootnoteReference(link(doc))).toBe(true);
  });

  it("leaves an ordinary link alone", () => {
    const doc = xhtml(`<p>Text <a href="#end">A link.</a></p>`);
    expect(isFootnoteReference(link(doc))).toBe(false);
  });

  it("leaves a note's link back to the text alone", () => {
    const doc = xhtml(
      `<p><sup><a epub:type="backlink" href="#r1">1</a></sup> The note.</p>`,
    );
    expect(isFootnoteReference(link(doc))).toBe(false);
  });
});

describe("footnoteText", () => {
  it("reads a marked note's paragraphs", () => {
    const doc = xhtml(
      `<aside epub:type="footnote" id="n1"><p>First part.</p><p>Second   part.</p></aside>`,
    );
    expect(footnoteText(doc.getElementById("n1"))).toEqual([
      "First part.",
      "Second part.",
    ]);
  });

  it("reads the block around a target that's inline", () => {
    const doc = xhtml(
      `<p><a id="n1" href="#r1">1.</a> The note's text.</p><p>Other text.</p>`,
    );
    expect(footnoteText(doc.getElementById("n1"))).toEqual([
      "1. The note's text.",
    ]);
  });

  it("reads the whole note when the target sits in its first paragraph", () => {
    const doc = xhtml(
      `<ol><li><p><a id="n1" href="#r1">1.</a> First part.</p><p>Second part.</p></li></ol>`,
    );
    expect(footnoteText(doc.getElementById("n1"))).toEqual([
      "1. First part.",
      "Second part.",
    ]);
  });

  it("leaves out the link back to the text", () => {
    const doc = xhtml(
      `<li id="n1"><p>The note. <a epub:type="backlink" href="#r1">↩</a></p></li>`,
    );
    expect(footnoteText(doc.getElementById("n1"))).toEqual(["The note."]);
  });

  it("reads a note with no paragraphs as one", () => {
    const doc = xhtml(`<li id="n1">Just text.</li>`);
    expect(footnoteText(doc.getElementById("n1"))).toEqual(["Just text."]);
  });

  it("finds no text without a target", () => {
    expect(footnoteText(null)).toEqual([]);
  });
});
