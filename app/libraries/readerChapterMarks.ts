// Where each chapter in a reflowable book's table of contents starts along
// the reader's progress bar, from 0 to 1.
//
// foliate measures progress by section size (`getSectionFractions()`), so a
// chapter that starts a section sits exactly at that section's fraction. A
// chapter that starts partway through a section (several chapters in one
// file) is placed by how much of the section's text comes before it, which
// tracks the same sizes closely. Elements here come from section documents,
// another realm, so nothing uses `instanceof`.

// What foliate's resolveHref gives for a link: the section, and a function
// finding the place in the section's document.
export interface ResolvedHref {
  index: number;
  anchor: (doc: Document) => unknown;
}

// The parts of a foliate book the reader uses.
export interface FoliateBook {
  // Asynchronous in an AZW3 book.
  resolveHref?: (
    href: string,
  ) => ResolvedHref | null | Promise<ResolvedHref | null>;
  sections?: Array<{ createDocument?: () => Promise<Document> }>;
}

export interface ChapterMark {
  position: number;
  // The names of the chapters that start here.
  label: string;
  // The first of those chapters' links, for going to its exact start.
  href: string;
}

// Chapters closer than this share one mark: the bar can't show them apart.
const MIN_GAP = 0.004;

// Lets the page draw and take input once a stretch of work has run this
// long. Yielding after every section would crawl in a background tab, where
// timers wait a second or more.
const WORK_BUDGET_MS = 16;
const breathe = () => new Promise((resolve) => setTimeout(resolve, 0));

// The share of a section's text before each target, in one pass over the
// text however many targets there are. A target's share counts the text of
// every text node that comes before it in the document.
const sharesBefore = (doc: Document, targets: Element[]) => {
  const shares = new Map<Element, number>();
  const body = doc.body;
  const total = body?.textContent?.length ?? 0;
  if (!body || total === 0) return shares;
  // Document order, so the walk below meets them in turn.
  const pending = [...targets].sort((a, b) =>
    a === b ? 0 : a.compareDocumentPosition(b) & 4 ? -1 : 1,
  );
  const walker = doc.createTreeWalker(body, NodeFilter.SHOW_TEXT);
  let before = 0;
  for (let text = walker.nextNode(); text; text = walker.nextNode()) {
    // A target reached: this text is inside it or after it.
    while (
      pending.length &&
      (pending[0].contains(text) ||
        pending[0].compareDocumentPosition(text) & 4)
    ) {
      shares.set(pending.shift()!, before / total);
    }
    before += text.textContent?.length ?? 0;
  }
  for (const target of pending) shares.set(target, before / total);
  return shares;
};

// A mark for each entry's chapter, in book order, or null if `cancelled`
// says the answer is no longer wanted. Reading a section's document is the
// slow part, so each is read at most once, one at a time, and let go before
// the next.
export const chapterMarks = async (
  entries: Array<{ label: string; href: string }>,
  book: FoliateBook,
  sectionFractions: number[],
  cancelled: () => boolean = () => false,
): Promise<ChapterMark[] | null> => {
  const marks: ChapterMark[] = [];
  // The chapters partway through each section, to place once it's read.
  const inSections = new Map<
    number,
    Array<{ label: string; href: string; resolved: ResolvedHref }>
  >();
  for (const { label, href } of entries) {
    if (!href) continue;
    const resolved = await book.resolveHref?.(href);
    if (!resolved || resolved.index < 0) continue;
    const start = sectionFractions[resolved.index];
    if (start === undefined) continue;
    if (!href.includes("#")) {
      marks.push({ position: start, label, href });
      continue;
    }
    const waiting = inSections.get(resolved.index) ?? [];
    waiting.push({ label, href, resolved });
    inSections.set(resolved.index, waiting);
  }

  let workedSince = performance.now();
  for (const [index, waiting] of [...inSections].sort(([a], [b]) => a - b)) {
    if (performance.now() - workedSince > WORK_BUDGET_MS) {
      await breathe();
      workedSince = performance.now();
    }
    if (cancelled()) return null;
    const start = sectionFractions[index];
    const end = sectionFractions[index + 1] ?? 1;
    const doc = await book.sections?.[index]?.createDocument?.();
    if (cancelled()) return null;
    const targets = waiting.map(({ resolved }) => {
      const target = doc ? resolved.anchor(doc) : null;
      return (target as Node | null)?.nodeType === 1
        ? (target as Element)
        : null;
    });
    const shares = doc
      ? sharesBefore(
          doc,
          targets.filter((t): t is Element => t !== null),
        )
      : new Map<Element, number>();
    waiting.forEach(({ label, href }, i) => {
      const share = targets[i] ? (shares.get(targets[i]!) ?? 0) : 0;
      marks.push({ position: start + share * (end - start), label, href });
    });
  }

  // A stable sort keeps the contents' order among chapters at one place, so
  // a part comes before its first chapter.
  marks.sort((a, b) => a.position - b.position);
  const merged: ChapterMark[] = [];
  for (const mark of marks) {
    const last = merged[merged.length - 1];
    if (last && mark.position - last.position < MIN_GAP) {
      if (!last.label.split(" / ").includes(mark.label)) {
        last.label = `${last.label} / ${mark.label}`;
      }
      continue;
    }
    merged.push({ ...mark });
  }
  return merged;
};
