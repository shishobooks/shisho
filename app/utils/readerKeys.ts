// Where the arrow keys and A and D belong to a control (the font size slider
// in the reader's settings popover, the chapter menu), not to page turns. A
// stray page turn would count as reading and could resume or finish a book.
const CONTROL_KEYS =
  '[role="dialog"], input, select, textarea, [contenteditable="true"]';

// The page turn a keydown in the reader asks for, or null for none.
export const pageTurnForKey = (
  event: KeyboardEvent,
): "next" | "prev" | null => {
  const target = event.target as Element | null;
  if (target?.closest?.(CONTROL_KEYS)) return null;
  switch (event.key) {
    case "ArrowRight":
    case "d":
    case "D":
      return "next";
    case "ArrowLeft":
    case "a":
    case "A":
      return "prev";
    default:
      return null;
  }
};
