import { IDBFactory } from "fake-indexeddb";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  BUNDLED_FONTS,
  fontFaceFor,
  FontImportError,
  importFont,
  listImportedFonts,
  removeImportedFont,
} from "./readerFonts";
import { BUNDLED_FONT_IDS } from "./readerSettings";

const fontFile = (name: string, bytes = [0, 1, 0, 0]) =>
  new File([new Uint8Array(bytes)], name);

beforeEach(() => {
  // A fresh, empty database for every test.
  vi.stubGlobal("indexedDB", new IDBFactory());
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("bundled fonts", () => {
  it("bundles every font the settings offer, under its own name", () => {
    expect(BUNDLED_FONTS.map((f) => f.id)).toEqual([...BUNDLED_FONT_IDS]);
    expect(BUNDLED_FONTS.map((f) => f.name)).toEqual([
      "Libron",
      "Literata",
      "Atkinson Hyperlegible",
      "Noto Serif",
      "Noto Sans",
      "OpenDyslexic",
    ]);
  });

  it("declares a bundled font's faces at absolute addresses, which a book's frame can load", () => {
    const face = fontFaceFor("opendyslexic", []);
    expect(face?.family).toBe("OpenDyslexic");
    const urls = [...face!.faces.matchAll(/url\("([^"]+)"\)/g)].map(
      (m) => m[1],
    );
    expect(urls).toHaveLength(4);
    for (const url of urls) {
      expect(url).toMatch(/^https?:\/\//);
      expect(url).toMatch(/OpenDyslexic.*\.woff2/);
    }
    expect(face!.faces).toContain("font-style: italic");
    expect(face!.faces).toContain("font-weight: bold");
  });

  it("declares a variable font across its weights", () => {
    const face = fontFaceFor("noto-sans", []);
    expect(face!.faces).toContain("font-weight: 100 900");
    expect(face!.faces).toContain('format("truetype")');
  });

  it("declares nothing for the book's own font", () => {
    expect(fontFaceFor("book", [])).toBeUndefined();
  });
});

describe("imported fonts", () => {
  it("keeps an imported font in the browser", async () => {
    const font = await importFont(fontFile("My Font-Regular.ttf"));
    expect(font.name).toBe("My Font-Regular");
    expect(font.format).toBe("truetype");

    const listed = await listImportedFonts();
    expect(listed).toHaveLength(1);
    expect(listed[0].id).toBe(font.id);
    expect(new Uint8Array(listed[0].data)).toEqual(
      new Uint8Array([0, 1, 0, 0]),
    );
  });

  it.each([
    ["a.otf", "opentype"],
    ["a.OTF", "opentype"],
    ["a.woff2", "woff2"],
  ])("accepts %s", async (name, format) => {
    expect((await importFont(fontFile(name))).format).toBe(format);
  });

  it.each(["a.woff", "a.zip", "font"])("refuses %s", async (name) => {
    await expect(importFont(fontFile(name))).rejects.toThrow(
      "Choose a TTF, OTF, or WOFF2 font file.",
    );
    expect(await listImportedFonts()).toEqual([]);
  });

  it("refuses a file the browser can't read as a font", async () => {
    vi.stubGlobal(
      "FontFace",
      class {
        load() {
          return Promise.reject(new Error("bad font"));
        }
      },
    );
    const refusal = importFont(fontFile("broken.ttf"));
    await expect(refusal).rejects.toThrow(
      "broken.ttf isn't a font this browser can read.",
    );
    await expect(refusal).rejects.toBeInstanceOf(FontImportError);
    expect(await listImportedFonts()).toEqual([]);
  });

  it("imports where the page isn't a secure context, which has no randomUUID", async () => {
    vi.stubGlobal("crypto", {});
    await importFont(fontFile("Mine.ttf"));
    expect(await listImportedFonts()).toHaveLength(1);
  });

  it("lists imported fonts in the order they were added", async () => {
    const a = await importFont(fontFile("A.ttf"));
    const b = await importFont(fontFile("B.otf"));
    expect((await listImportedFonts()).map((f) => f.id)).toEqual([a.id, b.id]);
  });

  it("removes an imported font", async () => {
    const a = await importFont(fontFile("A.ttf"));
    const b = await importFont(fontFile("B.otf"));
    await removeImportedFont(a.id);
    expect((await listImportedFonts()).map((f) => f.id)).toEqual([b.id]);
  });

  it("declares an imported font's face from its address in this page", async () => {
    const font = await importFont(fontFile("Mine.woff2"));
    const face = fontFaceFor(`imported:${font.id}`, [
      { font, url: "blob:http://localhost/abc" },
    ]);
    expect(face?.faces).toContain('url("blob:http://localhost/abc")');
    expect(face?.faces).toContain('format("woff2")');
    expect(face?.family).not.toBe("Mine");
  });

  it("declares nothing for an imported font that's gone", () => {
    expect(fontFaceFor("imported:missing", [])).toBeUndefined();
  });
});
