// What the ebook reader reads from foliate's renderer to tell the end.
export interface EndRenderer {
  scrolled?: boolean;
  atEnd?: boolean;
  start?: number;
  size?: number;
  viewSize?: number;
}

// Whether a scrolled reader is still on its section's first screen, where
// scrolling a little isn't reading yet.
export const withinFirstScreen = (renderer: EndRenderer | undefined): boolean =>
  Boolean(renderer?.scrolled) && (renderer?.start ?? 0) < (renderer?.size ?? 0);

// How close to the bottom counts as there, as foliate's own scrolling
// allows.
const SCROLL_SLACK_PX = 2;

// Whether the reader is at the end of the book, which finishes the
// Read-through. foliate's atEnd is a paginated test (the last page or so of
// the last section), which in scrolled flow comes about a screen early, so
// there the end is the bottom of the last section in the reading order.
export const readerAtEnd = (
  renderer: EndRenderer | undefined,
  sectionIndex: number | undefined,
  sections: ReadonlyArray<{ linear?: string }> | undefined,
): boolean => {
  if (!renderer) return false;
  if (!renderer.scrolled) return renderer.atEnd ?? false;
  if (sectionIndex === undefined || !sections) return false;
  let last = sections.length - 1;
  while (last > 0 && sections[last].linear === "no") last--;
  if (sectionIndex < last) return false;
  const { start = 0, size = 0, viewSize = 0 } = renderer;
  return viewSize - (start + size) <= SCROLL_SLACK_PX;
};
