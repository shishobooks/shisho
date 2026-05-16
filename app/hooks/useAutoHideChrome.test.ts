import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAutoHideChrome } from "./useAutoHideChrome";

beforeEach(() => {
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
});

describe("useAutoHideChrome", () => {
  it("always returns chromeVisible true when disabled", () => {
    const { result } = renderHook(() => useAutoHideChrome(false));
    expect(result.current.chromeVisible).toBe(true);

    act(() => vi.advanceTimersByTime(5000));
    expect(result.current.chromeVisible).toBe(true);
  });

  it("hides chrome after initial delay when enabled", () => {
    const { result } = renderHook(() => useAutoHideChrome(true));
    expect(result.current.chromeVisible).toBe(true);

    act(() => vi.advanceTimersByTime(2000));
    expect(result.current.chromeVisible).toBe(false);
  });

  it("re-shows chrome on mouse move and hides again after inactivity", () => {
    const { result } = renderHook(() => useAutoHideChrome(true));

    act(() => vi.advanceTimersByTime(2000));
    expect(result.current.chromeVisible).toBe(false);

    act(() => {
      window.dispatchEvent(new Event("mousemove"));
    });
    expect(result.current.chromeVisible).toBe(true);

    act(() => vi.advanceTimersByTime(3000));
    expect(result.current.chromeVisible).toBe(false);
  });

  it("toggleChrome toggles visibility and restarts inactivity timer", () => {
    const { result } = renderHook(() => useAutoHideChrome(true));

    act(() => vi.advanceTimersByTime(2000));
    expect(result.current.chromeVisible).toBe(false);

    act(() => result.current.toggleChrome());
    expect(result.current.chromeVisible).toBe(true);

    act(() => vi.advanceTimersByTime(3000));
    expect(result.current.chromeVisible).toBe(false);
  });

  it("toggleChrome hides chrome when currently visible", () => {
    const { result } = renderHook(() => useAutoHideChrome(true));
    expect(result.current.chromeVisible).toBe(true);

    act(() => result.current.toggleChrome());
    expect(result.current.chromeVisible).toBe(false);
  });

  it("toggleChrome is a no-op when disabled", () => {
    const { result } = renderHook(() => useAutoHideChrome(false));
    act(() => result.current.toggleChrome());
    expect(result.current.chromeVisible).toBe(true);
  });

  it("resets to visible when switching from enabled to disabled", () => {
    const { result, rerender } = renderHook(
      ({ enabled }) => useAutoHideChrome(enabled),
      { initialProps: { enabled: true } },
    );

    act(() => vi.advanceTimersByTime(2000));
    expect(result.current.chromeVisible).toBe(false);

    rerender({ enabled: false });
    expect(result.current.chromeVisible).toBe(true);
  });
});
