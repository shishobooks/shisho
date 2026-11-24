import { describe, expect, it } from "vitest";

import { stepPull } from "../../mobile/ios/Shisho/Reader/ReaderWeb/pull.js";

// The Shisho app's scrolled reader counts one pull past a section's end
// (positive) or start (negative) as the finger moves (#919). dy is how far
// the finger moved up since the last move.
const run = (moves: number[], edges: { atStart: boolean; atEnd: boolean }) =>
  moves.reduce(
    (amount, dy) => stepPull(amount, dy, edges.atStart, edges.atEnd),
    0,
  );

describe("stepPull", () => {
  it("starts past the end only moving on there", () => {
    expect(run([10, 20], { atStart: false, atEnd: true })).toBe(30);
    expect(run([10, 20], { atStart: false, atEnd: false })).toBe(0);
  });

  it("starts past the start only moving back there", () => {
    expect(run([-10, -20], { atStart: true, atEnd: false })).toBe(-30);
    expect(run([-10, -20], { atStart: false, atEnd: false })).toBe(0);
  });

  it("goes on once started, wherever the scroll reads", () => {
    expect(stepPull(-40, -20, false, false)).toBe(-60);
    expect(stepPull(40, 20, false, false)).toBe(60);
  });

  // On a page short enough to be at both edges, reversing undoes the pull
  // under way before one the other way can start, from nothing.
  it("undoes a pull before starting one the other way", () => {
    const step = (amount: number, dys: number[]) =>
      dys.reduce((a, dy) => stepPull(a, dy, true, true), amount);
    // Pulled back to -200, then four moves up end at nothing, not at a
    // pull forward.
    expect(step(0, [-50, -50, -50, -50])).toBe(-200);
    expect(step(-200, [50, 50, 50, 50])).toBe(0);
    // The next move up starts the pull forward from nothing.
    expect(step(0, [30])).toBe(30);
    // Back down, the pull forward goes to nothing before a pull back starts.
    expect(step(30, [-30])).toBe(0);
    expect(step(30, [-30, -30])).toBe(-30);
  });

  it("drops what's left of a move that crosses back over nothing", () => {
    expect(stepPull(20, -50, true, true)).toBe(0);
    expect(stepPull(-20, 50, true, true)).toBe(0);
  });
});
