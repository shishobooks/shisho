import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installStaleChunkReload } from "./staleChunkReload";

const firePreloadError = () => {
  const event = new Event("vite:preloadError", { cancelable: true });
  window.dispatchEvent(event);
  return event;
};

describe("installStaleChunkReload", () => {
  let reload: ReturnType<typeof vi.fn<() => void>>;
  let uninstall: () => void;

  beforeEach(() => {
    window.sessionStorage.clear();
    vi.setSystemTime(new Date("2026-11-17T12:00:00Z"));
    reload = vi.fn<() => void>();
    uninstall = installStaleChunkReload(reload);
  });

  afterEach(() => {
    uninstall();
    vi.restoreAllMocks();
  });

  it("reloads the page when a chunk from an older build fails to load", () => {
    const event = firePreloadError();
    expect(reload).toHaveBeenCalledTimes(1);
    // The original error still reaches the caller in case the reload is slow.
    expect(event.defaultPrevented).toBe(false);
  });

  it("does not reload again right after a reload, so a broken build cannot loop", () => {
    firePreloadError();
    uninstall();
    // Simulate the page coming back after the reload.
    uninstall = installStaleChunkReload(reload);
    vi.advanceTimersByTime(2_000);
    firePreloadError();
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("reloads again once enough time has passed", () => {
    firePreloadError();
    vi.advanceTimersByTime(60_000);
    firePreloadError();
    expect(reload).toHaveBeenCalledTimes(2);
  });

  it("does not reload while offline, since the reload would fail too", () => {
    vi.spyOn(window.navigator, "onLine", "get").mockReturnValue(false);
    firePreloadError();
    expect(reload).not.toHaveBeenCalled();
  });

  it("still reloads when session storage is unavailable", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    firePreloadError();
    expect(reload).toHaveBeenCalledTimes(1);
  });
});
