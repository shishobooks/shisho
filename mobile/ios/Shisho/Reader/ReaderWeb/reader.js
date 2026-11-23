// The Shisho app's bridge to the web reader's vendored foliate-js (ADR 0026).
// The app runs three of these pages, the current page and one on either
// side. Native code owns every page turn and gesture; this lays the book
// out, keeps foliate's own touch handling out of the way, and answers
// questions about links, notes, selection, and where things are. Native
// calls window.R; this answers through the "bridge" message handler, one
// JSON string per message.
//
// Some rules follow the web reader's, which is TypeScript and can't be
// imported here: the theme stylesheet (app/libraries/readerSettings.ts),
// footnotes (app/libraries/readerFootnotes.ts), chapter marks
// (app/libraries/readerChapterMarks.ts), and the end of the book
// (app/utils/readerEnd.ts). A change there changes these too.
import "../foliate/view.js";

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

// The text the line spacing applies to.
const TEXT = "p, li, blockquote, dd";

// The web reader's stylesheet at its default size and spacing; the app's
// display settings beyond theme and layout come later (#920).
const css = (s) => {
  const replaceBookColors = s.link
    ? `
      body { color: inherit !important; background: none !important; }
      body * { color: inherit !important; background-color: transparent !important; }
      a:any-link, a:any-link * { color: ${s.link} !important; }
      ${s.theme === "dark" || s.theme === "black" ? "img, svg { background-color: #fff !important; }" : ""}`
    : "";
  return `
      @namespace epub "http://www.idpf.org/2007/ops";
      * { -webkit-tap-highlight-color: transparent; }
      html { color: ${s.fg} !important; background: ${s.bg} !important; font-size: 16px !important; -webkit-text-size-adjust: 100% !important; }
      ${replaceBookColors}
      ${TEXT} { line-height: 1.45 !important; }`;
};

const applyStyles = (s) => {
  document.documentElement.style.background = s.bg;
  const renderer = view.renderer;
  if (!renderer) return;
  renderer.setStyles?.(css(s));
  // Setting an attribute lays the book out again even when it's unchanged.
  const set = (name, value) => {
    if (renderer.getAttribute(name) !== value)
      renderer.setAttribute(name, value);
  };
  set("gap", "7%");
  set("flow", s.layout);
  set("max-column-count", "2");
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
// hit-test a tap without a round trip: [x, y, width, height, id] per box.
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
      for (const r of a.getClientRects()) {
        const x = fr.left + r.left * scale;
        const width = r.width * scale;
        if (width === 0 || x + width < 0 || x > innerWidth) continue;
        if (id < 0) id = linkElements.push(a) - 1;
        out.push([
          Math.round(x),
          Math.round(fr.top + r.top * scale),
          Math.round(width),
          Math.round(r.height * scale),
          id,
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

const location = () => {
  const d = view.lastLocation;
  const { page, pages } = geometry();
  const index = d?.section?.current ?? currentIndex();
  return {
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
// native popover; another link inside the book jumps there.
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
        if (paragraphs.length)
          return post({ type: "note", paragraphs, rect: rectOf(a), href });
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

// MARK: - Touches

// Scrolled mode stops at a section's end: pulling on past the end, or a
// quick flick that starts there, continues into the next section, and the
// same at the start. Screen coordinates, since the frame moves as the
// container scrolls.
let pull = null;
const atScrollEnd = () => view.renderer.viewSize - view.renderer.end <= 2;
const atScrollStart = () => view.renderer.start <= 1;
const onPull = (e) => {
  if (!isScrolled()) return;
  const t = e.changedTouches?.[0];
  if (!t) return;
  if (e.type === "touchstart") {
    pull = {
      y0: t.screenY,
      y: t.screenY,
      t0: e.timeStamp,
      down: 0,
      up: 0,
      startEnd: atScrollEnd(),
      startStart: atScrollStart(),
    };
    return;
  }
  if (!pull) return;
  if (e.type === "touchmove") {
    const dy = pull.y - t.screenY;
    pull.y = t.screenY;
    pull.down =
      dy > 0 && atScrollEnd()
        ? pull.down + dy
        : Math.max(0, pull.down + Math.min(dy, 0));
    pull.up =
      dy < 0 && atScrollStart()
        ? pull.up - dy
        : Math.max(0, pull.up - Math.max(dy, 0));
    return;
  }
  const p = pull;
  pull = null;
  if (e.type !== "touchend") return;
  const quick = e.timeStamp - p.t0 < 300;
  // Past the first or last section there's nowhere to go, which foliate
  // rejects.
  const go = (turn) => Promise.resolve(turn).catch(() => {});
  if (p.down > 60 || (p.startEnd && quick && p.y0 - p.y > 30))
    go(view.renderer.next());
  else if (p.up > 60 || (p.startStart && quick && p.y - p.y0 > 30))
    go(view.renderer.prev());
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

let selectionActive = false;
const guardWindow = (win, isTop) => {
  const capture = { capture: true, passive: true };
  for (const type of ["touchstart", "touchmove", "touchend", "touchcancel"]) {
    win.addEventListener(
      type,
      (e) => {
        onPull(e);
        if (type === "touchstart") noteActivity();
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

const step = async (base, dir, token) => {
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
const fromNavigation = (href) => {
  const base =
    view.book.resources?.navPath ?? view.book.resources?.ncxPath ?? "";
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
const contents = async (chapters, relative) => {
  const fractions = view.getSectionFractions();
  const entries = [];
  const inSections = new Map();
  for (const [i, { href }] of chapters.entries()) {
    entries.push({ target: null, position: null });
    if (!href) continue;
    const target = relative ? fromNavigation(href) : href;
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
  apply(s) {
    chained("apply", async () => {
      applyStyles(s);
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
  goTo(target, fallback) {
    navigation = navigation.then(async () => {
      try {
        await view.renderer?.settled?.();
        await goToOrNear(target, fallback);
      } catch (err) {
        post({ type: "log", message: "jump failed: " + err });
      }
    });
  },
  next() {
    view.renderer?.next();
  },
  prev() {
    view.renderer?.prev();
  },
  activateLink(id) {
    const a = linkElements[id];
    if (a?.isConnected) activate(a);
  },
  // Scrolled mode: the page moves under native, so the hit test is here.
  tapLink(x, y) {
    const a = nearestLink(x, y, 24);
    if (!a) return false;
    activate(a);
    return true;
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
  const loadText = (name) => map.get(name)?.getData(new TextWriter()) ?? null;
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
}) => {
  const file = localFile(url, size);
  await view.open(
    format === "mobi" ? await openMOBI(file) : await openEPUB(file),
  );
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

post({ type: "boot" });
