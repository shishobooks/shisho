import type { UseQueryResult } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import { ShishoAPIError } from "@/libraries/api";
import type { ResourceListResponse } from "@/types";

import ResourceList from "./ResourceList";

vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
vi.mock("@/components/library/PaginationFooter", () => ({
  default: () => null,
}));

type Item = { id: number; name: string };

const state = {
  libraryId: "1",
  currentPage: 1,
  searchQuery: "",
  debouncedSearch: "",
  handleDebouncedSearchChange: vi.fn(),
  queryParams: { limit: 50, offset: 0 },
  limit: 50,
  offset: 0,
  handlePageChange: vi.fn(),
};

const list = (
  query: Partial<UseQueryResult<ResourceListResponse<Item>>>,
  debouncedSearch = "",
) => (
  <MemoryRouter>
    <ResourceList<Item>
      itemConfig={(item) => ({ name: item.name, aliases: [], badges: [] })}
      itemLabel="genres"
      linkTo={(item) => `/libraries/1/genres/${item.id}`}
      query={query as UseQueryResult<ResourceListResponse<Item>>}
      searchLabel="Search genres"
      searchPlaceholder="Search genres..."
      state={{ ...state, debouncedSearch }}
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
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
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
});
