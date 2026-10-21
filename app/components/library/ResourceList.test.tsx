import type { UseQueryResult } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { ShishoAPIError } from "@/libraries/api";
import { BOOK_COUNT_RESOURCE_SORT } from "@/libraries/resourceSort";
import type { SortLevel } from "@/libraries/sortSpec";
import { setAuth } from "@/testing/auth";
import { mockCoverDimensions } from "@/testing/coverDimensions";
import type { PreviewBook, ResourceListResponse } from "@/types";

import ResourceList from "./ResourceList";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
const library = vi.hoisted(() => ({ cover_aspect_ratio: "book" }));
vi.mock("@/hooks/queries/libraries", () => ({
  useUserLibrary: () => ({ data: library }),
}));
mockCoverDimensions(38, 56);

vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

// The Sort control picks a sheet or a drawer by viewport width.
beforeAll(() => {
  window.matchMedia = vi.fn(() => ({
    matches: true,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })) as unknown as typeof window.matchMedia;
});

type Item = {
  id: number;
  name: string;
  preview_books?: PreviewBook[];
};
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
  columns: 1,
  pageSizeResolved: true,
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
      itemConfig={(item) => ({
        name: item.name,
        aliases: [],
        badges: [],
        previewBooks: item.preview_books,
      })}
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

  it("asks for the last page, replacing history, when the page is past the end", () => {
    const handlePageChange = vi.fn();
    const pastTheEnd: Partial<UseQueryResult<ResourceListResponse<Item>>> = {
      ...settled,
      data: { items: [], total: 120 },
    };
    render(
      <MemoryRouter>
        <ResourceList<Item, Field>
          itemConfig={(item) => ({
            name: item.name,
            aliases: [],
            badges: [],
            previewBooks: item.preview_books,
          })}
          itemLabel="genres"
          linkTo={(item) => `/libraries/1/genres/${item.id}`}
          query={pastTheEnd as UseQueryResult<ResourceListResponse<Item>>}
          searchLabel="Search genres"
          state={{
            ...state,
            currentPage: 9,
            offset: 400,
            handlePageChange,
          }}
          subtitle="Browse genres"
          title="Genres"
        />
      </MemoryRouter>,
    );

    // 120 genres at 50 per page is three pages.
    expect(handlePageChange).toHaveBeenCalledWith(3, { replace: true });
    expect(
      screen.queryByText("No genres in this library yet."),
    ).not.toBeInTheDocument();
  });

  it("writes each badge's count with its noun and thousands grouped", () => {
    const oneGenre: Partial<UseQueryResult<ResourceListResponse<Item>>> = {
      ...settled,
      data: { items: [{ id: 1, name: "Fantasy" }], total: 1 },
    };
    render(
      <MemoryRouter>
        <ResourceList<Item, Field>
          itemConfig={(item) => ({
            name: item.name,
            aliases: [],
            badges: [
              { count: 1200, singular: "book", plural: "books" },
              { count: 1, singular: "file narrated", plural: "files narrated" },
            ],
          })}
          itemLabel="genres"
          linkTo={(item) => `/libraries/1/genres/${item.id}`}
          query={oneGenre as UseQueryResult<ResourceListResponse<Item>>}
          searchLabel="Search genres"
          state={state}
          subtitle="Browse genres"
          title="Genres"
        />
      </MemoryRouter>,
    );

    expect(screen.getByText("1,200 books")).toBeInTheDocument();
    expect(screen.getByText("1 file narrated")).toBeInTheDocument();
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

  describe("covers", () => {
    afterEach(() => {
      library.cover_aspect_ratio = "book";
    });

    const withPreviews = {
      ...settled,
      isSuccess: true,
      data: {
        items: [
          {
            id: 1,
            name: "Fantasy",
            preview_books: [
              {
                id: 7,
                title: "Dune",
                cover_cache_key: "7-1",
                cover_aspect: "book" as const,
              },
              {
                id: 8,
                title: "Emma",
                cover_cache_key: "8-1",
                cover_aspect: "square" as const,
              },
            ],
          },
        ],
        total: 1,
      },
    };

    it("shows an entry's book covers to a role that can read books", () => {
      setAuth({ permissions: ["books:read"] });
      const { container } = renderList(withPreviews);

      expect(container.querySelectorAll("img")).toHaveLength(2);
    });

    it("sizes the cover slot for the library's cover preference", () => {
      setAuth({ permissions: ["books:read"] });
      library.cover_aspect_ratio = "audiobook_fallback_book";
      const { container } = renderList(withPreviews);

      const slot = container.querySelector("img")!.parentElement!;
      expect(slot.style.width).toBe("80px");
    });

    it("leaves the cover slot out for a role that cannot read books", () => {
      setAuth({ permissions: ["people:read"] });
      const { container } = renderList(withPreviews);

      expect(screen.getByText("Fantasy")).toBeInTheDocument();
      expect(container.querySelectorAll("img")).toHaveLength(0);
      expect(screen.queryByTestId("cover-fan-placeholder")).toBeNull();
    });
  });
});
