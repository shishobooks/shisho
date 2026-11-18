import { describe, expect, it } from "vitest";

import {
  DEFAULT_READER_SETTINGS,
  footnoteStyle,
  readerCSS,
  readerLayout,
  readerSettingsFrom,
  recordBookAlignment,
  stepSetting,
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
      // 130% of the browser's 16px, the size those settings scaled.
      size: 21,
      theme: "sepia",
      layout: "scrolled",
    });
  });

  it("takes an old size below the smallest one as the smallest", () => {
    // 50% of 16px is 8px.
    expect(
      readerSettingsFrom(null, { viewer_reflowable_font_size: 50 }).size,
    ).toBe(10);
  });

  it("rounds an old percentage to the nearest half pixel", () => {
    expect(
      readerSettingsFrom(null, { viewer_reflowable_font_size: 140 }).size,
    ).toBe(22.5);
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
      font: "imported:3",
      size: 18.5,
      lineSpacing: 1.8,
      margins: 12,
      justify: false,
      forceFontSize: true,
      theme: "dark",
      layout: "scrolled",
      pages: "one",
    };
    expect(readerSettingsFrom(JSON.stringify(stored))).toEqual(stored);
  });

  it("replaces each value it doesn't understand with its default", () => {
    const stored = {
      font: "comic-sans",
      size: 47,
      lineSpacing: "relaxed",
      margins: 16,
      justify: "yes",
      forceFontSize: 1,
      theme: "purple",
      layout: "sideways",
      pages: "two",
    };
    expect(readerSettingsFrom(JSON.stringify(stored))).toEqual(
      DEFAULT_READER_SETTINGS,
    );
  });

  it.each([
    ["size", 17.3, 17.5],
    ["lineSpacing", 1.62, 1.6],
    ["margins", 4.4, 4],
  ])("snaps a %s between steps to the nearest step", (key, value, snapped) => {
    expect(
      readerSettingsFrom(
        JSON.stringify({ ...DEFAULT_READER_SETTINGS, [key]: value }),
      )[key as "size"],
    ).toBe(snapped);
  });

  it("ignores stored text that isn't JSON", () => {
    expect(readerSettingsFrom("{not json")).toEqual(DEFAULT_READER_SETTINGS);
  });
});

describe("readerCSS", () => {
  const css = (overrides = {}, fontFace?: Parameters<typeof readerCSS>[1]) =>
    readerCSS({ ...DEFAULT_READER_SETTINGS, ...overrides }, fontFace);

  it("sets the line spacing", () => {
    expect(css({ lineSpacing: 1.75 })).toContain(
      "line-height: 1.75 !important",
    );
  });

  it("sets the size on the page once, so it doesn't compound", () => {
    const out = css({ size: 18.5 });
    expect(out.match(/font-size: 18.5px/g)).toHaveLength(1);
    expect(out).toMatch(/html \{[^}]*font-size: 18.5px !important/);
  });

  it("leaves the book's own text sizes alone unless the size is forced", () => {
    expect(css({ forceFontSize: false })).not.toContain("font-size: inherit");
  });

  it("sets the body text at the chosen size when the size is forced", () => {
    expect(css({ forceFontSize: true })).toMatch(
      /body \{\s*font-size: 1rem !important/,
    );
  });

  it("sets every element but headings and small print at the body's size when forced", () => {
    const out = css({ forceFontSize: true });
    const kept = out.match(
      /body :not\(([^)]*)\) \{\s*font-size: inherit !important/,
    )![1];
    for (const tag of ["h1", "h6", "small", "sup", "sub", "code"]) {
      expect(kept.split(/,\s*/)).toContain(tag);
    }
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
  it("shows two pages side by side when there's room, by default", () => {
    expect(DEFAULT_READER_SETTINGS.pages).toBe("auto");
    expect(readerLayout(DEFAULT_READER_SETTINGS).maxColumnCount).toBe("2");
  });

  it("never shows more than one page when set to one", () => {
    expect(
      readerLayout({ ...DEFAULT_READER_SETTINGS, pages: "one" }).maxColumnCount,
    ).toBe("1");
  });

  it("passes the layout to foliate's flow", () => {
    expect(
      readerLayout({ ...DEFAULT_READER_SETTINGS, layout: "scrolled" }).flow,
    ).toBe("scrolled");
  });

  it("makes the margin on each side the chosen share of the page", () => {
    // foliate leaves its whole gap on each side of the text, half outside
    // the frame and half inside it.
    expect(readerLayout({ ...DEFAULT_READER_SETTINGS, margins: 6 }).gap).toBe(
      "6%",
    );
  });

  it("keeps a gutter between two columns at the smallest margins", () => {
    // foliate also uses the gap between the columns of a two-page spread.
    expect(
      parseFloat(readerLayout({ ...DEFAULT_READER_SETTINGS, margins: 1 }).gap),
    ).toBeGreaterThan(0);
    expect(readerSettingsFrom(JSON.stringify({ margins: 0 })).margins).toBe(
      DEFAULT_READER_SETTINGS.margins,
    );
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

describe("stepSetting", () => {
  it("steps a setting up and down by its step", () => {
    expect(stepSetting("size", 16, 1)).toBe(16.5);
    expect(stepSetting("lineSpacing", 1.45, -1)).toBe(1.4);
    expect(stepSetting("margins", 4, 1)).toBe(5);
  });

  it("stops at the ends of the range", () => {
    expect(stepSetting("size", 40, 1)).toBe(40);
    expect(stepSetting("margins", 1, -1)).toBe(1);
  });
});

describe("footnoteStyle", () => {
  const style = (
    overrides = {},
    fontFace?: Parameters<typeof footnoteStyle>[1],
  ) =>
    footnoteStyle(
      { ...DEFAULT_READER_SETTINGS, ...overrides },
      fontFace,
      "Georgia, serif",
    );

  it.each([
    ["light", "#ffffff", "#111111"],
    ["sepia", "#f4ecd8", "#5b4636"],
    ["dark", "#1a1a1a", "#e8e8e8"],
    ["black", "#000000", "#c8c8c8"],
  ])("colors a footnote like the %s page", (theme, bg, fg) => {
    expect(style({ theme })).toEqual(
      expect.objectContaining({ backgroundColor: bg, color: fg }),
    );
  });

  it("gives the note's link the theme's link color", () => {
    expect(style({ theme: "sepia" }).link).toBe("#8a4b08");
    expect(style({ theme: "light" }).link).toMatch(/^#/);
  });

  it("rounds a note's size to a half pixel", () => {
    // 90% of 16px.
    expect(style({ size: 16 }).fontSize).toBe("14.5px");
  });

  it("sets a note a little smaller than the text, at its line spacing", () => {
    expect(style({ size: 20, lineSpacing: 1.6 })).toEqual(
      expect.objectContaining({ fontSize: "18px", lineHeight: 1.6 }),
    );
  });

  it("sets a note in the chosen font", () => {
    expect(
      style({}, { family: "Literata", fallback: "serif", faces: "" })
        .fontFamily,
    ).toBe('"Literata", serif');
  });

  it("sets a note in the book's font when that's the choice", () => {
    expect(style({}).fontFamily).toBe("Georgia, serif");
  });
});
