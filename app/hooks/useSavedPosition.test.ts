import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useSavedPosition } from "./useSavedPosition";

const query = vi.hoisted(() => ({
  current: {} as Record<string, unknown>,
}));

vi.mock("@/hooks/queries/reading", () => ({
  useFilePosition: () => query.current,
}));

const failedLoad = {
  data: undefined,
  error: new Error("offline"),
  isEnabled: true,
  isFetching: false,
  isFetchedAfterMount: true,
};

describe("useSavedPosition", () => {
  beforeEach(() => {
    query.current = failedLoad;
  });

  // A failed query refetches on window focus. The reader opened at the start
  // by then, so a turn there mustn't replace the place the refetch found.
  it("stays failed when a later refetch succeeds", () => {
    const { result, rerender } = renderHook(() => useSavedPosition(1));
    expect(result.current.failed).toBe(true);
    expect(result.current.settled).toBe(true);

    query.current = {
      data: { position: { page: 12 } },
      error: null,
      isEnabled: true,
      isFetching: false,
      isFetchedAfterMount: true,
    };
    rerender();

    expect(result.current.failed).toBe(true);
  });

  it("isn't failed by a background refetch failing after a good load", () => {
    query.current = {
      data: { position: { page: 12 } },
      error: null,
      isEnabled: true,
      isFetching: false,
      isFetchedAfterMount: true,
    };
    const { result, rerender } = renderHook(() => useSavedPosition(1));
    expect(result.current.saved).toEqual({ page: 12 });

    query.current = { ...failedLoad, data: { position: { page: 12 } } };
    rerender();

    expect(result.current.failed).toBe(false);
  });
});
