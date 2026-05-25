import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { useDebounce } from "./useDebounce";

describe("useDebounce", () => {
  it("debounces value changes by the specified delay", () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDebounce(value, 300),
      { initialProps: { value: "" } },
    );

    expect(result.current).toBe("");

    rerender({ value: "hello" });
    expect(result.current).toBe("");

    act(() => {
      vi.advanceTimersByTime(300);
    });
    expect(result.current).toBe("hello");
  });

  it("resets the timer when value changes before delay expires", () => {
    const { result, rerender } = renderHook(
      ({ value }) => useDebounce(value, 300),
      { initialProps: { value: "" } },
    );

    rerender({ value: "a" });
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(result.current).toBe("");

    rerender({ value: "ab" });
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(result.current).toBe("");

    act(() => {
      vi.advanceTimersByTime(100);
    });
    expect(result.current).toBe("ab");
  });

  describe("immediate option", () => {
    const immediateEmpty = { immediate: (v: string) => v === "" };

    it("updates synchronously when the predicate matches", () => {
      const { result, rerender } = renderHook(
        ({ value }) => useDebounce(value, 300, immediateEmpty),
        { initialProps: { value: "initial" } },
      );

      rerender({ value: "" });
      expect(result.current).toBe("");
    });

    it("still debounces values that do not match the predicate", () => {
      const { result, rerender } = renderHook(
        ({ value }) => useDebounce(value, 300, immediateEmpty),
        { initialProps: { value: "" } },
      );

      rerender({ value: "search" });
      expect(result.current).toBe("");

      act(() => {
        vi.advanceTimersByTime(300);
      });
      expect(result.current).toBe("search");
    });

    it("handles non-empty → empty (immediate) → non-empty (debounced)", () => {
      const { result, rerender } = renderHook(
        ({ value }) => useDebounce(value, 300, immediateEmpty),
        { initialProps: { value: "" } },
      );

      rerender({ value: "abc" });
      act(() => {
        vi.advanceTimersByTime(300);
      });
      expect(result.current).toBe("abc");

      rerender({ value: "" });
      expect(result.current).toBe("");

      rerender({ value: "xyz" });
      expect(result.current).toBe("");

      act(() => {
        vi.advanceTimersByTime(300);
      });
      expect(result.current).toBe("xyz");
    });

    it("propagates empty immediately during rapid toggling", () => {
      const { result, rerender } = renderHook(
        ({ value }) => useDebounce(value, 200, immediateEmpty),
        { initialProps: { value: "" } },
      );

      rerender({ value: "abc" });
      act(() => {
        vi.advanceTimersByTime(200);
      });
      expect(result.current).toBe("abc");

      rerender({ value: "" });
      expect(result.current).toBe("");

      rerender({ value: "xyz" });
      act(() => {
        vi.advanceTimersByTime(100);
      });
      expect(result.current).toBe("");

      act(() => {
        vi.advanceTimersByTime(100);
      });
      expect(result.current).toBe("xyz");
    });
  });
});
