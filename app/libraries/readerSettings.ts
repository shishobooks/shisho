import type { UserSettingsResponse } from "@/types";

// The ebook reader's display settings. The Shisho app's readers offer the
// same list with the same names and steps, so a change here changes what
// those specs promise too.
//
// They're kept per browser (localStorage), not per account: a phone and a
// desktop want different sizes. Nothing here reaches the server, so Reading
// Positions are unaffected.

export const READER_THEMES = ["light", "sepia", "dark", "black"] as const;
export type ReaderTheme = (typeof READER_THEMES)[number];

export const READER_LAYOUTS = ["paginated", "scrolled"] as const;
export type ReaderLayout = (typeof READER_LAYOUTS)[number];

// How many pages a paginated book shows at once. Auto shows two side by side
// when the reader is wider than tall and has room for both; One never does.
export const READER_PAGES = ["auto", "one"] as const;
export type ReaderPages = (typeof READER_PAGES)[number];

// The settings set by number, each with its range and step.
export const NUMERIC_SETTINGS = {
  // The text's base size in CSS pixels; the book's own sizes scale from it.
  size: { min: 10, max: 40, step: 0.5 },
  // A multiple of the font size.
  lineSpacing: { min: 1, max: 2.5, step: 0.05 },
  // The space on each side of the text, as a percent of the page's width.
  // At least 1: foliate leaves the same space between the two columns of a
  // spread, which would otherwise run together.
  margins: { min: 1, max: 15, step: 1 },
} as const;
export type NumericSetting = keyof typeof NUMERIC_SETTINGS;

// The size browsers give text that sets none, which the old percentage
// setting scaled.
const BROWSER_FONT_SIZE = 16;

// The book's own fonts.
export const BOOK_FONT = "book";
// An imported font is chosen as this prefix and its id in IndexedDB.
export const IMPORTED_FONT_PREFIX = "imported:";
export const BUNDLED_FONT_IDS = [
  "libron",
  "literata",
  "atkinson-hyperlegible",
  "noto-serif",
  "noto-sans",
  "opendyslexic",
] as const;
export type BundledFontId = (typeof BUNDLED_FONT_IDS)[number];

export interface ReaderSettings {
  // BOOK_FONT, a BundledFontId, or IMPORTED_FONT_PREFIX + an id.
  font: string;
  size: number;
  // Sets body text at `size` even where the book sets its own sizes.
  forceFontSize: boolean;
  lineSpacing: number;
  margins: number;
  justify: boolean;
  theme: ReaderTheme;
  layout: ReaderLayout;
  pages: ReaderPages;
}

export const DEFAULT_READER_SETTINGS: ReaderSettings = {
  font: BOOK_FONT,
  size: BROWSER_FONT_SIZE,
  forceFontSize: false,
  lineSpacing: 1.45,
  margins: 7,
  justify: true,
  theme: "light",
  layout: "paginated",
  pages: "auto",
};

// The settings accounts used to keep on the server, before they moved to the
// browser. A browser with nothing stored starts from them.
export type AccountReaderSettings = Partial<
  Pick<
    UserSettingsResponse,
    | "viewer_reflowable_font_size"
    | "viewer_reflowable_theme"
    | "viewer_reflowable_flow"
  >
>;

const oneOf = <T extends string>(
  options: readonly T[],
  value: unknown,
  fallback: T,
): T => (options.includes(value as T) ? (value as T) : fallback);

// The nearest step, without the float noise of 0.05 steps (1.4500000001).
const snap = (setting: NumericSetting, value: number) => {
  const { step } = NUMERIC_SETTINGS[setting];
  return Number((Math.round(value / step) * step).toFixed(2));
};

const validNumber = (setting: NumericSetting, value: unknown) => {
  const { min, max } = NUMERIC_SETTINGS[setting];
  const fallback = DEFAULT_READER_SETTINGS[setting];
  if (typeof value !== "number" || !Number.isFinite(value)) return fallback;
  if (value < min || value > max) return fallback;
  return snap(setting, value);
};

const validBoolean = (value: unknown, fallback: boolean) =>
  typeof value === "boolean" ? value : fallback;

const clamp = (setting: NumericSetting, value: number) => {
  const { min, max } = NUMERIC_SETTINGS[setting];
  return Math.min(max, Math.max(min, value));
};

// A setting one step up (1) or down (-1), held to its range.
export const stepSetting = (
  setting: NumericSetting,
  value: number,
  direction: 1 | -1,
) =>
  snap(
    setting,
    clamp(setting, value + direction * NUMERIC_SETTINGS[setting].step),
  );

const validFont = (value: unknown) => {
  if (typeof value !== "string") return BOOK_FONT;
  if (value === BOOK_FONT) return value;
  if (BUNDLED_FONT_IDS.includes(value as BundledFontId)) return value;
  if (
    value.startsWith(IMPORTED_FONT_PREFIX) &&
    value.length > IMPORTED_FONT_PREFIX.length
  ) {
    return value;
  }
  return BOOK_FONT;
};

const parse = (raw: string | null): Record<string, unknown> | null => {
  if (raw == null) return null;
  try {
    const value: unknown = JSON.parse(raw);
    return value && typeof value === "object"
      ? (value as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
};

// The settings from what this browser stored (raw localStorage text), or,
// when it stored nothing, from the account's old settings. A value it can't
// use falls back to its default, so a setting that later gains or loses an
// option never breaks the reader.
export const readerSettingsFrom = (
  raw: string | null,
  account?: AccountReaderSettings,
): ReaderSettings => {
  const d = DEFAULT_READER_SETTINGS;
  const stored = parse(raw);
  if (!stored) {
    // The account kept a percentage of the browser's default size.
    const percent = account?.viewer_reflowable_font_size;
    return {
      ...d,
      size:
        percent == null
          ? d.size
          : snap("size", clamp("size", (percent / 100) * BROWSER_FONT_SIZE)),
      theme: oneOf(READER_THEMES, account?.viewer_reflowable_theme, d.theme),
      layout: oneOf(READER_LAYOUTS, account?.viewer_reflowable_flow, d.layout),
    };
  }
  return {
    font: validFont(stored.font),
    size: validNumber("size", stored.size),
    forceFontSize: validBoolean(stored.forceFontSize, d.forceFontSize),
    lineSpacing: validNumber("lineSpacing", stored.lineSpacing),
    margins: validNumber("margins", stored.margins),
    justify: validBoolean(stored.justify, d.justify),
    theme: oneOf(READER_THEMES, stored.theme, d.theme),
    layout: oneOf(READER_LAYOUTS, stored.layout, d.layout),
    pages: oneOf(READER_PAGES, stored.pages, d.pages),
  };
};

// `link` replaces the book's link colors on the page (Light keeps them);
// `noteLink` is the link in a footnote popover.
const THEME_COLORS: Record<
  ReaderTheme,
  { fg: string; bg: string; link: string | null; noteLink: string }
> = {
  light: { fg: "#111111", bg: "#ffffff", link: null, noteLink: "#0645ad" },
  sepia: { fg: "#5b4636", bg: "#f4ecd8", link: "#8a4b08", noteLink: "#8a4b08" },
  dark: { fg: "#e8e8e8", bg: "#1a1a1a", link: "#8ab4f8", noteLink: "#8ab4f8" },
  black: { fg: "#c8c8c8", bg: "#000000", link: "#8ab4f8", noteLink: "#8ab4f8" },
};

// A font other than the book's: its @font-face rules and the family to set.
export interface ReaderFontFace {
  family: string;
  fallback: string;
  faces: string;
}

// The stylesheet foliate injects into each section of the book.
export const readerCSS = (
  settings: ReaderSettings,
  fontFace?: ReaderFontFace,
): string => {
  const { fg, bg, link } = THEME_COLORS[settings.theme];
  const dark = settings.theme === "dark" || settings.theme === "black";
  // Books ship their own stylesheets with more specific selectors, so the
  // reader's choices are `!important`, as foliate's own theming is
  // (`setStylesImportant` in paginator.js).
  //
  // Many books (Project Gutenberg's among them) set a white background and
  // black text on <body> or deeper, which would leave a white page inside a
  // dark reader. Every theme but Light replaces the book's colors below
  // <html>; Light keeps the book's own colors.
  const replaceBookColors = link
    ? `
      body {
        color: inherit !important;
        background: none !important;
      }
      body * {
        color: inherit !important;
        background-color: transparent !important;
      }
      a:any-link, a:any-link * {
        color: ${link} !important;
      }
      ${
        // Transparent images often hold black line art (diagrams, drop
        // caps) that would vanish on a dark page.
        dark ? "img, svg { background-color: #fff !important; }" : ""
      }
    `
    : "";
  const font = fontFace
    ? `
      ${fontFace.faces}
      body, body * {
        font-family: "${fontFace.family}", ${fontFace.fallback} !important;
      }
      code, pre, kbd, samp, tt, code *, pre *, kbd *, samp *, tt * {
        font-family: monospace !important;
      }
    `
    : "";
  return `
    @namespace epub "http://www.idpf.org/2007/ops";
    html {
      color: ${fg} !important;
      background: ${bg} !important;
      font-size: ${settings.size}px !important;
    }
    ${replaceBookColors}
    ${font}
    ${settings.forceFontSize ? FORCED_SIZE : ""}
    ${TEXT} {
      line-height: ${settings.lineSpacing} !important;
    }
    /* Only text the book leaves at the start or justified: a centered scene
       break or a right-aligned signature keeps its place. */
    html[${ALIGNMENT_RECORDED}] :is(${TEXT}):not([${OWN_ALIGNMENT}]) {
      text-align: ${settings.justify ? "justify" : "start"} !important;
      ${settings.justify ? "-webkit-hyphens: auto; hyphens: auto;" : ""}
    }
  `;
};

// The text the line spacing and justification settings apply to.
const TEXT = "p, li, blockquote, dd";

// Text at the chosen size, for a book whose own sizes (often in px) would
// ignore it: everything takes its parent's size, down from the body.
// Headings, small print (small, sup, sub) and code keep the book's sizes,
// and what's inside them takes theirs. Anything else the book sizes on
// purpose, such as a drop cap or a heading made of a styled paragraph, is
// flattened: that's what forcing the size means.
const FORCED_SIZE = `
  body {
    font-size: 1rem !important;
  }
  body :not(h1, h2, h3, h4, h5, h6, small, sup, sub, code, pre, kbd, samp, tt) {
    font-size: inherit !important;
  }
`;
const ALIGNMENT_RECORDED = "data-shisho-align";
const OWN_ALIGNMENT = "data-shisho-own-align";

// Marks the text in a section that the book aligns on purpose (centered, or
// against the far side), which the justification setting leaves alone. It
// runs as foliate loads the section, before the reader's alignment applies
// and before layout, so it reads the book's own alignment and costs no
// second layout.
export const recordBookAlignment = (doc: Document) => {
  const view = doc.defaultView ?? window;
  // foliate keeps a section's frame hidden until it lays it out, and Firefox
  // computes no style in a hidden frame, so show it for the reading, as
  // foliate itself does (paginator.js `load`).
  const frame = view.frameElement as HTMLElement | null;
  const display = frame?.style.display;
  if (frame) frame.style.display = "block";
  try {
    for (const el of doc.querySelectorAll(TEXT)) {
      const { textAlign, direction } = view.getComputedStyle(el);
      const far = direction === "rtl" ? "left" : "right";
      const aligned = [
        textAlign,
        // The align attribute, which not every engine reports as a style.
        el.closest("[align]")?.getAttribute("align")?.toLowerCase(),
      ];
      if (
        aligned.some((a) =>
          ["center", "-webkit-center", "end", far].includes(a ?? ""),
        )
      ) {
        el.setAttribute(OWN_ALIGNMENT, "");
      }
    }
  } finally {
    if (frame) frame.style.display = display ?? "";
  }
  doc.documentElement.setAttribute(ALIGNMENT_RECORDED, "");
};

// The attributes set on foliate's renderer. foliate leaves its whole gap on
// each side of the text, half outside the frame and half as padding inside
// it, and the same gap between two columns.
export const readerLayout = (settings: ReaderSettings) => ({
  flow: settings.layout,
  gap: `${settings.margins}%`,
  maxColumnCount: settings.pages === "one" ? "1" : "2",
});

// A footnote popover is part of the book, so it takes the page's colors and
// type, the text a little smaller as books set notes. `bookFontFamily` is the
// family the book itself uses around the reference.
export const footnoteStyle = (
  settings: ReaderSettings,
  fontFace: ReaderFontFace | undefined,
  bookFontFamily: string,
) => {
  const { fg, bg, noteLink } = THEME_COLORS[settings.theme];
  return {
    backgroundColor: bg,
    color: fg,
    link: noteLink,
    fontFamily: fontFace
      ? `"${fontFace.family}", ${fontFace.fallback}`
      : bookFontFamily,
    // To the half pixel, as the text size steps.
    fontSize: `${Math.round(settings.size * 0.9 * 2) / 2}px`,
    lineHeight: settings.lineSpacing,
  };
};
