import { describe, expect, it } from "vitest";

import { drawChosenMatch, drawMatch } from "./searchMarks";

// A match on two lines, as foliate measures it.
const RECTS = [
  { left: 10, top: 20, width: 50, height: 16 },
  { left: 0, top: 40, width: 30, height: 16 },
];

const rects = (g: SVGElement) =>
  Array.from(g.querySelectorAll("rect"), (rect) => ({
    x: Number(rect.getAttribute("x")),
    y: Number(rect.getAttribute("y")),
    width: Number(rect.getAttribute("width")),
    height: Number(rect.getAttribute("height")),
  }));

describe("search marks", () => {
  it("shades each line of a match with no border", () => {
    const g = drawMatch(RECTS);
    expect(g.getAttribute("data-search-mark")).toBe("match");
    expect(g.getAttribute("stroke")).toBeNull();
    expect(rects(g)).toEqual([
      { x: 10, y: 20, width: 50, height: 16 },
      { x: 0, y: 40, width: 30, height: 16 },
    ]);
  });

  it("marks the chosen match more strongly, a little past its text, with no border", () => {
    const g = drawChosenMatch(RECTS);
    expect(g.getAttribute("data-search-mark")).toBe("chosen");
    expect(g.getAttribute("stroke")).toBeNull();
    expect(Number(g.style.opacity)).toBeGreaterThan(
      Number(drawMatch(RECTS).style.opacity),
    );
    expect(rects(g)).toEqual([
      { x: 8, y: 18, width: 54, height: 20 },
      { x: -2, y: 38, width: 34, height: 20 },
    ]);
  });
});
