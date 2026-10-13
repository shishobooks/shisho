import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

import { removeUnresolvedSrcset } from "@/libraries/foliate/mobi.js";

const parse = (body: string) =>
  new DOMParser().parseFromString(
    `<html xmlns="http://www.w3.org/1999/xhtml"><body>${body}</body></html>`,
    "application/xhtml+xml",
  );

describe("removeUnresolvedSrcset", () => {
  it("drops a srcset left with the EPUB's relative paths", () => {
    const doc = parse(
      `<img src="blob:http://x/1" srcset="../images/a-2x.png 2x, ../images/a.png 1x"/>`,
    );
    removeUnresolvedSrcset(doc);
    const img = doc.querySelector("img")!;
    expect(img.hasAttribute("srcset")).toBe(false);
    expect(img.getAttribute("src")).toBe("blob:http://x/1");
  });

  it("keeps a srcset whose candidates were all resolved", () => {
    const srcset = "blob:http://x/2 2x, blob:http://x/1 1x";
    const doc = parse(`<img src="blob:http://x/1" srcset="${srcset}"/>`);
    removeUnresolvedSrcset(doc);
    expect(doc.querySelector("img")!.getAttribute("srcset")).toBe(srcset);
  });

  it("keeps a resolved srcset holding a data URL", () => {
    const srcset = "data:image/png;base64,AAAA 1x, blob:http://x/2 2x";
    const doc = parse(`<img src="blob:http://x/1" srcset="${srcset}"/>`);
    removeUnresolvedSrcset(doc);
    expect(doc.querySelector("img")!.getAttribute("srcset")).toBe(srcset);
  });

  it("keeps a resolved srcset with a trailing comma", () => {
    const srcset = "blob:http://x/1 1x, blob:http://x/2 2x,";
    const doc = parse(`<img src="blob:http://x/1" srcset="${srcset}"/>`);
    removeUnresolvedSrcset(doc);
    expect(doc.querySelector("img")!.getAttribute("srcset")).toBe(srcset);
  });

  it("drops an unresolved srcset on a picture source", () => {
    const doc = parse(
      `<picture><source srcset="../images/a.webp"/><img src="blob:http://x/1"/></picture>`,
    );
    removeUnresolvedSrcset(doc);
    expect(doc.querySelector("source")!.hasAttribute("srcset")).toBe(false);
  });
});

// The vendored mobi.js carries local patches that a refresh from upstream
// would silently drop. See "Local patches" in app/libraries/foliate/README.md.
describe("vendored mobi.js patches", () => {
  const source = readFileSync("app/libraries/foliate/mobi.js", "utf8");

  it("runs removeUnresolvedSrcset on every KF8 section", () => {
    expect(source).toMatch(
      /async loadSection\(section\) \{[\s\S]*?removeUnresolvedSrcset\(doc\)[\s\S]*?\n {4}\}/,
    );
  });

  it("gives KF8 resource blobs their MIME type", () => {
    expect(source).toContain("new Blob([newData], { type: newType })");
  });
});
