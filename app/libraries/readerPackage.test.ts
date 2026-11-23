import { describe, expect, it } from "vitest";

import { cleanPackage } from "../../mobile/ios/Shisho/Reader/ReaderWeb/package.js";

// The Shisho app reads a book's original package document, which the
// server's generated copy would have rewritten; the app's reader tidies the
// quirks a browser's strict XML parser or foliate trip on (#919).
const DC = "http://purl.org/dc/elements/1.1/";

const parse = (text: string) =>
  new DOMParser().parseFromString(text, "application/xml");

const dcTitles = (text: string) =>
  Array.from(parse(text).getElementsByTagNameNS(DC, "title")).map(
    (el) => el.textContent,
  );

describe("cleanPackage", () => {
  it("drops whatever comes before the XML declaration", () => {
    const text = `\uFEFF\n  <?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf"><metadata xmlns:dc="${DC}"><dc:title>A</dc:title></metadata></package>`;
    const cleaned = cleanPackage(text);
    expect(cleaned.startsWith("<?xml")).toBe(true);
    expect(parse(cleaned).querySelector("parsererror")).toBeNull();
  });

  it("declares a dc prefix the document uses but never declares", () => {
    const text = `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf"><metadata><dc:title>A</dc:title><dc:language>en</dc:language></metadata></package>`;
    expect(dcTitles(cleanPackage(text))).toEqual(["A"]);
  });

  it("moves unprefixed Dublin Core elements into Dublin Core", () => {
    const text = `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf"><metadata dc="${DC}"><title>A</title><language>en</language><meta name="cover" content="c"/></metadata></package>`;
    const doc = parse(cleanPackage(text));
    expect(dcTitles(cleanPackage(text))).toEqual(["A"]);
    expect(doc.getElementsByTagNameNS(DC, "language")[0]?.textContent).toBe(
      "en",
    );
    expect(doc.getElementsByTagName("meta")).toHaveLength(1);
  });

  it("reads Dublin Core 1.0 as 1.1", () => {
    const text = `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf"><metadata xmlns:dc="http://purl.org/dc/elements/1.0/"><dc:title>A</dc:title></metadata></package>`;
    expect(dcTitles(cleanPackage(text))).toEqual(["A"]);
  });

  it("declares a prefix on a prefixed package element", () => {
    const text = `<?xml version="1.0"?><opf:package xmlns:opf="http://www.idpf.org/2007/opf"><opf:metadata><dc:title>A</dc:title></opf:metadata></opf:package>`;
    const cleaned = cleanPackage(text);
    expect(parse(cleaned).querySelector("parsererror")).toBeNull();
    expect(dcTitles(cleaned)).toEqual(["A"]);
  });

  it("declares a prefix on the package element, not in a comment before it", () => {
    const text = `<?xml version="1.0"?><!-- this doc uses <package> --><opf:package xmlns:opf="http://www.idpf.org/2007/opf"><opf:metadata><dc:title>A</dc:title></opf:metadata></opf:package>`;
    const cleaned = cleanPackage(text);
    expect(parse(cleaned).querySelector("parsererror")).toBeNull();
    expect(dcTitles(cleaned)).toEqual(["A"]);
  });

  it("keeps a processing instruction that only looks like a declaration", () => {
    const text = `<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="${DC}"><?xml-stylesheet href="a.css"?><metadata><dc:title>A</dc:title></metadata></package>`;
    expect(cleanPackage(text)).toBe(text);
  });

  it("leaves a well-formed package as it is", () => {
    const text = `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="${DC}"><metadata><dc:title>A</dc:title></metadata></package>`;
    expect(cleanPackage(text)).toBe(text);
  });
});
