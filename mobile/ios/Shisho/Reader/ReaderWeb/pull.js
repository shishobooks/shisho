// One pull past a scrolled section's end or start, counted as the finger
// moves: positive past the end, toward the next section, and negative past
// the start, toward the previous one. dy is how far the finger moved up
// since the last move; atStart and atEnd say whether the page is at that
// edge now. The two ways are the same rule, mirrored.
export const stepPull = (amount, dy, atStart, atEnd) => {
  // A pull under way follows the finger, whatever the scroll reads, and
  // stops at nothing: reversing undoes it before a pull the other way can
  // start, as on a page short enough to be at both edges.
  if (amount > 0) return Math.max(0, amount + dy);
  if (amount < 0) return Math.min(0, amount + dy);
  if (dy > 0 && atEnd) return dy;
  if (dy < 0 && atStart) return dy;
  return 0;
};
