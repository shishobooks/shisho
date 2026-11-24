import { describe, expect, it } from "vitest";

import {
  recordBookAlignment as appRecordBookAlignment,
  bookCSS,
  bookLayout,
  fontFaces,
  type PageStyleSettings,
} from "../../mobile/ios/Shisho/Reader/ReaderWeb/styles.js";
import { fontFaceFor } from "./readerFonts";
import {
  DEFAULT_READER_SETTINGS,
  pageSettingsFor,
  readerCSS,
  readerLayout,
  type PageSettings,
  type ReaderSettings,
} from "./readerSettings";

// The Shisho app's reader styles the book as the web reader does, with the
// same settings (#920). Its page script can't import this TypeScript, so it
// repeats it in styles.js; these check the two agree.

// The theme colors the app sends with its settings, ReaderSettings.swift's
// copy of the web's.
const COLORS = {
  light: { fg: "#111111", bg: "#ffffff", link: null },
  sepia: { fg: "#5b4636", bg: "#f4ecd8", link: "#8a4b08" },
  dark: { fg: "#e8e8e8", bg: "#1a1a1a", link: "#8ab4f8" },
  black: { fg: "#c8c8c8", bg: "#000000", link: "#8ab4f8" },
} as const;

const FACES = [
  {
    src: "shisho-reader://app/fonts/libron/Libron-Regular.woff2",
    format: "woff2",
    weight: "normal",
    style: "normal",
  },
  {
    src: "shisho-reader://app/fonts/libron/Libron-Italic.woff2",
    format: "woff2",
    weight: "normal",
    style: "italic",
  },
];

// What the app sends the page for these settings.
const appSettings = (
  page: PageSettings,
  font: PageStyleSettings["font"],
): PageStyleSettings => ({
  theme: page.theme,
  layout: page.layout,
  pages: page.pages,
  ...COLORS[page.theme],
  size: page.size,
  forceFontSize: page.forceFontSize,
  lineSpacing: page.lineSpacing,
  margins: page.margins,
  justify: page.justify,
  font,
});

const squash = (css: string) => css.replace(/\s+/g, " ").trim();

const cases: Array<Partial<ReaderSettings>> = [
  {},
  { theme: "sepia", size: 21.5, lineSpacing: 1.8, margins: 3 },
  { theme: "dark", justify: false, forceFontSize: true },
  { theme: "black", pages: "one", layout: "scrolled", margins: 15 },
];

describe("the app reader's styles", () => {
  it.each(cases)("give the web reader's stylesheet for %o", (overrides) => {
    const page = pageSettingsFor(
      { ...DEFAULT_READER_SETTINGS, ...overrides },
      "light",
    );
    expect(squash(bookCSS(appSettings(page, null)))).toBe(
      squash(readerCSS(page)),
    );
    expect(bookLayout(appSettings(page, null))).toEqual(readerLayout(page));
  });

  it("give the web reader's stylesheet with a font chosen", () => {
    const page = pageSettingsFor(
      { ...DEFAULT_READER_SETTINGS, theme: "dark" },
      "light",
    );
    const font = { family: "Libron", fallback: "serif", faces: FACES };
    expect(squash(bookCSS(appSettings(page, font)))).toBe(
      squash(
        readerCSS(page, {
          family: "Libron",
          fallback: "serif",
          faces: fontFaces("Libron", FACES),
        }),
      ),
    );
  });

  it("declare a font's faces as the web reader does", () => {
    // The web's faces are at its own addresses; the rules are the same.
    const urls = (css: string) =>
      squash(css.replace(/url\("[^"]*\/([^/"]+)"\)/g, 'url("$1")'));
    const web = fontFaceFor("libron")?.faces ?? "";
    const app = fontFaces("Libron", [
      ...FACES,
      {
        src: "shisho-reader://app/fonts/libron/Libron-Bold.woff2",
        format: "woff2",
        weight: "bold",
        style: "normal",
      },
      {
        src: "shisho-reader://app/fonts/libron/Libron-BoldItalic.woff2",
        format: "woff2",
        weight: "bold",
        style: "italic",
      },
    ]);
    expect(urls(app)).toBe(urls(web));
  });

  it("keep the alignment a book gives a paragraph on purpose", () => {
    document.head.innerHTML = `<style>.break { text-align: center }</style>`;
    document.body.innerHTML = `<p>Body.</p><p class="break">* * *</p><p align="right">Signed.</p>`;
    document.documentElement.removeAttribute("data-shisho-align");
    appRecordBookAlignment(document);
    expect(
      [...document.querySelectorAll("[data-shisho-own-align]")].map(
        (el) => el.textContent,
      ),
    ).toEqual(["* * *", "Signed."]);
    expect(document.documentElement).toHaveAttribute("data-shisho-align");
  });
});
