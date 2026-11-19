// How foliate's overlay draws search matches over the book's text. A draw
// function takes the match's boxes, one per line it spans, in the overlay's
// coordinates, and returns an SVG group (see foliate's overlayer.js).

interface Box {
  left: number;
  top: number;
  width: number;
  height: number;
}

const SVG_NS = "http://www.w3.org/2000/svg";

// Marker-pen colors that show over the book's own colors in every reader
// theme, laid over the text rather than behind it, so they're see-through.
const MATCH_COLOR = "#facc15";
const CHOSEN_COLOR = "#f59e0b";

const draw = (
  boxes: Iterable<Box>,
  mark: "match" | "chosen",
  { color, opacity, pad }: { color: string; opacity: number; pad: number },
) => {
  const g = document.createElementNS(SVG_NS, "g");
  // What the mark is, for tests that can't see colors.
  g.setAttribute("data-search-mark", mark);
  g.setAttribute("fill", color);
  g.style.opacity = String(opacity);
  for (const { left, top, width, height } of boxes) {
    const rect = document.createElementNS(SVG_NS, "rect");
    rect.setAttribute("x", String(left - pad));
    rect.setAttribute("y", String(top - pad));
    rect.setAttribute("width", String(width + pad * 2));
    rect.setAttribute("height", String(height + pad * 2));
    rect.setAttribute("rx", "3");
    g.append(rect);
  }
  return g;
};

/** Every match of a search, lightly shaded. */
export const drawMatch = (boxes: Iterable<Box>) =>
  draw(boxes, "match", { color: MATCH_COLOR, opacity: 0.3, pad: 0 });

/**
 * The match the reader went to: drawn over its shading in a deeper amber
 * that reaches a little past the text, so it stands out among the matches
 * on the page without a border.
 */
export const drawChosenMatch = (boxes: Iterable<Box>) =>
  draw(boxes, "chosen", { color: CHOSEN_COLOR, opacity: 0.45, pad: 2 });
