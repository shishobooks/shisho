import { describe, expect, it } from "vitest";

import { readerAtEnd } from "@/utils/readerEnd";

// Ten sections, the last one outside the reading order (an index, say).
const sections = [
  ...Array.from({ length: 9 }, () => ({ linear: "yes" })),
  { linear: "no" },
];

const scrolled = (start: number) => ({
  scrolled: true,
  atEnd: true, // foliate's paginated test, which is wrong here
  start,
  size: 800,
  viewSize: 5000,
});

describe("readerAtEnd", () => {
  it("uses foliate's own test in paginated flow", () => {
    expect(readerAtEnd({ scrolled: false, atEnd: true }, 8, sections)).toBe(
      true,
    );
    expect(readerAtEnd({ scrolled: false, atEnd: false }, 8, sections)).toBe(
      false,
    );
  });

  it("is the bottom of the last section in the reading order in scrolled flow", () => {
    expect(readerAtEnd(scrolled(4200), 8, sections)).toBe(true);
    expect(readerAtEnd(scrolled(4199), 8, sections)).toBe(true);
  });

  it("isn't a screen before the bottom in scrolled flow", () => {
    expect(readerAtEnd(scrolled(3400), 8, sections)).toBe(false);
    expect(readerAtEnd(scrolled(4150), 8, sections)).toBe(false);
  });

  it("isn't the bottom of an earlier section in scrolled flow", () => {
    expect(readerAtEnd(scrolled(4200), 7, sections)).toBe(false);
  });

  it("is nothing without a renderer", () => {
    expect(readerAtEnd(undefined, 8, sections)).toBe(false);
  });
});
