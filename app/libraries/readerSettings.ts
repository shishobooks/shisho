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

export const LINE_SPACINGS = ["compact", "normal", "relaxed"] as const;
export type LineSpacing = (typeof LINE_SPACINGS)[number];

export const READER_MARGINS = ["small", "medium", "large"] as const;
export type ReaderMargins = (typeof READER_MARGINS)[number];

export const FONT_SIZE = { min: 50, max: 200, step: 5 } as const;

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
  // Percent of the book's own size.
  size: number;
  lineSpacing: LineSpacing;
  margins: ReaderMargins;
  justify: boolean;
  theme: ReaderTheme;
  layout: ReaderLayout;
}

export const DEFAULT_READER_SETTINGS: ReaderSettings = {
  font: BOOK_FONT,
  size: 100,
  lineSpacing: "normal",
  margins: "medium",
  justify: true,
  theme: "light",
  layout: "paginated",
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

const validSize = (value: unknown, fallback: number) => {
  if (typeof value !== "number" || !Number.isFinite(value)) return fallback;
  if (value < FONT_SIZE.min || value > FONT_SIZE.max) return fallback;
  return Math.round(value / FONT_SIZE.step) * FONT_SIZE.step;
};

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
    return {
      ...d,
      size: validSize(account?.viewer_reflowable_font_size, d.size),
      theme: oneOf(READER_THEMES, account?.viewer_reflowable_theme, d.theme),
      layout: oneOf(READER_LAYOUTS, account?.viewer_reflowable_flow, d.layout),
    };
  }
  return {
    font: validFont(stored.font),
    size: validSize(stored.size, d.size),
    lineSpacing: oneOf(LINE_SPACINGS, stored.lineSpacing, d.lineSpacing),
    margins: oneOf(READER_MARGINS, stored.margins, d.margins),
    justify: typeof stored.justify === "boolean" ? stored.justify : d.justify,
    theme: oneOf(READER_THEMES, stored.theme, d.theme),
    layout: oneOf(READER_LAYOUTS, stored.layout, d.layout),
  };
};

const LINE_HEIGHTS: Record<LineSpacing, string> = {
  compact: "1.2",
  normal: "1.45",
  relaxed: "1.75",
};

const THEME_COLORS: Record<
  ReaderTheme,
  { fg: string; bg: string; link: string | null }
> = {
  light: { fg: "#111111", bg: "#ffffff", link: null },
  sepia: { fg: "#5b4636", bg: "#f4ecd8", link: "#8a4b08" },
  dark: { fg: "#e8e8e8", bg: "#1a1a1a", link: "#8ab4f8" },
  black: { fg: "#c8c8c8", bg: "#000000", link: "#8ab4f8" },
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
      font-size: ${settings.size}% !important;
    }
    ${replaceBookColors}
    ${font}
    ${TEXT} {
      line-height: ${LINE_HEIGHTS[settings.lineSpacing]} !important;
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

// foliate's gap is the space beside the text as a share of the page, and its
// max-inline-size caps how long a line gets on a wide screen.
const MARGIN_LAYOUT: Record<
  ReaderMargins,
  { gap: string; maxInlineSize: string }
> = {
  small: { gap: "4%", maxInlineSize: "840px" },
  medium: { gap: "7%", maxInlineSize: "720px" },
  large: { gap: "12%", maxInlineSize: "600px" },
};

// The attributes set on foliate's renderer.
export const readerLayout = (settings: ReaderSettings) => ({
  flow: settings.layout,
  ...MARGIN_LAYOUT[settings.margins],
});
