// Where each chapter in a reflowable book's table of contents starts along
// the reader's progress bar, from 0 to 1.
//
// foliate measures progress by section size (`getSectionFractions()`), so a
// chapter that starts a section sits exactly at that section's fraction. A
// chapter that starts partway through a section (several chapters in one
// file) is placed by how much of the section's text comes before it, which
// tracks the same sizes closely. Elements here come from section documents,
// another realm, so nothing uses `instanceof`.

interface ResolvedHref {
  index: number;
  anchor: (doc: Document) => unknown;
}

export interface MarkerBook {
  // Asynchronous in an AZW3 book.
  resolveHref?: (
    href: string,
  ) => ResolvedHref | null | Promise<ResolvedHref | null>;
  sections?: Array<{ createDocument?: () => Promise<Document> }>;
}

// Chapters closer than this share one marker: the bar can't show them apart.
const MIN_GAP = 0.004;

// The share of a section's text that comes before an element.
const textBefore = (doc: Document, target: Element) => {
  const body = doc.body;
  const total = body?.textContent?.length ?? 0;
  if (!body || total === 0) return 0;
  const range = doc.createRange();
  range.setStart(body, 0);
  range.setEndBefore(target);
  return range.toString().length / total;
};

// The positions of the entries' chapters, in book order, or null if
// `cancelled` says the answer is no longer wanted. Reading a section's
// document is the slow part, so each is read at most once.
export const chapterPositions = async (
  entries: Array<{ href: string }>,
  book: MarkerBook,
  sectionFractions: number[],
  cancelled: () => boolean = () => false,
): Promise<number[] | null> => {
  const docs = new Map<number, Promise<Document | undefined>>();
  const positions: number[] = [];
  for (const { href } of entries) {
    if (!href) continue;
    const resolved = await book.resolveHref?.(href);
    if (!resolved || resolved.index < 0) continue;
    const start = sectionFractions[resolved.index];
    if (start === undefined) continue;
    const end = sectionFractions[resolved.index + 1] ?? 1;
    if (!href.includes("#")) {
      positions.push(start);
      continue;
    }
    if (!docs.has(resolved.index)) {
      if (cancelled()) return null;
      docs.set(
        resolved.index,
        book.sections?.[resolved.index]?.createDocument?.() ??
          Promise.resolve(undefined),
      );
    }
    const doc = await docs.get(resolved.index);
    const target = doc ? resolved.anchor(doc) : null;
    const share =
      doc && (target as Node | null)?.nodeType === 1
        ? textBefore(doc, target as Element)
        : 0;
    positions.push(start + share * (end - start));
  }
  positions.sort((a, b) => a - b);
  return positions.filter(
    (position, i) => i === 0 || position - positions[i - 1] >= MIN_GAP,
  );
};
