// Footnotes in a reflowable book, found the way foliate-js's footnotes.js
// finds them: a link the book marks as a note reference, or, for a book that
// marks nothing, a link set as a superscript.
//
// Elements here come from the book's frame, another realm, so nothing uses
// `instanceof`.
//
// The Shisho app's reader repeats these rules in
// mobile/ios/Shisho/Reader/ReaderWeb/reader.js, which can't import
// TypeScript: change both.

const EPUB_NS = "http://www.idpf.org/2007/ops";

const tokens = (value: string | null | undefined) =>
  new Set(value?.split(/\s+/).filter(Boolean));
const epubTypes = (el: Element | null) =>
  tokens(el?.getAttributeNS?.(EPUB_NS, "type"));
const roles = (el: Element | null) => tokens(el?.getAttribute?.("role"));

const REF_TYPES = ["noteref", "biblioref", "glossref"];
const REF_ROLES = ["doc-noteref", "doc-biblioref", "doc-glossref"];

const isBacklink = (el: Element) =>
  epubTypes(el).has("backlink") || roles(el).has("doc-backlink");

const isSuper = (el: Element | null) => {
  if (!el) return false;
  if (el.matches("sup")) return true;
  const view = el.ownerDocument?.defaultView;
  if (!view) return false;
  const { verticalAlign } = view.getComputedStyle(el);
  return (
    verticalAlign === "super" ||
    verticalAlign === "top" ||
    verticalAlign === "text-top" ||
    /^\d/.test(verticalAlign)
  );
};

// Whether a link in the book points at a note to show in place.
export const isFootnoteReference = (a: Element): boolean => {
  const types = epubTypes(a);
  const linkRoles = roles(a);
  if (REF_ROLES.some((r) => linkRoles.has(r))) return true;
  if (REF_TYPES.some((t) => types.has(t))) return true;
  if (isBacklink(a)) return false;
  return (
    isSuper(a) ||
    (a.children.length === 1 && isSuper(a.children[0])) ||
    isSuper(a.parentElement)
  );
};

const INLINE = "a, span, sup, sub, em, strong, i, b, small, big";

// The note a link's target belongs to: the list item or aside around it, or
// else the target itself, or the block around it when the target is an
// anchor inside the note's text.
const noteBlock = (target: Element): Element | null => {
  // A note in a list item or an aside is the whole item, however many
  // paragraphs it has.
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

const clean = (text: string | null) => (text ?? "").replace(/\s+/g, " ").trim();

// A note's text, one string per paragraph, without its link back.
export const footnoteText = (target: Element | null): string[] => {
  if (!target) return [];
  const block = noteBlock(target);
  if (!block) return [];
  const note = block.cloneNode(true) as Element;
  note.querySelectorAll("a").forEach((a) => isBacklink(a) && a.remove());
  const paragraphs = [...note.querySelectorAll("p")];
  const texts = paragraphs.length
    ? paragraphs.map((p) => clean(p.textContent))
    : [clean(note.textContent)];
  return texts.filter(Boolean);
};
