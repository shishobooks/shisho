import { describe, expect, it } from "vitest";

import {
  DEFAULT_READER_SETTINGS,
  readerCSS,
  readerLayout,
  readerSettingsFrom,
  recordBookAlignment,
} from "./readerSettings";

describe("readerSettingsFrom", () => {
  it("uses the defaults when nothing is stored or saved", () => {
    expect(readerSettingsFrom(null)).toEqual(DEFAULT_READER_SETTINGS);
  });

  it("starts from the account's old reader settings until this browser stores its own", () => {
    expect(
      readerSettingsFrom(null, {
        viewer_reflowable_font_size: 130,
        viewer_reflowable_theme: "sepia",
        viewer_reflowable_flow: "scrolled",
      }),
    ).toEqual({
      ...DEFAULT_READER_SETTINGS,
      size: 130,
      theme: "sepia",
      layout: "scrolled",
    });
  });

  it("prefers what this browser stored over the account's old settings", () => {
    const stored = {
      ...DEFAULT_READER_SETTINGS,
      font: "literata",
      theme: "black",
    };
    expect(
      readerSettingsFrom(JSON.stringify(stored), {
        viewer_reflowable_font_size: 130,
        viewer_reflowable_theme: "sepia",
        viewer_reflowable_flow: "scrolled",
      }),
    ).toEqual(stored);
  });

  it("keeps every valid stored value", () => {
    const stored = {
      font: "imported:abc",
      size: 85,
      lineSpacing: "relaxed",
      margins: "large",
      justify: false,
      theme: "dark",
      layout: "scrolled",
    };
    expect(readerSettingsFrom(JSON.stringify(stored))).toEqual(stored);
  });

  it("replaces each value it doesn't understand with its default", () => {
    const stored = {
      font: "comic-sans",
      size: 47,
      lineSpacing: "huge",
      margins: 3,
      justify: "yes",
      theme: "purple",
      layout: "sideways",
    };
    expect(readerSettingsFrom(JSON.stringify(stored))).toEqual(
      DEFAULT_READER_SETTINGS,
    );
  });

  it("snaps a size between steps to the nearest step", () => {
    expect(
      readerSettingsFrom(
        JSON.stringify({ ...DEFAULT_READER_SETTINGS, size: 112 }),
      ).size,
    ).toBe(110);
  });

  it("ignores stored text that isn't JSON", () => {
    expect(readerSettingsFrom("{not json")).toEqual(DEFAULT_READER_SETTINGS);
  });
});

describe("readerCSS", () => {
  const css = (overrides = {}, fontFace?: Parameters<typeof readerCSS>[1]) =>
    readerCSS({ ...DEFAULT_READER_SETTINGS, ...overrides }, fontFace);

  it.each([
    ["compact", "1.2"],
    ["normal", "1.45"],
    ["relaxed", "1.75"],
  ])("sets %s line spacing", (lineSpacing, value) => {
    expect(css({ lineSpacing })).toContain(`line-height: ${value} !important`);
  });

  it("sets the size on the page once, so it doesn't compound", () => {
    const out = css({ size: 135 });
    expect(out.match(/font-size: 135%/g)).toHaveLength(1);
    expect(out).toMatch(/html \{[^}]*font-size: 135% !important/);
  });

  it("aligns only paragraphs the book doesn't align itself, once they're recorded", () => {
    expect(css({ justify: true })).toMatch(
      /html\[data-shisho-align\] :is\(p, li, blockquote, dd\):not\(\[data-shisho-own-align\]\) \{[^}]*text-align: justify !important/,
    );
  });

  it("justifies paragraphs, hyphenating them, when justification is on", () => {
    const out = css({ justify: true });
    expect(out).toContain("text-align: justify !important");
    expect(out).toContain("hyphens: auto");
  });

  it("aligns paragraphs to the start when justification is off", () => {
    const out = css({ justify: false });
    expect(out).toContain("text-align: start !important");
    expect(out).not.toContain("hyphens: auto");
  });

  it("keeps the book's own font when the font is the book's", () => {
    expect(css({ font: "book" })).not.toContain("font-family");
  });

  it("declares the chosen font's faces and sets it on the text", () => {
    const out = css(
      { font: "literata" },
      {
        family: "Literata",
        fallback: "serif",
        faces: '@font-face { font-family: "Literata"; }',
      },
    );
    expect(out).toContain('@font-face { font-family: "Literata"; }');
    expect(out).toMatch(
      /body \* \{\s*font-family: "Literata", serif !important/,
    );
  });

  it("keeps code in a monospace font under a chosen font", () => {
    const out = css(
      { font: "literata" },
      { family: "Literata", fallback: "serif", faces: "" },
    );
    expect(out).toMatch(/pre \*[^{]*\{\s*font-family: monospace !important/);
  });

  it.each([
    ["light", "#ffffff", "#111111"],
    ["sepia", "#f4ecd8", "#5b4636"],
    ["dark", "#1a1a1a", "#e8e8e8"],
    ["black", "#000000", "#c8c8c8"],
  ])("colors the page for the %s theme", (theme, bg, fg) => {
    const out = css({ theme });
    expect(out).toContain(`background: ${bg} !important`);
    expect(out).toContain(`color: ${fg} !important`);
  });

  it("keeps the book's own colors in the light theme", () => {
    expect(css({ theme: "light" })).not.toContain("color: inherit");
  });

  it.each(["sepia", "dark", "black"])(
    "replaces the book's colors in the %s theme",
    (theme) => {
      expect(css({ theme })).toContain("color: inherit !important");
    },
  );

  it.each(["dark", "black"])(
    "puts images on white in the %s theme, so transparent ones stay visible",
    (theme) => {
      expect(css({ theme })).toContain(
        "img, svg { background-color: #fff !important; }",
      );
    },
  );

  it.each(["light", "sepia"])(
    "leaves images alone in the %s theme",
    (theme) => {
      expect(css({ theme })).not.toContain("img, svg");
    },
  );
});

describe("readerLayout", () => {
  it("passes the layout to foliate's flow", () => {
    expect(
      readerLayout({ ...DEFAULT_READER_SETTINGS, layout: "scrolled" }).flow,
    ).toBe("scrolled");
  });

  it("widens the gap and shortens the lines as margins grow", () => {
    const small = readerLayout({
      ...DEFAULT_READER_SETTINGS,
      margins: "small",
    });
    const medium = readerLayout({
      ...DEFAULT_READER_SETTINGS,
      margins: "medium",
    });
    const large = readerLayout({
      ...DEFAULT_READER_SETTINGS,
      margins: "large",
    });
    const pct = (v: string) => parseFloat(v);
    expect(pct(small.gap)).toBeLessThan(pct(medium.gap));
    expect(pct(medium.gap)).toBeLessThan(pct(large.gap));
    expect(pct(small.maxInlineSize)).toBeGreaterThan(pct(medium.maxInlineSize));
    expect(pct(medium.maxInlineSize)).toBeGreaterThan(pct(large.maxInlineSize));
  });
});

describe("recordBookAlignment", () => {
  const section = (head: string, body: string) => {
    document.head.innerHTML = head;
    document.body.innerHTML = body;
    document.documentElement.removeAttribute("data-shisho-align");
    return document;
  };
  const own = (doc: Document) =>
    [...doc.querySelectorAll("[data-shisho-own-align]")].map(
      (el) => el.textContent,
    );

  it("keeps the alignment a book gives a paragraph on purpose", () => {
    const doc = section(
      `<style>.break { text-align: center } .sig { text-align: right }</style>`,
      `<p>Body.</p><p class="break">* * *</p><p class="sig">Signed.</p>
       <p align="center">Title</p><li>Item</li><p style="text-align: justify">Set.</p>`,
    );
    recordBookAlignment(doc);
    expect(own(doc)).toEqual(["* * *", "Signed.", "Title"]);
    expect(doc.documentElement).toHaveAttribute("data-shisho-align");
  });

  it("takes left as the far side in a right-to-left book", () => {
    const doc = section(
      `<style>.l { text-align: left } .r { text-align: right }</style>`,
      `<div dir="rtl"><p class="l">Left.</p><p class="r">Right.</p></div>`,
    );
    recordBookAlignment(doc);
    expect(own(doc)).toEqual(["Left."]);
  });
});
