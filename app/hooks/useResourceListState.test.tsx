import { act, render } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { describe, expect, it } from "vitest";

import { BOOK_COUNT_RESOURCE_SORT } from "@/libraries/resourceSort";

import {
  useResourceListState,
  type ResourceListState,
} from "./useResourceListState";

type BookCountSortField = (typeof BOOK_COUNT_RESOURCE_SORT)["fields"][number];

const renderState = (entry: string) => {
  const result: {
    current?: ResourceListState<BookCountSortField>;
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
  it("names a page by the index of its first entry", () => {
    const { router, result } = renderState("/");

    act(() => result.current?.handlePageChange(3));

    // The fallback width fits three columns, 48 entries per page.
    expect(router.state.location.search).toBe("?start=96");
    expect(router.state.historyAction).toBe("PUSH");
  });

  it("replaces the history entry when asked to", () => {
    const { router, result } = renderState("/?start=400");

    act(() => result.current?.handlePageChange(3, { replace: true }));

    expect(router.state.location.search).toBe("?start=96");
    expect(router.state.historyAction).toBe("REPLACE");
  });

  it("opens an old page-number link on the same page", () => {
    const { result } = renderState("/?page=2");

    expect(result.current?.offset).toBe(result.current?.limit);
    expect(result.current?.currentPage).toBe(2);
  });

  it("lays entries out in the columns the width fits, in whole rows", () => {
    const { result } = renderState("/");

    expect(result.current?.columns).toBe(3);
    expect(result.current?.limit).toBe(48);
    expect(result.current?.pageSizeResolved).toBe(true);
  });
});
