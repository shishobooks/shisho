import { act, render } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { describe, expect, it } from "vitest";

import { BOOK_COUNT_RESOURCE_SORT } from "@/libraries/resourceSort";

import {
  useResourceListState,
  type ResourceListState,
} from "./useResourceListState";

const renderState = (entry: string) => {
  const result: {
    current?: Pick<ResourceListState<string>, "handlePageChange">;
  } = {};
  const Harness = () => {
    result.current = useResourceListState(BOOK_COUNT_RESOURCE_SORT);
    return null;
  };
  const router = createMemoryRouter([{ path: "/", element: <Harness /> }], {
    initialEntries: [entry],
  });
  render(<RouterProvider router={router} />);
  return { router, result };
};

describe("useResourceListState", () => {
  it("adds a history entry for a page change", () => {
    const { router, result } = renderState("/?page=2");

    act(() => result.current?.handlePageChange(3));

    expect(router.state.location.search).toBe("?page=3");
    expect(router.state.historyAction).toBe("PUSH");
  });

  it("replaces the history entry when asked to", () => {
    const { router, result } = renderState("/?page=9");

    act(() => result.current?.handlePageChange(3, { replace: true }));

    expect(router.state.location.search).toBe("?page=3");
    expect(router.state.historyAction).toBe("REPLACE");
  });
});
