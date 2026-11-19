// Where the arrow keys and A and D belong to a control (the font size slider
// in the reader's settings popover, the chapter menu and its open list), not
// to page turns. A stray page turn would count as reading and could resume or
// finish a book.
const CONTROL_KEYS =
  '[role="dialog"], [role="combobox"], [role="listbox"], input, select, textarea, [contenteditable="true"]';

export type PageTurn = "next" | "prev";

// The page turn a rightward or leftward input makes once the Reading Direction
// is applied. Inputs name the turn as a left to right book takes it, so in a
// right to left book they run the other way: right goes back.
export const turnInDirection = <T extends PageTurn | null>(
  turn: T,
  rightToLeft: boolean,
): T => (turn && rightToLeft ? (turn === "next" ? "prev" : "next") : turn) as T;

// The page turn a keydown in the reader asks for, or null for none.
export const pageTurnForKey = (event: KeyboardEvent): PageTurn | null => {
  // Browser shortcuts such as Cmd+D and Alt+Left aren't page turns.
  if (event.ctrlKey || event.metaKey || event.altKey) return null;
  // Shift+Arrow extends a selection in the book.
  if (event.shiftKey && event.key.startsWith("Arrow")) return null;
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

export type SearchShortcut = "find" | "next" | "previous";

// Apple devices, iPads with a keyboard included, take shortcuts with Cmd. Ctrl
// is theirs to keep: in a Mac text field Ctrl+F moves the cursor.
export const isApplePlatform = () =>
  /Mac|iPhone|iPad|iPod/.test(navigator.platform) ||
  /Mac OS X|iPhone|iPad/.test(navigator.userAgent);

// The letter a key types, or on a layout without Latin letters (Cyrillic,
// Greek, Hebrew), the Latin letter in its place, as browsers match shortcuts.
const shortcutLetter = (event: KeyboardEvent) => {
  if (/^[a-z]$/i.test(event.key)) return event.key.toLowerCase();
  const code = /^Key([A-Z])$/.exec(event.code);
  return code ? code[1].toLowerCase() : null;
};

// The search a keydown in the reader asks for, or null for none: the
// browser's own find keys, which can't search a Book, since the reader only
// ever holds the part of it on screen. Cmd on Apple devices, Ctrl elsewhere,
// and F3 as Windows and Linux use it.
export const searchShortcutForKey = (
  event: KeyboardEvent,
  apple = isApplePlatform(),
): SearchShortcut | null => {
  // Ctrl+Alt is AltGr on Windows, which types characters.
  if (event.altKey) return null;
  const modifier = apple ? event.metaKey : event.ctrlKey;
  const other = apple ? event.ctrlKey : event.metaKey;
  if (modifier && !other) {
    const letter = shortcutLetter(event);
    if (letter === "f" && !event.shiftKey) return "find";
    if (letter === "g") return event.shiftKey ? "previous" : "next";
    return null;
  }
  if (event.key === "F3" && !event.ctrlKey && !event.metaKey) {
    return event.shiftKey ? "previous" : "next";
  }
  return null;
};
