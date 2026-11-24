// The Shisho app's bridge to the web reader's vendored foliate-js (ADR 0026).
// The app runs three of these pages, the current page and one on either
// side. Native code owns every page turn and gesture; this lays the book
// out, keeps foliate's own touch handling out of the way, and answers
// questions about links, notes, selection, and where things are. Native
// calls window.R; this answers through the "bridge" message handler, one
// JSON string per message.
//
// Some rules follow the web reader's, which is TypeScript and can't be
// imported here: the display settings' stylesheet (styles.js, checked
// against app/libraries/readerSettings.ts),
// footnotes (app/libraries/readerFootnotes.ts), chapter marks
// (app/libraries/readerChapterMarks.ts), and the end of the book
// (app/utils/readerEnd.ts). A change there changes these too.
import "../foliate/view.js";

import { cleanPackage } from "./package.js";
import { stepPull } from "./pull.js";
import { bookCSS, bookLayout, recordBookAlignment } from "./styles.js";

const post = (msg) =>
  window.webkit?.messageHandlers?.bridge?.postMessage(JSON.stringify(msg));
const view = document.getElementById("view");
const raf2 = () =>
  new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
const wait = (ms) => new Promise((r) => setTimeout(r, ms));

window.addEventListener("error", (e) =>
  post({ type: "error", message: String(e.message) }),
);
window.addEventListener("unhandledrejection", (e) =>
  post({ type: "error", message: String(e.reason?.stack ?? e.reason) }),
);

// While true, moves come from opening or syncing, not from the reader.
let quiet = true;

// MARK: - Styles

// The web reader's stylesheet (styles.js), with what only the app's page
// needs: no tap highlight, and no text inflation on rotation.
const css = (s) => `
      * { -webkit-tap-highlight-color: transparent; }
      html { -webkit-text-size-adjust: 100% !important; }
      ${bookCSS(s)}`;

// The space above and below the text, inside the page.
const TEXT_BAND = "16px";

// relayout, for the page showing: a neighbor waits for its fonts before it
// says it's ready, and a late relayout would move it off its page.
const applyStyles = (s, relayout = true) => {
  document.documentElement.style.background = s.bg;
  const renderer = view.renderer;
  if (!renderer) return;
  renderer.setStyles?.(css(s));
  const layout = bookLayout(s);
  // Setting an attribute lays the book out again even when it's unchanged.
  const set = (name, value) => {
    if (renderer.getAttribute(name) === value) return false;
    renderer.setAttribute(name, value);
    return true;
  };
  const gap = set("gap", layout.gap);
  const flow = set("flow", layout.flow);
  const columns = set("max-column-count", layout.maxColumnCount);
  // foliate keeps a band above and below the text for running heads, 48 px
  // unless told otherwise. The app draws none, and its own readout has room
  // under the page, so the band shrinks to a little air. (The web reader
  // keeps it: its search panel hangs into the top band.)
  const band = set("margin", TEXT_BAND);
  // The paginator only sets a style for these, which a section already laid
  // out doesn't notice; a new flow renders itself.
  if ((gap || columns || band) && !flow) renderer.render?.();
  if (relayout) relayoutWhenFontLoads(s);
};

// The place the last settings change pinned, and the page's moves then. A
// run of changes keeps pinning the same place until the reader moves:
// pinning each time to the start of the page the last change landed on
// would walk back a little with each one.
let pinnedPlace = null;

// A section already on screen lays out before a newly chosen font arrives,
// so it lays out again once the font is in, as the web reader does. foliate
// keeps the place across it.
let fontWait = 0;
// Bumped each time native moves this page as a neighbor: a font that
// arrives after leaves it alone.
let moves = 0;
const relayoutWhenFontLoads = (s) => {
  const wait = ++fontWait;
  const moved = moves;
  if (!s.font) return;
  const family = `1em "${s.font.family}"`;
  for (const { doc } of view.renderer?.getContents?.() ?? []) {
    if (doc?.fonts?.check?.(family) !== false) continue;
    doc.fonts
      .load(family)
      .then(() => {
        if (wait === fontWait && moved === moves) view.renderer?.render?.();
      })
      .catch(() => {
        // The page keeps the fallback font it already shows.
      });
  }
};

// MARK: - Where the page is

// A fixed-layout book is paged whatever the layout setting.
const isScrolled = () =>
  !view.isFixedLayout && view.renderer?.getAttribute("flow") === "scrolled";
const currentContents = () => view.renderer?.getContents?.()?.[0] ?? null;
const currentDoc = () => currentContents()?.doc ?? null;
// foliate's fixed-layout renderer doesn't give its pages' sections in
// getContents, but knows the one showing.
const currentIndex = () =>
  view.isFixedLayout
    ? (view.renderer?.index ?? null)
    : (currentContents()?.index ?? null);

// The renderer's page getters throw before it has shown a section, as in a
// neighbor that has only opened the book.
const geometry = () => {
  try {
    if (!currentDoc()) return { page: null, pages: null };
    // A fixed-layout book's pages are its sections, one place each.
    if (view.isFixedLayout) return { page: 0, pages: 1 };
    return { page: view.renderer.page, pages: view.renderer.pages };
  } catch {
    return { page: null, pages: null };
  }
};

// Whether the reader is at the end of the book, as the web reader decides
// it (app/utils/readerEnd.ts): foliate's own test paginated, the bottom of
// the last section scrolled.
const atEnd = () => {
  const r = view.renderer;
  if (!r) return false;
  try {
    if (!r.scrolled) return !!r.atEnd;
    const sections = view.book?.sections ?? [];
    let last = sections.length - 1;
    while (last > 0 && sections[last].linear === "no") last--;
    if (currentIndex() < last) return false;
    return r.viewSize - (r.start + r.size) <= 2;
  } catch {
    return false;
  }
};

// The links on the current page, in the WebView's points, so native can
// hit-test a tap without a round trip: [x, y, width, height, id, note] per
// box, note 1 for a note reference.
let linkElements = [];
const linkRects = () => {
  linkElements = [];
  if (isScrolled()) return [];
  const out = [];
  // Every frame showing: a fixed-layout spread has two, each scaled to fit.
  for (const { doc } of view.renderer?.getContents?.() ?? []) {
    const frame = doc?.defaultView?.frameElement;
    if (!frame) continue;
    const fr = frame.getBoundingClientRect();
    const scale = frame.offsetWidth > 0 ? fr.width / frame.offsetWidth : 1;
    for (const a of doc.querySelectorAll("a[href]")) {
      let id = -1;
      let note = null;
      for (const r of a.getClientRects()) {
        const x = fr.left + r.left * scale;
        const width = r.width * scale;
        if (width === 0 || x + width < 0 || x > innerWidth) continue;
        if (id < 0) id = linkElements.push(a) - 1;
        // Not a superscript citation of a web page, which opens a browser.
        note ??=
          isNoteReference(a) &&
          !view.book.isExternal?.(
            resolveLink(a, sectionOfDoc.get(doc) ?? currentIndex()),
          )
            ? 1
            : 0;
        out.push([
          Math.round(x),
          Math.round(fr.top + r.top * scale),
          Math.round(width),
          Math.round(r.height * scale),
          id,
          note,
        ]);
      }
    }
  }
  return out;
};

// Where the page showing starts along the book, from 0 to 1. foliate's
// fraction is where the page ends, which on a chapter's last page is where
// the next chapter starts, so the chapter showing is found from this.
const pageStart = (index, page, pages) => {
  const fractions = view.getSectionFractions?.() ?? [];
  if (index == null || fractions[index] === undefined) return null;
  const from = fractions[index];
  const to = fractions[index + 1] ?? 1;
  let within = 0;
  try {
    const r = view.renderer;
    if (r.scrolled) within = r.viewSize > 0 ? r.start / r.viewSize : 0;
    else if (pages > 2) within = (page - 1) / (pages - 2);
  } catch {
    within = 0;
  }
  return from + Math.min(1, Math.max(0, within)) * (to - from);
};

// The section showing as a share of the book, and the page showing in it
// from 1, for counting pages left. Paginated or fixed-layout only: scrolled,
// a page isn't a page.
const sectionPlace = (index, page, pages) => {
  const fractions = view.getSectionFractions?.() ?? [];
  if (index == null || fractions[index] === undefined || page == null)
    return null;
  try {
    if (view.renderer?.scrolled) return null;
  } catch {
    return null;
  }
  const from = fractions[index];
  const to = fractions[index + 1] ?? 1;
  // The paginator counts a blank page at each end of a section.
  if (view.isFixedLayout) return { from, to, page: 1, pages: 1 };
  const textPages = Math.max(1, pages - 2);
  return {
    from,
    to,
    page: Math.min(Math.max(1, page), textPages),
    pages: textPages,
  };
};

// Where the chapters start, as the contents placed them, resolved once the
// book is open and grouped by section: an anchor function each, as foliate
// resolves an href to. Native sends them to every page. An EPUB file's
// sections are resolved once each, not once per chapter: foliate's
// resolveHref searches the manifest and spine, and contents can run to tens
// of thousands of entries.
let chapterHrefs = [];
let chapterTargets = null;
const resolvedChapters = () => {
  if (chapterTargets || !view.book) return chapterTargets ?? new Map();
  chapterTargets = new Map();
  const files = new Map();
  const resolve = (href) => {
    const resolved = view.book.resolveHref(href);
    // A KF8 MOBI resolves later; its chapters are counted the estimated way.
    return resolved && !(resolved instanceof Promise) && resolved.index >= 0
      ? resolved
      : null;
  };
  for (const href of chapterHrefs) {
    try {
      const [path, hash] = href.split("#");
      let target;
      if (hash && view.book.getTOCFragment) {
        if (!files.has(path)) files.set(path, resolve(path)?.index ?? null);
        const index = files.get(path);
        target =
          index == null
            ? null
            : { index, anchor: (doc) => view.book.getTOCFragment(doc, hash) };
      } else {
        // A MOBI's own targets, as filepos:500, carry their place without
        // a #.
        target = resolve(href);
      }
      if (!target) continue;
      if (!chapterTargets.has(target.index))
        chapterTargets.set(target.index, []);
      chapterTargets.get(target.index).push(target.anchor);
    } catch {
      // A chapter this copy doesn't have.
    }
  }
  return chapterTargets;
};

// The pages the chapters in the section showing start on, laid out as it is
// now: the same until the section or its layout changes, and dropped when
// the page lays out again in place.
let chapterPages = null;
const chapterPagesIn = (index, doc, r) => {
  const key = `${index} ${r.size} ${r.pages} ${view.book?.dir}`;
  if (chapterPages?.doc === doc && chapterPages.key === key)
    return chapterPages.pages;
  const pages = [];
  for (const anchor of resolvedChapters().get(index) ?? []) {
    // A chapter that starts the section, anchored at 0, is on its first
    // page.
    const node = anchor?.(doc);
    if (!node || typeof node !== "object") continue;
    const page = r.pageOf?.(node);
    if (page == null) continue;
    pages.push({
      page,
      atTop:
        node.getBoundingClientRect().top <
        (doc.defaultView?.innerHeight ?? 0) * 0.15,
    });
  }
  pages.sort((a, b) => a.page - b.page);
  chapterPages = { doc, key, pages };
  return pages;
};

// The first page after the one showing that a chapter starts on, in the
// section showing, as foliate pages it (the paginator's #scrollToRect), and
// whether it starts at the page's top. Paginated only; null when the next
// chapter is in a later section. Exact where the chapters' share of the text
// isn't, as on pages that are a photo.
const nextChapterPage = (index, page) => {
  try {
    const r = view.renderer;
    if (view.isFixedLayout || r?.scrolled || page == null || !r.size)
      return null;
    const doc = currentDoc();
    if (!doc) return null;
    return chapterPagesIn(index, doc, r).find((c) => c.page > page) ?? null;
  } catch {
    return null;
  }
};

const location = () => {
  const d = view.lastLocation;
  const { page, pages } = geometry();
  const index = d?.section?.current ?? currentIndex();
  return {
    section: sectionPlace(index, page, pages),
    nextChapter: nextChapterPage(index, page),
    cfi: d?.cfi ?? null,
    fraction: Number.isFinite(d?.fraction) ? d.fraction : null,
    start: pageStart(index, page, pages),
    index,
    page,
    pages,
    atEnd: atEnd(),
    // Reading time counts per screen, as on the web.
    screen: `${index}:${page}`,
    links: linkRects(),
    images: imageRects(),
  };
};

// Resolves once the page is really painted: fonts loaded (foliate lays out
// again on fonts.ready), the images on this page decoded, then two frames so
// the layers have been committed. A neighbor is ready only after this.
const settle = async () => {
  const doc = currentDoc();
  if (doc) {
    await Promise.race([doc.fonts.ready, wait(1500)]);
    const fr = doc.defaultView?.frameElement?.getBoundingClientRect();
    const images = fr
      ? Array.from(doc.images).filter((img) => {
          const r = img.getBoundingClientRect();
          const x = fr.left + r.left;
          return x + r.width > 0 && x < innerWidth;
        })
      : [];
    if (images.length)
      await Promise.race([
        Promise.all(images.map((img) => img.decode().catch(() => {}))),
        wait(800),
      ]);
  }
  await raf2();
};

// The section the book opened in. Scrolling a little on its first screen
// isn't reading yet, as on the web (app/utils/readerEnd.ts
// withinFirstScreen), so it's reported as a quiet move.
let openedSection = null;
const withinFirstScreen = () => {
  const r = view.renderer;
  return !!r?.scrolled && (r.start ?? 0) < (r.size ?? 0);
};

// Every page reports its moves; native listens to the one showing, whose
// role changes with each turn.
view.addEventListener("relocate", (e) => {
  // foliate's fixed-layout renderer gives no reason for a jump; it's a move
  // like any other.
  const reason =
    e.detail?.reason ?? (view.isFixedLayout && !quiet ? "navigation" : null);
  // Laid out again in place, as once fonts or images load: where the
  // chapters fall can move without the page count changing.
  if (reason === "anchor") chapterPages = null;
  // The reader moved: the next settings change pins the place anew.
  if (!quiet && reason && reason !== "anchor") pinnedPlace = null;
  const firstScreen =
    reason === "scroll" &&
    currentIndex() === openedSection &&
    withinFirstScreen();
  post({
    type: "relocate",
    loc: location(),
    reason,
    quiet: quiet || firstScreen,
  });
  if (!quiet) preloadNotes();
});

// MARK: - Notes and links

const EPUB_NS = "http://www.idpf.org/2007/ops";
const tokens = (value) => new Set(value?.split(/\s+/).filter(Boolean));
const epubTypes = (el) => tokens(el?.getAttributeNS?.(EPUB_NS, "type"));
const roles = (el) => tokens(el?.getAttribute?.("role"));
const isBacklink = (el) =>
  epubTypes(el).has("backlink") || roles(el).has("doc-backlink");
const isSuper = (el) => {
  if (!el) return false;
  if (el.matches("sup")) return true;
  const v = el.ownerDocument?.defaultView;
  if (!v) return false;
  const align = v.getComputedStyle(el).verticalAlign;
  return (
    align === "super" ||
    align === "top" ||
    align === "text-top" ||
    /^\d/.test(align)
  );
};

// A link the book marks as a note reference or, for a book that marks
// nothing, a link set as a superscript.
const isNoteReference = (a) => {
  const types = epubTypes(a);
  const linkRoles = roles(a);
  if (
    ["doc-noteref", "doc-biblioref", "doc-glossref"].some((r) =>
      linkRoles.has(r),
    )
  )
    return true;
  if (["noteref", "biblioref", "glossref"].some((t) => types.has(t)))
    return true;
  if (isBacklink(a)) return false;
  return (
    isSuper(a) ||
    (a.children.length === 1 && isSuper(a.children[0])) ||
    isSuper(a.parentElement)
  );
};

const INLINE = "a, span, sup, sub, em, strong, i, b, small, big";
const noteBlock = (target) => {
  const note = target.closest("li, aside");
  if (note) return note;
  let el = target;
  while (el.matches(INLINE) && el.parentElement) el = el.parentElement;
  if (el === el.ownerDocument.body) {
    const next = target.nextElementSibling;
    return next && !next.matches(INLINE) ? next : null;
  }
  return el;
};
const clean = (text) => (text ?? "").replace(/\s+/g, " ").trim();
const noteText = (target) => {
  if (!target) return [];
  const block = noteBlock(target);
  if (!block) return [];
  const note = block.cloneNode(true);
  note.querySelectorAll("a").forEach((a) => isBacklink(a) && a.remove());
  const paragraphs = [...note.querySelectorAll("p")];
  return (
    paragraphs.length
      ? paragraphs.map((p) => clean(p.textContent))
      : [clean(note.textContent)]
  ).filter(Boolean);
};

// The documents notes live in, loaded ahead so the first note opens
// quickly: each section the current page's note references point into.
const noteDocs = new Map();
const noteDoc = (index) => {
  if (!noteDocs.has(index)) {
    const promise =
      view.book.sections[index]?.createDocument?.() ?? Promise.resolve(null);
    noteDocs.set(
      index,
      promise.catch(() => null),
    );
    // A few books' worth of note files, not every section read.
    if (noteDocs.size > 6) noteDocs.delete(noteDocs.keys().next().value);
  }
  return noteDocs.get(index);
};
const resolveLink = (a, index) => {
  const raw = a.getAttribute("href");
  const href = view.book.sections[index]?.resolveHref?.(raw) ?? raw;
  return href;
};
const preloadNotes = () => {
  const doc = currentDoc();
  const index = currentIndex();
  if (!doc || index == null || (isScrolled() && !doc)) return;
  for (const a of doc.querySelectorAll("a[href]")) {
    if (!isNoteReference(a)) continue;
    const href = resolveLink(a, index);
    if (view.book.isExternal?.(href)) continue;
    Promise.resolve(view.book.resolveHref(href))
      .then((resolved) => {
        if (resolved && resolved.index !== index && resolved.index >= 0)
          noteDoc(resolved.index);
      })
      .catch(() => {});
  }
};

const rectOf = (a) => {
  const frame = a.ownerDocument.defaultView?.frameElement;
  const fr = frame?.getBoundingClientRect() ?? { left: 0, top: 0 };
  const r = a.getBoundingClientRect();
  return [fr.left + r.left, fr.top + r.top, r.width, r.height];
};

// Follows a link native hit-tested. A note reference shows its note in a
// native card; another link inside the book jumps there.
const activate = async (a) => {
  // A fixed-layout spread shows two sections; a link belongs to its own.
  const index = sectionOfDoc.get(a.ownerDocument) ?? currentIndex();
  const href = resolveLink(a, index);
  if (view.book.isExternal?.(href)) return post({ type: "external", href });
  if (isNoteReference(a)) {
    try {
      const resolved = await view.book.resolveHref(href);
      if (resolved && resolved.index >= 0) {
        const doc =
          resolved.index === index
            ? a.ownerDocument
            : await noteDoc(resolved.index);
        const paragraphs = doc ? noteText(resolved.anchor(doc)) : [];
        // The font around the reference, for the card, as the web
        // reader's takes it.
        // And its line height, as a multiple of the text's size; "normal"
        // gives none.
        const style = a.ownerDocument.defaultView?.getComputedStyle(
          a.closest("p, li, blockquote, dd, td, div") ?? a.parentElement ?? a,
        );
        const fontFamily = style?.fontFamily ?? "";
        const lineHeight =
          parseFloat(style?.lineHeight) / parseFloat(style?.fontSize);
        if (paragraphs.length)
          return post({
            type: "note",
            paragraphs,
            rect: rectOf(a),
            href,
            fontFamily,
            lineHeight: Number.isFinite(lineHeight) ? lineHeight : null,
          });
      }
    } catch (err) {
      post({ type: "log", message: "note failed: " + err });
    }
  }
  post({ type: "link", href });
};

const nearestLink = (x, y, radius) => {
  const doc = currentDoc();
  const fr = doc?.defaultView?.frameElement?.getBoundingClientRect();
  if (!fr) return null;
  let best = null;
  let bestDistance = radius;
  for (const a of doc.querySelectorAll("a[href]")) {
    for (const r of a.getClientRects()) {
      const left = fr.left + r.left,
        top = fr.top + r.top;
      if (r.width === 0 || top + r.height < 0 || top > innerHeight) continue;
      const d = Math.hypot(
        Math.max(left - x, 0, x - left - r.width),
        Math.max(top - y, 0, y - top - r.height),
      );
      if (d <= bestDistance) {
        bestDistance = d;
        best = a;
      }
    }
  }
  return best;
};

// MARK: - Images

// The images on the page big enough to open on their own, as photos and
// maps are, rather than an icon in a line of text: their boxes, as link
// boxes are reported, so native hit-tests a tap without a round trip.
const MIN_IMAGE = 60;
let imageElements = [];
const imagesIn = (doc) => [
  ...doc.querySelectorAll("img"),
  ...doc.getElementsByTagNameNS("http://www.w3.org/2000/svg", "image"),
];
const imageRects = () => {
  imageElements = [];
  if (isScrolled()) return [];
  const out = [];
  for (const { doc } of view.renderer?.getContents?.() ?? []) {
    const frame = doc?.defaultView?.frameElement;
    if (!frame) continue;
    const fr = frame.getBoundingClientRect();
    const scale = frame.offsetWidth > 0 ? fr.width / frame.offsetWidth : 1;
    for (const img of imagesIn(doc)) {
      const r = img.getBoundingClientRect();
      const x = fr.left + r.left * scale;
      const width = r.width * scale;
      const height = r.height * scale;
      if (width < MIN_IMAGE || height < MIN_IMAGE) continue;
      if (x + width < 0 || x > innerWidth) continue;
      const id = imageElements.push(img) - 1;
      out.push([
        Math.round(x),
        Math.round(fr.top + r.top * scale),
        Math.round(width),
        Math.round(height),
        id,
      ]);
    }
  }
  return out;
};

// The image under a point scrolled, where taps come through the page.
const imageAt = (x, y) => {
  const doc = currentDoc();
  const fr = doc?.defaultView?.frameElement?.getBoundingClientRect();
  if (!fr) return null;
  for (const img of imagesIn(doc)) {
    const r = img.getBoundingClientRect();
    if (r.width < MIN_IMAGE || r.height < MIN_IMAGE) continue;
    const left = fr.left + r.left;
    const top = fr.top + r.top;
    if (x >= left && x <= left + r.width && y >= top && y <= top + r.height)
      return img;
  }
  return null;
};

// An SVG drawn as a PNG, its longer side 2048 pixels, to zoom into.
const rasterize = async (blob) => {
  const url = URL.createObjectURL(blob);
  try {
    const im = new Image();
    im.src = url;
    await im.decode();
    const w = im.naturalWidth || 1024;
    const h = im.naturalHeight || 1024;
    const scale = 2048 / Math.max(w, h);
    const canvas = document.createElement("canvas");
    canvas.width = Math.round(w * scale);
    canvas.height = Math.round(h * scale);
    canvas.getContext("2d").drawImage(im, 0, 0, canvas.width, canvas.height);
    return await new Promise((resolve, reject) =>
      canvas.toBlob(
        (b) => (b ? resolve(b) : reject(new Error("no PNG"))),
        "image/png",
      ),
    );
  } finally {
    URL.revokeObjectURL(url);
  }
};

// Sends an image's bytes to native to show on its own, zoomable.
const openImage = async (img) => {
  try {
    const src =
      img.currentSrc ||
      img.src ||
      img.getAttribute("href") ||
      img.getAttributeNS("http://www.w3.org/1999/xlink", "href");
    if (!src) return;
    let blob = await (await fetch(src)).blob();
    // Native shows bitmaps, so an SVG is drawn to one first.
    if (blob.type === "image/svg+xml") blob = await rasterize(blob);
    const data = await new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result);
      reader.onerror = () => reject(reader.error);
      reader.readAsDataURL(blob);
    });
    post({ type: "image", message: data });
  } catch (err) {
    post({ type: "log", message: "image failed: " + err });
  }
};

// MARK: - Touches

// Scrolled mode stops at a section's end. Pulling on past the end, or back
// past the start, shows native's hint, and letting go past PULL brings in
// the section beside. Screen coordinates, since the frame moves as the
// container scrolls.
const PULL = 150;
let pull = null;
let pullShown = null;
const atScrollEnd = () => view.renderer.viewSize - view.renderer.end <= 2;
const atScrollStart = () => view.renderer.start <= 1;
// Whether there's a section that way to pull into.
const hasSection = (dir) => {
  const sections = view.book?.sections ?? [];
  for (let i = currentIndex() + dir; i >= 0 && i < sections.length; i += dir)
    if (sections[i].linear !== "no") return true;
  return false;
};
const showPull = (dir, amount) => {
  const shown =
    amount > 0 && hasSection(dir)
      ? { dir, amount, armed: amount >= PULL }
      : null;
  if (
    shown?.dir === pullShown?.dir &&
    shown?.armed === pullShown?.armed &&
    Math.abs((shown?.amount ?? 0) - (pullShown?.amount ?? 0)) < 2
  )
    return;
  pullShown = shown;
  // The section it brings in, by where it starts, so the hint names the
  // chapter showing at its top: two chapters in one section are passed by.
  const next = shown ? adjacentSection(currentIndex(), dir) : null;
  post({
    type: "pull",
    dir,
    amount: shown ? Math.round(amount) : 0,
    threshold: PULL,
    // By the rule letting go uses, which the rounded amount can miss.
    armed: !!shown?.armed,
    destination:
      next == null ? null : (view.getSectionFractions?.()[next] ?? null),
  });
};
const onPull = (e) => {
  if (!isScrolled()) return;
  const t = e.changedTouches?.[0];
  if (!t) return;
  if (e.type === "touchstart") {
    pull = { y: t.screenY, amount: 0, start: view.renderer.start };
    return;
  }
  if (!pull) return;
  if (e.type === "touchmove") {
    const dy = pull.y - t.screenY;
    pull.y = t.screenY;
    // At the top, the overscroll makes foliate's start (the absolute
    // scrollTop) grow again as the finger keeps going down, so that's the
    // top too, and a scroll that reaches it can go on into a pull, as one
    // that reaches the bottom can.
    const start = view.renderer.start;
    const atStart = atScrollStart() || (dy < 0 && start > pull.start);
    pull.start = start;
    pull.amount = stepPull(pull.amount, dy, atStart, atScrollEnd());
    if (pull.amount > 0) showPull(1, pull.amount);
    else if (pull.amount < 0) showPull(-1, -pull.amount);
    else if (pullShown) showPull(pullShown.dir, 0);
    return;
  }
  const p = pull;
  pull = null;
  if (pullShown) showPull(pullShown.dir, 0);
  if (e.type !== "touchend") return;
  if (Math.abs(p.amount) >= PULL)
    post({ type: "pullRelease", dir: Math.sign(p.amount) });
};

// Input inside the book, for timing reading; at most every few seconds.
let lastActivity = 0;
const noteActivity = () => {
  const now = Date.now();
  if (now - lastActivity < 3000) return;
  lastActivity = now;
  post({ type: "activity" });
};

// A selection dragged past the page's edge turns this page in place,
// keeping the selection: the next page is another WebView, which can't
// hold it.
let edgeTimer = null;
const checkSelectionEdge = (doc) => {
  clearTimeout(edgeTimer);
  edgeTimer = setTimeout(() => {
    const sel = doc.getSelection();
    if (
      !sel ||
      sel.isCollapsed ||
      !sel.rangeCount ||
      isScrolled() ||
      view.isFixedLayout
    )
      return;
    const fr = doc.defaultView?.frameElement?.getBoundingClientRect();
    if (!fr) return;
    const focus = doc.createRange();
    focus.setStart(sel.focusNode, sel.focusOffset);
    const rect = focus.getClientRects()[0] ?? focus.getBoundingClientRect();
    const x = fr.left + rect.left;
    const rtl = view.book?.dir === "rtl";
    if (x > innerWidth - 4) post({ type: "selectionEdge", dir: rtl ? -1 : 1 });
    else if (x < 4) post({ type: "selectionEdge", dir: rtl ? 1 : -1 });
  }, 600);
};

// Scrolled, whether the text was still moving when a finger touched down:
// a tap that only stops a scroll isn't a tap on the page, so it doesn't
// show the controls. From the paginator's scroll events, which go on
// through momentum, so an idle page does no work.
let scrollMovedAt = 0;
let touchStoppedScroll = false;
const noteScroll = () => {
  scrollMovedAt = performance.now();
};

let selectionActive = false;
const guardWindow = (win, isTop) => {
  const capture = { capture: true, passive: true };
  for (const type of ["touchstart", "touchmove", "touchend", "touchcancel"]) {
    win.addEventListener(
      type,
      (e) => {
        onPull(e);
        if (type === "touchstart") {
          noteActivity();
          touchStoppedScroll =
            isScrolled() && performance.now() - scrollMovedAt < 100;
        }
        // foliate's paginator drags the page from touchmove and snaps
        // on touchend; native owns that.
        e.stopPropagation();
      },
      capture,
    );
  }
  if (isTop) return;
  // A link is followed only by native's hit test, never twice.
  win.addEventListener(
    "click",
    (e) => {
      e.preventDefault();
      e.stopImmediatePropagation();
    },
    true,
  );
  // foliate scrolls to a focused element and turns when a pointer
  // selection reaches the page's edge; either would move the page under
  // native.
  win.addEventListener("focusin", (e) => e.stopImmediatePropagation(), true);
  win.addEventListener(
    "selectionchange",
    (e) => {
      e.stopImmediatePropagation();
      const sel = win.document.getSelection();
      const active = !!sel && !sel.isCollapsed && sel.toString().length > 0;
      if (active !== selectionActive) {
        selectionActive = active;
        post({ type: "selection", active });
      }
      if (active) checkSelectionEdge(win.document);
    },
    true,
  );
};
guardWindow(window, true);
// Each section's document by the section it shows.
const sectionOfDoc = new WeakMap();
view.addEventListener("load", ({ detail: { doc, index } }) => {
  if (index != null) sectionOfDoc.set(doc, index);
  guardWindow(doc.defaultView, false);
  // Before the justification setting applies and before layout, so it
  // reads the book's own alignment.
  recordBookAlignment(doc);
});

// MARK: - Neighbors

// Each call waits for the one before, so a sync never overlaps the open or
// another sync.
let chain = Promise.resolve();
// A sync or jump that fails still answers its token, as failed, so native
// stops waiting on it.
const chained = (label, f, token) => {
  chain = chain.then(async () => {
    quiet = true;
    try {
      await f();
    } catch (err) {
      post({
        type: "log",
        message: label + " failed: " + String(err?.stack ?? err),
      });
      if (token != null) post({ type: "synced", token, failed: true });
      else
        post({
          type: "error",
          message: label + ": " + String(err?.stack ?? err),
        });
    } finally {
      quiet = false;
    }
  });
};

// From the page at base (section index and page, as the paginator counts
// them, with a blank page at each end), the target one page in dir, or null
// at the book's edge.
const stepTarget = ({ index, page, pages }, dir) => {
  const sections = view.book.sections;
  const adjacent = (d) => {
    for (let i = index + d; i >= 0 && i < sections.length; i += d)
      if (sections[i].linear !== "no") return i;
    return null;
  };
  const textPages = Math.max(1, pages - 2);
  const at = (p) => (textPages > 1 ? (p - 1) / (textPages - 1) : 0);
  if (dir > 0)
    return page < textPages
      ? { index, anchor: at(page + 1) }
      : adjacent(1) != null
        ? { index: adjacent(1), anchor: 0 }
        : null;
  return page > 1
    ? { index, anchor: at(page - 1) }
    : adjacent(-1) != null
      ? { index: adjacent(-1), anchor: 1 }
      : null;
};

const ownBase = () => {
  const { page, pages } = geometry();
  const index = currentIndex();
  return index != null && page != null ? { index, page, pages } : null;
};

// Moves this neighbor one page from base and reports it with the base, so
// native can check it against where the current page really is.
// A fixed-layout neighbor goes to the current page's section and turns
// once, which is exact: its pages are whole sections, or halves of a spread.
const stepFixed = async (base, dir) => {
  await view.renderer.goTo({ index: base.index });
  await (dir > 0 ? view.renderer.next() : view.renderer.prev());
  await settle();
  return currentIndex() !== base.index;
};

// The section a step that way from index goes to, past any outside the
// reading order, or null past the first or last.
const adjacentSection = (index, dir) => {
  if (index == null) return null;
  const sections = view.book?.sections ?? [];
  for (let i = index + dir; i >= 0 && i < sections.length; i += dir)
    if (sections[i].linear !== "no") return i;
  return null;
};

// Scrolled, a neighbor is the next or previous section from its top, so a
// sideways drag brings in the chapter beside, as a turn brings in a page.
// Its base is the section alone: scrolling within it moves nothing.
const stepSection = async (index, dir, token) => {
  const next = adjacentSection(index, dir);
  if (next != null) {
    await view.renderer.goTo({ index: next });
    await settle();
  }
  post({
    type: "synced",
    token,
    base: [index, 0, 0],
    edge: next == null,
    loc: location(),
  });
};

const step = async (base, dir, token) => {
  if (base && isScrolled() && !view.isFixedLayout)
    return stepSection(base.index, dir, token);
  const b = base ? [base.index, base.page, base.pages] : null;
  let target = null;
  if (base && view.isFixedLayout) {
    target = (await stepFixed(base, dir)) || null;
  } else {
    target = base ? stepTarget(base, dir) : null;
    if (target) {
      await view.renderer.goTo(target);
      await settle();
    }
  }
  post({
    type: "synced",
    token,
    base: b,
    edge: !!base && !target,
    loc: location(),
  });
};

// MARK: - Contents

// Where a chapter's link goes in this book. Shisho keeps an EPUB's chapter
// links as its navigation document wrote them, relative to that document,
// as foliate's own table of contents resolves them.
const ROOT = "https://invalid.invalid/";
const fromNavigation = (href, base) => {
  try {
    const url = new URL(href, ROOT + base);
    url.search = "";
    return decodeURI(url.href.replace(ROOT, ""));
  } catch {
    return href;
  }
};

// The share of a section's text before each target, in one pass.
const sharesBefore = (doc, targets) => {
  const shares = new Map();
  const body = doc.body;
  const total = body?.textContent?.length ?? 0;
  if (!body || total === 0) return shares;
  const pending = [...targets].sort((a, b) =>
    a === b ? 0 : a.compareDocumentPosition(b) & 4 ? -1 : 1,
  );
  const walker = doc.createTreeWalker(body, NodeFilter.SHOW_TEXT);
  let before = 0;
  for (let text = walker.nextNode(); text; text = walker.nextNode()) {
    while (
      pending.length &&
      (pending[0].contains(text) ||
        pending[0].compareDocumentPosition(text) & 4)
    )
      shares.set(pending.shift(), before / total);
    before += text.textContent?.length ?? 0;
  }
  for (const target of pending) shares.set(target, before / total);
  return shares;
};

// Where each chapter goes and starts along the book, as the web reader
// places its chapter marks.
// The navigation document Shisho's chapter links are written relative to:
// the scan reads the nav, or the NCX when it can't, so the base is the one
// that resolves more of a few of them.
const navigationBase = async (chapters) => {
  const bases = [
    ...new Set(
      [view.book.resources?.navPath, view.book.resources?.ncxPath].filter(
        Boolean,
      ),
    ),
  ];
  if (bases.length < 2) return bases[0] ?? "";
  const sample = chapters.filter((c) => c.href).slice(0, 12);
  let best = bases[0];
  let bestCount = -1;
  for (const base of bases) {
    let count = 0;
    for (const { href } of sample) {
      try {
        const resolved = await view.book.resolveHref(
          fromNavigation(href, base),
        );
        if (resolved && resolved.index >= 0) count += 1;
      } catch {
        // Not in this book.
      }
    }
    if (count > bestCount) [best, bestCount] = [base, count];
  }
  return best;
};

const contents = async (chapters, relative) => {
  const fractions = view.getSectionFractions();
  const base = relative ? await navigationBase(chapters) : "";
  const entries = [];
  const inSections = new Map();
  for (const [i, { href }] of chapters.entries()) {
    entries.push({ target: null, position: null });
    if (!href) continue;
    const target = relative ? fromNavigation(href, base) : href;
    const resolved = await view.book.resolveHref(target);
    if (!resolved || resolved.index < 0) continue;
    entries[i].target = target;
    const start = fractions[resolved.index];
    if (start === undefined) continue;
    // An EPUB link without a #fragment starts its section; a MOBI's
    // filepos or kindle:pos link can be anywhere in one.
    if (relative && !target.includes("#")) {
      entries[i].position = start;
      continue;
    }
    const waiting = inSections.get(resolved.index) ?? [];
    waiting.push({ i, resolved });
    inSections.set(resolved.index, waiting);
  }
  for (const [index, waiting] of inSections) {
    const start = fractions[index];
    const end = fractions[index + 1] ?? 1;
    const doc = await view.book.sections[index]?.createDocument?.();
    const targets = waiting.map(({ resolved }) => {
      const t = doc ? resolved.anchor(doc) : null;
      return t?.nodeType === 1 ? t : null;
    });
    const shares = doc ? sharesBefore(doc, targets.filter(Boolean)) : new Map();
    waiting.forEach(({ i }, k) => {
      const share = targets[k] ? (shares.get(targets[k]) ?? 0) : 0;
      entries[i].position = start + share * (end - start);
    });
  }
  return entries;
};

const flattenToc = (items, depth = 0, out = []) => {
  for (const item of items ?? []) {
    out.push({
      label: (item.label ?? "").trim(),
      href: item.href ?? "",
      depth,
    });
    flattenToc(item.subitems, depth + 1, out);
  }
  return out;
};

// Goes to target, or, when this copy of the book can't find it (a CFI from
// another edition), to fallback of the way through.
const goToOrNear = async (target, fallback) => {
  // foliate goes nowhere, without an error, for a section it doesn't have.
  // A CFI from a longer edition can name a section past this copy's last.
  const resolved = await view.goTo(target);
  const found =
    resolved?.index >= 0 && resolved.index < (view.book?.sections?.length ?? 0);
  if (!found && fallback != null) {
    await view.goTo({ fraction: fallback });
  }
};

// The arguments the page was last opened with, to open the book again when
// that open failed: a neighbor asked to step from a book it never opened
// opens it first.
let lastOpen = null;
const openIfNeeded = async () => {
  if (!view.book && lastOpen) await open({ ...lastOpen, idle: true });
};

// MARK: - Search

// A phrase anywhere in the text, ignoring case and accents, as the web
// reader searches (SEARCH_OPTIONS in app/hooks/useBookSearch.ts).
const SEARCH_OPTIONS = {
  matchCase: false,
  matchDiacritics: false,
  matchWholeWords: false,
};

// Bumped by each search and each cancel, which leaves the last one's
// results unsent and stops it at its next section.
let searchRun = 0;

// Searches the whole book, sending each section's matches as they're found
// and then that the search is done, each under native's token. Only the
// page that was current runs it; the others mark the chosen result.
const search = async (query, token) => {
  const run = ++searchRun;
  const stale = () => run !== searchRun;
  try {
    // A page opening the book again, as after its process ended, searches
    // once it has.
    await chain;
    if (stale()) return;
    const found = view.search({
      ...SEARCH_OPTIONS,
      query,
      // foliate's own marks only reach this page; native sends the
      // matches to every page, which shades them (addMatches).
      draw: () => document.createElementNS(SVG_NS, "g"),
    });
    // Leaving the loop stops foliate's search, which reads no further.
    for await (const item of found) {
      if (stale()) return;
      if (item === "done") break;
      if (!("subitems" in item)) continue;
      post({
        type: "searchResults",
        token,
        results: item.subitems.map(({ cfi, excerpt, label }) => ({
          cfi,
          pre: excerpt.pre,
          match: excerpt.match,
          post: excerpt.post,
          chapter: label ?? item.label ?? "",
        })),
      });
    }
  } catch (err) {
    post({ type: "log", message: "search failed: " + err });
  }
  if (!stale()) post({ type: "searchDone", token });
};

const SVG_NS = "http://www.w3.org/2000/svg";

// The marks the web reader draws (app/libraries/searchMarks.ts): every
// match lightly shaded, and the one the reader went to in a deeper amber
// that reaches a little past its text, laid over the shading.
const drawBoxes = (g, boxes, { color, opacity, pad }) => {
  const layer = document.createElementNS(SVG_NS, "g");
  layer.setAttribute("fill", color);
  layer.style.opacity = String(opacity);
  for (const { left, top, width, height } of boxes) {
    if (!width || !height) continue;
    const rect = document.createElementNS(SVG_NS, "rect");
    rect.setAttribute("x", String(left - pad));
    rect.setAttribute("y", String(top - pad));
    rect.setAttribute("width", String(width + pad * 2));
    rect.setAttribute("height", String(height + pad * 2));
    rect.setAttribute("rx", "3");
    layer.append(rect);
  }
  g.append(layer);
};
const MATCH = { color: "#facc15", opacity: 0.3, pad: 0 };
const CHOSEN = { color: "#f59e0b", opacity: 0.45, pad: 2 };
const drawMatch = (boxes) => {
  const g = document.createElementNS(SVG_NS, "g");
  drawBoxes(g, boxes, MATCH);
  return g;
};
// One annotation per match, so the chosen one draws both marks.
const drawChosen = (boxes) => {
  const g = document.createElementNS(SVG_NS, "g");
  const list = Array.from(boxes);
  drawBoxes(g, list, MATCH);
  drawBoxes(g, list, CHOSEN);
  return g;
};

// The search's matches by section, which every page shades in the sections
// it shows, whichever page found them; and the one the reader went to.
let matches = new Map();
let matchSet = new Set();
let chosenResult = null;
const mark = (cfi) =>
  Promise.resolve(view.addAnnotation({ value: cfi })).catch(() => {});
const unmark = (cfi) =>
  Promise.resolve(view.deleteAnnotation({ value: cfi })).catch(() => {});
// The sections showing now; the rest are marked as they load.
const shownSections = () =>
  new Set((view.renderer?.getContents?.() ?? []).map(({ index }) => index));

// Matches sent before the book opened, as to a page opening again after its
// process ended, marked once it has.
let pendingMatches = [];

const addMatches = (cfis) => {
  if (!view.book) {
    pendingMatches.push(...cfis);
    return;
  }
  const shown = shownSections();
  for (const cfi of cfis) {
    if (matchSet.has(cfi)) continue;
    let index;
    try {
      ({ index } = view.resolveCFI(cfi));
    } catch {
      continue;
    }
    matchSet.add(cfi);
    if (!matches.has(index)) matches.set(index, []);
    matches.get(index).push(cfi);
    if (shown.has(index)) mark(cfi);
  }
};

const clearMatches = () => {
  for (const cfi of matchSet) unmark(cfi);
  if (chosenResult && !matchSet.has(chosenResult)) unmark(chosenResult);
  matches = new Map();
  matchSet = new Set();
  pendingMatches = [];
  chosenResult = null;
};

// foliate draws an annotation when it's added and again whenever its
// section loads, which it announces with "create-overlay".
view.addEventListener("draw-annotation", ({ detail: { draw, annotation } }) => {
  if (annotation.value === chosenResult) draw(drawChosen);
  else if (matchSet.has(annotation.value)) draw(drawMatch);
});
view.addEventListener("create-overlay", ({ detail: { index } }) => {
  for (const cfi of matches.get(index) ?? []) mark(cfi);
  if (chosenResult && !matchSet.has(chosenResult)) mark(chosenResult);
});

const markResult = (cfi) => {
  const old = chosenResult;
  chosenResult = cfi;
  if (!view.book) return;
  // Drawn again as a plain match, or taken off when it's no longer one.
  if (old && old !== cfi) (matchSet.has(old) ? mark : unmark)(old);
  if (cfi) mark(cfi);
};

// MARK: - Native calls

window.R = {
  // Opens the book. The current page (neighborDir 0) shows start; a
  // neighbor opens there at the same time, then steps one page itself. An
  // idle neighbor, in scrolled mode, only opens the book.
  open(args) {
    lastOpen = args;
    // A neighbor's open answers its token, as a sync does, even when it
    // fails.
    chained("open", () => open(args), args.neighborDir ? args.token : null);
  },
  // keepPlace, for the page showing, pins the place to the text first, so
  // the reflow keeps it: a page moved there as a neighbor holds its place
  // as a share of the section's pages, which a new size changes.
  apply(s, keepPlace) {
    chained("apply", async () => {
      // After any jump in flight, whose move would otherwise pass for this
      // relayout.
      await navigation;
      // Only a page that shows something: an empty page's CFI is its
      // section's body, which would go back to the section's start.
      const shown = view.lastLocation?.range;
      const here =
        keepPlace &&
        !view.isFixedLayout &&
        landed() &&
        shown &&
        !shown.collapsed &&
        shown.toString().trim()
          ? view.lastLocation.cfi
          : null;
      const place =
        here && pinnedPlace?.moves === moves ? pinnedPlace.cfi : here;
      pinnedPlace = place ? { cfi: place, moves } : null;
      if (place) await goToOrNear(place, null).catch(() => {});
      applyStyles(s, keepPlace);
      await view.renderer?.settled?.();
      await settle();
      post({ type: "applied", loc: location() });
    });
  },
  // Moves this neighbor to the page next to the current page's place,
  // given as section index and page. It jumps there with one goTo: a CFI
  // round trip isn't exact, and foliate's next() and prev() hold a lock and
  // relocate twice at a section's edge.
  sync(index, page, pages, dir, token) {
    moves++;
    chained(
      "sync",
      async () => {
        await openIfNeeded();
        await step({ index, page, pages }, dir, token);
      },
      token,
    );
  },
  // A jump a neighbor makes at the same time as the current page: go
  // there, then step one page.
  jump(target, dir, token, fallback) {
    moves++;
    chained(
      "jump",
      async () => {
        await openIfNeeded();
        await goToOrNear(target, fallback);
        await settle();
        await step(dir ? ownBase() : null, dir, token);
      },
      token,
    );
  },
  // Jumps of the current page run one at a time, each after any turn in
  // progress, which would otherwise drop it.
  // A jump on the current page. Jumps land in turn, each answering its
  // token once it has, or has failed.
  goTo(target, fallback, token) {
    // A jump moves the reader, even one landing while a settings change
    // settles, when its move passes for the relayout's.
    pinnedPlace = null;
    navigation = navigation.then(async () => {
      try {
        await view.renderer?.settled?.();
        await goToOrNear(target, fallback);
      } catch (err) {
        post({ type: "log", message: "jump failed: " + err });
      }
      post({ type: "jumped", token });
    });
  },
  next() {
    pinnedPlace = null;
    view.renderer?.next();
  },
  prev() {
    pinnedPlace = null;
    view.renderer?.prev();
  },
  activateLink(id) {
    const a = linkElements[id];
    if (a?.isConnected) activate(a);
  },
  // Scrolled mode: the page moves under native, so the hit test is here.
  tapLink(x, y) {
    if (touchStoppedScroll) {
      touchStoppedScroll = false;
      return true;
    }
    const a = nearestLink(x, y, 24);
    if (!a) {
      // Scrolled, an image opens on its own from the middle of the page.
      const img =
        x > innerWidth / 3 && x < (innerWidth * 2) / 3 && imageAt(x, y);
      if (!img) return false;
      openImage(img);
      return true;
    }
    activate(a);
    return true;
  },
  setChapterTargets(hrefs) {
    chapterHrefs = hrefs;
    chapterTargets = null;
    chapterPages = null;
  },
  // Moves into the section beside in place, for a pull let go before the
  // neighbor that slides in was ready: at its top, where the neighbor would
  // have shown it. Not foliate's prev, which lands at the previous section's
  // end, and during the bounce at the top only scrolls back to it.
  pullThrough(dir) {
    pinnedPlace = null;
    const next = adjacentSection(currentIndex(), dir);
    if (next == null) return;
    Promise.resolve(view.renderer?.goTo({ index: next })).catch(() => {});
  },
  // A sideways drag took the touch: a pull under way stops, and letting go
  // of it brings nothing in.
  cancelPull() {
    pull = null;
    if (pullShown) showPull(pullShown.dir, 0);
  },
  openImage(id) {
    const img = imageElements[id];
    if (img) openImage(img);
  },
  clearSelection() {
    currentDoc()?.getSelection()?.removeAllRanges();
  },
  preloadNotes() {
    preloadNotes();
  },
  // The section the reader opened in, the same for every page, whichever
  // is showing.
  setOpenedSection(index) {
    openedSection = index;
  },
  // Searches the book: the answers come by token, from whichever page this
  // is by then.
  search(query, token) {
    search(query, token);
  },
  // Stops a search under way, on the page running it, and takes its marks
  // off every page.
  cancelSearch() {
    searchRun++;
    view.clearSearch?.();
    clearMatches();
  },
  // Matches the search found, shaded on whichever page shows them.
  addMatches(cfis) {
    addMatches(cfis);
  },
  // Marks the chosen search result, or none.
  markResult(cfi) {
    markResult(cfi);
  },
  // Native accepts the answer from whichever page gives it, by token,
  // since a turn can make this page a neighbor before it answers.
  contents(chapters, relative, token) {
    contents(chapters, relative)
      .then((entries) => post({ type: "contents", contents: entries, token }))
      .catch((err) => post({ type: "log", message: "contents: " + err }));
  },
};

let navigation = Promise.resolve();

// Whether a page is showing: foliate goes nowhere, without an error, for a
// CFI whose section this copy of the book doesn't have.
const landed = () => currentDoc() != null && view.lastLocation != null;

// MARK: - The book, a part at a time

// The book on the device, read a part at a time through the app, as the web
// reader reads one from the server (app/libraries/rangeFile.ts), so no page
// holds a whole book: a photo cookbook runs to hundreds of megabytes, and
// three pages each read it.
// Reads smaller than this come from a block of BLOCK bytes kept for the next
// one: foliate's MOBI reader reads four bytes of every resource record at
// open, hundreds of reads that would each be a round trip.
const SMALL_READ = 16 * 1024;
const BLOCK = 256 * 1024;
const BLOCKS_KEPT = 8;

const localFile = (url, size) => {
  const fetchRange = async (offset, length) => {
    const response = await fetch(url, {
      headers: { Range: `bytes=${offset}-${offset + length - 1}` },
    });
    if (response.status !== 206) {
      throw new Error(`reading the book failed: ${response.status}`);
    }
    return new Uint8Array(await response.arrayBuffer());
  };
  const blocks = new Map();
  const block = (start) => {
    let data = blocks.get(start);
    if (!data) {
      data = fetchRange(start, Math.min(BLOCK, size - start));
      data.catch(() => blocks.delete(start));
      blocks.set(start, data);
      if (blocks.size > BLOCKS_KEPT) blocks.delete(blocks.keys().next().value);
    }
    return data;
  };
  return {
    size,
    async read(offset, length) {
      if (length <= 0) return new Uint8Array(0);
      const start = Math.floor(offset / BLOCK) * BLOCK;
      if (length <= SMALL_READ && offset + length <= start + BLOCK) {
        const data = await block(start);
        return data.slice(offset - start, offset - start + length);
      }
      return fetchRange(offset, length);
    },
  };
};

// What foliate's MOBI reader asks of a file: slice(start, end) and its
// bytes (app/libraries/rangeMobi.ts).
const rangeBlob = (file, start, end) => {
  const clamp = (at) => Math.min(Math.max(at, 0), end - start);
  return {
    size: end - start,
    slice: (from = 0, to = end - start) =>
      rangeBlob(file, start + clamp(from), start + clamp(to)),
    arrayBuffer: async () => {
      const data = await file.read(start, end - start);
      return data.buffer.slice(
        data.byteOffset,
        data.byteOffset + data.byteLength,
      );
    },
  };
};

// An EPUB through zip.js, reading each entry as foliate asks for it, as
// openRangeEPUB in app/libraries/rangeEpub.ts does. Reads are local, so it
// needs none of the web's fetching ahead.
const openEPUB = async (file) => {
  const [
    { EPUB },
    { BlobReader, BlobWriter, configure, TextWriter, ZipReader },
  ] = await Promise.all([
    import("../foliate/epub.js"),
    import("../foliate/vendor/zip.js"),
  ]);
  class RangeReader extends BlobReader {
    constructor() {
      super(new Blob());
      this.size = file.size;
    }
    readUint8Array(offset, length) {
      return file.read(offset, length);
    }
  }
  // An entry's data comes through in reads of chunkSize; zip.js's own 64 KB
  // would be dozens of round trips for one photo.
  configure({ useWebWorkers: false, chunkSize: 1024 * 1024 });
  const reader = new ZipReader(new RangeReader());
  const entries = await reader.getEntries();
  const map = new Map(entries.map((entry) => [entry.filename, entry]));
  // The package as Shisho's generated copy would have it, near enough:
  // see package.js.
  const loadText = async (name) => {
    const text = (await map.get(name)?.getData(new TextWriter())) ?? null;
    return text && name.toLowerCase().endsWith(".opf")
      ? cleanPackage(text)
      : text;
  };
  const loadBlob = (name, type) =>
    map.get(name)?.getData(new BlobWriter(type)) ?? null;
  const getSize = (name) => map.get(name)?.uncompressedSize ?? 0;
  return new EPUB({ loadText, loadBlob, getSize }).init();
};

// A MOBI or AZW3 through foliate's MOBI reader, a record at a time
// (openRangeMOBI in app/libraries/rangeMobi.ts).
const openMOBI = async (file) => {
  const [{ MOBI }, { unzlibSync }] = await Promise.all([
    import("../foliate/mobi.js"),
    import("../foliate/vendor/fflate.js"),
  ]);
  return new MOBI({ unzlib: unzlibSync }).open(rangeBlob(file, 0, file.size));
};

const open = async ({
  url,
  size,
  format,
  start,
  fallback,
  settings: s,
  neighborDir,
  token,
  idle,
  language,
}) => {
  const file = localFile(url, size);
  const book = format === "mobi" ? await openMOBI(file) : await openEPUB(file);
  // Shisho's normalized language, for the page's lang and direction: a
  // book's own can be a form foliate can't use, as en_US or English.
  if (language && book.metadata) book.metadata.language = language;
  await view.open(book);
  if (pendingMatches.length) {
    const cfis = pendingMatches;
    pendingMatches = [];
    addMatches(cfis);
  }
  view.renderer?.addEventListener?.("scroll", noteScroll, { passive: true });
  applyStyles(s);
  post({
    type: "opened",
    toc: flattenToc(view.book?.toc),
    rtl: view.book?.dir === "rtl",
    // Paged whatever the layout setting, as foliate lays it out.
    fixedLayout: !!view.isFixedLayout,
  });
  if (idle) return;
  let restored = !!start;
  // foliate can show the section and then fail to find the place in it, as
  // when the book's text changed: that's a failed restore too.
  let failed = false;
  try {
    await view.init(start ? { lastLocation: start } : { showTextStart: false });
  } catch (err) {
    failed = true;
    post({ type: "log", message: "restore failed: " + err });
  }
  // A saved place this copy of the book can't find opens at the same
  // percentage, and failing that at the start.
  if ((failed || !landed()) && start && fallback != null) {
    restored = false;
    failed = false;
    try {
      await view.goTo({ fraction: fallback });
    } catch {
      failed = true;
    }
  }
  if (failed || !landed()) {
    restored = false;
    await view.init({ showTextStart: false });
  }
  openedSection ??= currentIndex();
  await settle();
  if (neighborDir) return step(ownBase(), neighborDir, token);
  post({ type: "ready", loc: location(), restored });
  preloadNotes();
};

// WebKit on iOS asks QuickLook whether it previews a frame's document when
// that document isn't HTML or plain text. The first ask loads QuickLook into
// this page's process, which blocks it for a second or more (several on a
// cold start). Every section of a book is XHTML, so the page asks once here,
// as it starts, instead of in the middle of laying out the first page.
// WebCore's SubresourceLoader::shouldCreatePreviewLoaderForResponse is the
// check.
const warmQuickLook = () => {
  const frame = document.createElement("iframe");
  frame.setAttribute("sandbox", "allow-same-origin");
  frame.setAttribute("aria-hidden", "true");
  frame.style.cssText =
    "position:absolute;width:0;height:0;border:0;visibility:hidden";
  const url = URL.createObjectURL(
    new Blob(
      [
        '<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title></title></head><body/></html>',
      ],
      { type: "application/xhtml+xml" },
    ),
  );
  frame.addEventListener("load", () => {
    // The first load is the frame's initial blank document.
    if (frame.contentDocument?.URL !== url) return;
    frame.remove();
    URL.revokeObjectURL(url);
  });
  frame.src = url;
  document.body.append(frame);
};
warmQuickLook();

post({ type: "boot" });
