import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  pagesToProbe,
  PROBE_TIMEOUT_MS,
  useDetectedPageFlow,
} from "./useDetectedPageFlow";

// jsdom never loads images, so a fake Image reports the size each URL is
// given here, or fails or hangs, when the test says so.
const sizes = new Map<string, [number, number] | "error" | "hang">();
const pending: Array<() => void> = [];

class FakeImage {
  naturalWidth = 0;
  naturalHeight = 0;
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  set src(url: string) {
    const size = sizes.get(url);
    if (size === "hang") return;
    pending.push(() => {
      if (size === "error" || size === undefined) {
        this.onerror?.();
        return;
      }
      [this.naturalWidth, this.naturalHeight] = size;
      this.onload?.();
    });
  }
}

const loadAll = () =>
  act(async () => {
    pending.splice(0).forEach((load) => load());
  });

beforeEach(() => {
  sizes.clear();
  pending.length = 0;
  vi.stubGlobal("Image", FakeImage);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("pagesToProbe", () => {
  it("skips the cover when there are pages after it", () => {
    expect(pagesToProbe(100)).toEqual([1, 2]);
    expect(pagesToProbe(2)).toEqual([1]);
  });

  it("probes the only page of a one-page file", () => {
    expect(pagesToProbe(1)).toEqual([0]);
    expect(pagesToProbe(0)).toEqual([]);
  });
});

describe("useDetectedPageFlow", () => {
  it("scrolls a file whose pages are long strips", async () => {
    // Solo Leveling's strips are 940 by 11,281.
    sizes.set("/p1", [940, 11281]);
    sizes.set("/p2", [940, 11281]);
    const { result } = renderHook(() => useDetectedPageFlow(["/p1", "/p2"]));

    expect(result.current).toBeUndefined();
    await loadAll();
    expect(result.current).toBe("scrolled");
  });

  it("paginates a file of printed pages and double-page spreads", async () => {
    sizes.set("/p1", [1000, 1500]);
    sizes.set("/p2", [2000, 1500]);
    const { result } = renderHook(() => useDetectedPageFlow(["/p1", "/p2"]));

    await loadAll();
    expect(result.current).toBe("paginated");
  });

  it("scrolls when one probed page is a strip, such as after a credits page", async () => {
    sizes.set("/p1", [1000, 1500]);
    sizes.set("/p2", [800, 4000]);
    const { result } = renderHook(() => useDetectedPageFlow(["/p1", "/p2"]));

    await loadAll();
    expect(result.current).toBe("scrolled");
  });

  it("paginates when the pages fail to load", async () => {
    sizes.set("/p1", "error");
    const { result } = renderHook(() => useDetectedPageFlow(["/p1"]));

    await loadAll();
    expect(result.current).toBe("paginated");
  });

  it("stops waiting for a page that never loads", async () => {
    sizes.set("/p1", "hang");
    const { result } = renderHook(() => useDetectedPageFlow(["/p1"]));

    await act(async () => {
      vi.advanceTimersByTime(PROBE_TIMEOUT_MS);
    });
    expect(result.current).toBe("paginated");
  });

  it("paginates a file with no pages to probe", () => {
    const { result } = renderHook(() => useDetectedPageFlow([]));
    expect(result.current).toBe("paginated");
  });

  it("probes nothing without URLs", () => {
    const { result } = renderHook(() => useDetectedPageFlow(null));
    expect(result.current).toBeUndefined();
    expect(pending).toHaveLength(0);
  });
});
