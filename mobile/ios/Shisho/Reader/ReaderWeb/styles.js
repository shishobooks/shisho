// The web reader's display settings as the page styles the book with them:
// readerCSS, recordBookAlignment, and readerLayout from
// app/libraries/readerSettings.ts, and the @font-face rules from
// app/libraries/readerFonts.ts. That's TypeScript and can't be imported
// here, so app/libraries/readerStyles.test.ts checks the two give the same
// stylesheet: a change there changes this too.
//
// s is ReaderSettings.script from the app: the settings, the theme's colors
// (fg, bg, link), and font, the chosen font's family, fallback, and faces,
// or null for the book's own fonts.

// The text the line spacing and justification settings apply to.
const TEXT = "p, li, blockquote, dd";
const ALIGNMENT_RECORDED = "data-shisho-align";
const OWN_ALIGNMENT = "data-shisho-own-align";

// Text at the chosen size, for a book whose own sizes would ignore it:
// everything takes its parent's size, down from the body, except headings,
// small print, and code.
const FORCED_SIZE = `
  body {
    font-size: 1rem !important;
  }
  body :not(h1, h2, h3, h4, h5, h6, small, sup, sub, code, pre, kbd, samp, tt) {
    font-size: inherit !important;
  }
`;

// The @font-face rules for a font's faces, at the reader's own addresses.
export const fontFaces = (family, faces) =>
  faces
    .map(
      (face) => `@font-face {
        font-family: "${family}";
        src: url("${face.src}") format("${face.format}");
        font-weight: ${face.weight};
        font-style: ${face.style};
        font-display: block;
      }`,
    )
    .join("\n");

// The stylesheet foliate injects into each section of the book.
export const bookCSS = (s) => {
  const dark = s.theme === "dark" || s.theme === "black";
  // Every theme but Light replaces the book's colors below <html>.
  const replaceBookColors = s.link
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
        color: ${s.link} !important;
      }
      ${dark ? "img, svg { background-color: #fff !important; }" : ""}
    `
    : "";
  const font = s.font
    ? `
      ${fontFaces(s.font.family, s.font.faces)}
      body, body * {
        font-family: "${s.font.family}", ${s.font.fallback} !important;
      }
      code, pre, kbd, samp, tt, code *, pre *, kbd *, samp *, tt * {
        font-family: monospace !important;
      }
    `
    : "";
  return `
    @namespace epub "http://www.idpf.org/2007/ops";
    html {
      color: ${s.fg} !important;
      background: ${s.bg} !important;
      font-size: ${s.size}px !important;
    }
    ${replaceBookColors}
    ${font}
    ${s.forceFontSize ? FORCED_SIZE : ""}
    ${TEXT} {
      line-height: ${s.lineSpacing} !important;
    }
    /* Only text the book leaves at the start or justified: a centered scene
       break or a right-aligned signature keeps its place. */
    html[${ALIGNMENT_RECORDED}] :is(${TEXT}):not([${OWN_ALIGNMENT}]) {
      text-align: ${s.justify ? "justify" : "start"} !important;
      ${s.justify ? "-webkit-hyphens: auto; hyphens: auto;" : ""}
    }
  `;
};

// The attributes set on foliate's renderer: the gap is the margin on each
// side and between two pages of a spread.
export const bookLayout = (s) => ({
  flow: s.layout,
  gap: `${s.margins}%`,
  maxColumnCount: s.pages === "one" ? "1" : "2",
});

// Marks the text in a section that the book aligns on purpose (centered, or
// against the far side), which the justification setting leaves alone. It
// runs as foliate loads the section, before the reader's alignment applies.
export const recordBookAlignment = (doc) => {
  const view = doc.defaultView ?? window;
  // foliate keeps a section's frame hidden until it lays it out; show it for
  // the reading, as foliate itself does.
  const frame = view.frameElement;
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
