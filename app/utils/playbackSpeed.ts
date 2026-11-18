// Audiobook playback speed in the web player. Speed moves in 0.1 steps
// between the limits and is remembered per Book in this browser's storage,
// never on the server, so each narrator keeps its own speed here and a Book
// with none saved starts at 1x.

export const MIN_SPEED = 0.5;
export const MAX_SPEED = 4;
export const DEFAULT_SPEED = 1;

// A few common speeds to pick directly, besides stepping.
export const SPEED_PRESETS = [0.75, 1, 1.25, 1.5, 2, 3] as const;

const clamp = (speed: number) =>
  Math.min(MAX_SPEED, Math.max(MIN_SPEED, speed));

/**
 * The next 0.1 step from speed in direction (1 up, -1 down), within the
 * limits. A speed between steps, such as the 1.25x preset, moves to the
 * nearest step that way. Works in tenths so the result has no float drift.
 */
export const stepSpeed = (speed: number, direction: 1 | -1): number => {
  const tenths = speed * 10;
  const nearest = Math.round(tenths);
  const onStep = Math.abs(tenths - nearest) < 1e-6;
  const next =
    direction > 0
      ? (onStep ? nearest : Math.floor(tenths)) + 1
      : (onStep ? nearest : Math.ceil(tenths)) - 1;
  return clamp(next / 10);
};

/** Formats a speed for display: 1 → "1x", 1.4 → "1.4x". */
export const formatSpeed = (speed: number): string =>
  `${Number(speed.toFixed(2))}x`;

const storageKey = (bookId: number) => `shisho-playback-speed-${bookId}`;

/** The speed saved for bookId in this browser, or 1x when there is none. */
export const loadBookSpeed = (bookId: number): number => {
  try {
    const stored = localStorage.getItem(storageKey(bookId));
    if (!stored) return DEFAULT_SPEED;
    const speed = Number(stored);
    if (!Number.isFinite(speed) || speed < MIN_SPEED || speed > MAX_SPEED) {
      return DEFAULT_SPEED;
    }
    return speed;
  } catch {
    return DEFAULT_SPEED;
  }
};

/**
 * Remembers speed for bookId in this browser. 1x is what a Book starts at, so
 * it's forgotten rather than stored. A browser that refuses storage keeps the
 * speed for this visit only.
 */
export const saveBookSpeed = (bookId: number, speed: number) => {
  try {
    if (speed === DEFAULT_SPEED) {
      localStorage.removeItem(storageKey(bookId));
    } else {
      localStorage.setItem(storageKey(bookId), String(speed));
    }
  } catch {
    // Storage blocked or full: the speed still applies until the page closes.
  }
};
