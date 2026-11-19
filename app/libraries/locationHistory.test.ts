import { describe, expect, it } from "vitest";

import { LocationHistory } from "./locationHistory";

// Nothing is on screen but the place the reader is at.
const notOnScreen = () => false;

describe("LocationHistory", () => {
  it("starts with nowhere to go", () => {
    const history = new LocationHistory();
    expect(history.canGoBack).toBe(false);
    expect(history.canGoForward).toBe(false);
    expect(history.back("here", notOnScreen)).toBeNull();
    expect(history.forward("here", notOnScreen)).toBeNull();
  });

  it("goes back to where a jump left from, and forward to where back left", () => {
    const history = new LocationHistory();
    history.recordJump("start");
    expect(history.canGoBack).toBe(true);

    // The reader paged on from where the jump landed.
    expect(history.back("paged-on", notOnScreen)).toBe("start");
    expect(history.canGoBack).toBe(false);
    expect(history.canGoForward).toBe(true);
    expect(history.forward("start", notOnScreen)).toBe("paged-on");
    expect(history.canGoForward).toBe(false);
    expect(history.back("paged-on", notOnScreen)).toBe("start");
  });

  it("goes back through several jumps in turn", () => {
    const history = new LocationHistory();
    history.recordJump("a");
    history.recordJump("b");
    expect(history.back("c", notOnScreen)).toBe("b");
    expect(history.back("b", notOnScreen)).toBe("a");
    expect(history.forward("a", notOnScreen)).toBe("b");
    expect(history.forward("b", notOnScreen)).toBe("c");
  });

  it("forgets the way forward on a new jump", () => {
    const history = new LocationHistory();
    history.recordJump("a");
    history.back("b", notOnScreen);
    history.recordJump("a");
    expect(history.canGoForward).toBe(false);
    expect(history.back("c", notOnScreen)).toBe("a");
  });

  it("skips a place already on screen", () => {
    const history = new LocationHistory();
    history.recordJump("a");
    history.recordJump("b");
    // The reader paged back to b by hand: going back to it would do nothing.
    expect(history.back("b-page", (cfi) => cfi === "b")).toBe("a");
    expect(history.forward("a", (cfi) => cfi === "b-page")).toBeNull();
  });

  it("drops entries that are all on screen", () => {
    const history = new LocationHistory();
    history.recordJump("a");
    expect(history.back("a", (cfi) => cfi === "a")).toBeNull();
    expect(history.canGoBack).toBe(false);
    expect(history.canGoForward).toBe(false);
  });

  it("keeps one entry per group of jumps, the first", () => {
    const search = {};
    const history = new LocationHistory();
    history.recordJump("before-search", search);
    history.recordJump("result-1", search);
    history.recordJump("result-2", search);
    expect(history.back("result-3", notOnScreen)).toBe("before-search");
    expect(history.canGoBack).toBe(false);
  });

  it("starts a group again after another jump or going back", () => {
    const search = {};
    const history = new LocationHistory();
    history.recordJump("before-search", search);
    history.recordJump("result-1");
    history.recordJump("link-target", search);
    expect(history.back("result-2", notOnScreen)).toBe("link-target");

    history.recordJump("link-target", search);
    expect(history.back("result-3", notOnScreen)).toBe("link-target");
    expect(history.back("link-target", notOnScreen)).toBe("result-1");
  });

  it("tells a new group from an old one", () => {
    const history = new LocationHistory();
    history.recordJump("before-first", {});
    history.recordJump("before-second", {});
    expect(history.back("here", notOnScreen)).toBe("before-second");
    expect(history.back("before-second", notOnScreen)).toBe("before-first");
  });

  it("puts back a move that didn't happen", () => {
    const search = {};
    const history = new LocationHistory();
    history.recordJump("a");
    history.recordJump("b", search);
    const saved = history.save();
    expect(history.back("c", notOnScreen)).toBe("b");
    history.restore(saved);
    expect(history.canGoForward).toBe(false);
    // The group survives too, so the search still has one entry.
    history.recordJump("result", search);
    expect(history.back("c", notOnScreen)).toBe("b");
    expect(history.back("b", notOnScreen)).toBe("a");
  });
});
