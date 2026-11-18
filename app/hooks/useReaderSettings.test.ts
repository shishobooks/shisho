import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { READER_SETTINGS_KEY, useReaderSettings } from "./useReaderSettings";

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useReaderSettings", () => {
  it("keeps settings under a hyphenated key, as the app's other keys are", () => {
    expect(READER_SETTINGS_KEY).toBe("shisho-reader-settings");
  });

  it("keeps a change made after the callback was handed out", () => {
    const { result } = renderHook(() => useReaderSettings());
    // A font import holds on to the callback while it awaits the file.
    const [, staleUpdate] = result.current;

    act(() => result.current[1]({ theme: "dark" }));
    act(() => staleUpdate({ font: "literata" }));

    expect(result.current[0]).toEqual(
      expect.objectContaining({ theme: "dark", font: "literata" }),
    );
  });

  it("still applies a change when the browser refuses to store it", () => {
    localStorage.setItem(READER_SETTINGS_KEY, JSON.stringify({ size: 18 }));
    const { result } = renderHook(() => useReaderSettings());
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("full", "QuotaExceededError");
    });

    act(() => result.current[1]({ size: 24 }));

    expect(result.current[0].size).toBe(24);
  });
});
