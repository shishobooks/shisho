import type { UseQueryResult } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";
import { beforeAll, describe, expect, it, vi } from "vitest";

import { ShishoAPIError } from "@/libraries/api";
import { BOOK_COUNT_RESOURCE_SORT } from "@/libraries/resourceSort";
import type { SortLevel } from "@/libraries/sortSpec";
import type { ResourceListResponse } from "@/types";

import ResourceList from "./ResourceList";

vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
vi.mock("@/components/library/PaginationFooter", () => ({
  default: () => null,
}));

// The Sort control picks a sheet or a drawer by viewport width.
beforeAll(() => {
  window.matchMedia = vi.fn(() => ({
    matches: true,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })) as unknown as typeof window.matchMedia;
});

type Item = { id: number; name: string };
type Field = (typeof BOOK_COUNT_RESOURCE_SORT.fields)[number];

const defaultSort = {
  config: BOOK_COUNT_RESOURCE_SORT,
  levels: BOOK_COUNT_RESOURCE_SORT.defaultSort,
  isDirty: false,
  param: "name:asc",
  apply: vi.fn(),
  reset: vi.fn(),
};

const state = {
  libraryId: "1",
  currentPage: 1,
  searchQuery: "",
  setSearchQuery: vi.fn(),
  sort: defaultSort,
  queryParams: { limit: 50, offset: 0 },
  limit: 50,
  offset: 0,
  handlePageChange: vi.fn(),
};

const list = (
  query: Partial<UseQueryResult<ResourceListResponse<Item>>>,
  searchQuery = "",
  sort = defaultSort,
) => (
  <MemoryRouter>
    <ResourceList<Item, Field>
      itemConfig={(item) => ({ name: item.name, aliases: [], badges: [] })}
      itemLabel="genres"
      linkTo={(item) => `/libraries/1/genres/${item.id}`}
      query={query as UseQueryResult<ResourceListResponse<Item>>}
      searchLabel="Search genres"
      searchPlaceholder="Search genres..."
      state={{ ...state, searchQuery, sort }}
      subtitle="Browse genres"
      title="Genres"
    />
  </MemoryRouter>
);

const renderList = (
  query: Partial<UseQueryResult<ResourceListResponse<Item>>>,
) => render(list(query));

const settled = {
  isLoading: false,
  isFetching: false,
  isEnabled: true,
  refetch: vi.fn(),
} as const;

describe("ResourceList", () => {
  it("keeps the header and search and reports a failure where the results go", async () => {
    const refetch = vi.fn();
    renderList({
      data: undefined,
      error: new ShishoAPIError(
        "Internal Server Error",
        "internal_server_error",
        500,
      ),
      isError: true,
      isSuccess: false,
      isLoading: false,
      isFetching: false,
      isEnabled: true,
      refetch,
    });

    expect(screen.getByRole("heading", { name: "Genres" })).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Search genres...")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent(
      /^Failed to load genres/,
    );

    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetch).toHaveBeenCalledTimes(1);
  });

  it("reports a failed search after an earlier search loaded", () => {
    const view = render(
      list({
        ...settled,
        data: { items: [{ id: 1, name: "Fantasy" }], total: 1 },
        error: null,
        isError: false,
        isSuccess: true,
      }),
    );
    expect(screen.getByText("Fantasy")).toBeInTheDocument();

    view.rerender(
      list(
        {
          ...settled,
          data: undefined,
          error: new TypeError("Failed to fetch"),
          isError: true,
          isSuccess: false,
        },
        "fan",
      ),
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      /^Failed to load genres/,
    );
    expect(screen.queryByText("Loading...")).not.toBeInTheDocument();
  });

  it("keeps the loaded results when a background refetch fails", () => {
    renderList({
      ...settled,
      data: { items: [{ id: 1, name: "Fantasy" }], total: 1 },
      error: new TypeError("Failed to fetch"),
      isError: true,
      isSuccess: false,
    });

    expect(screen.getByText("Fantasy")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("announces the count and the empty state in one region that stays mounted", () => {
    const { rerender } = render(
      list({ ...settled, isLoading: true, isFetching: true }),
    );
    const region = screen
      .getAllByRole("status")
      .find((element) => !element.textContent?.includes("Loading"));
    expect(region).toHaveTextContent(/^$/);

    rerender(
      list({
        ...settled,
        data: { items: [{ id: 1, name: "Fantasy" }], total: 1 },
      }),
    );
    expect(region).toBeInTheDocument();
    expect(region).toHaveTextContent("Showing 1-1 of 1 genres");

    rerender(list({ ...settled, data: { items: [], total: 0 } }));
    expect(region).toBeInTheDocument();
    expect(region).toHaveTextContent("No genres in this library yet.");
  });

  it("shows a changed sort as chips that can be removed or reset", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const apply = vi.fn();
    const reset = vi.fn();
    const levels: SortLevel<Field>[] = [
      { field: "book_count", direction: "desc" },
      { field: "name", direction: "asc" },
    ];
    render(
      list({ ...settled, data: { items: [], total: 0 } }, "", {
        ...defaultSort,
        levels,
        isDirty: true,
        param: "book_count:desc,name:asc",
        apply,
        reset,
      }),
    );

    expect(screen.getByRole("button", { name: /^Sort/ })).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: /Book count descending/ }),
    );
    expect(apply).toHaveBeenCalledWith([{ field: "name", direction: "asc" }]);
    await user.click(screen.getByRole("button", { name: "reset to default" }));
    expect(reset).toHaveBeenCalled();
  });

  it("shows no sort chips for the default sort", () => {
    render(list({ ...settled, data: { items: [], total: 0 } }));
    expect(screen.queryByText("Sorted by:")).not.toBeInTheDocument();
  });
});
