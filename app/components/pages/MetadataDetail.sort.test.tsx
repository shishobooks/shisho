import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, waitFor } from "@testing-library/react";
import type { ComponentType, ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import {
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
  type MockInstance,
} from "vitest";

import { API } from "@/libraries/api";
import { ALL_PERMISSIONS, setAuth } from "@/testing/auth";

import GenreDetail from "./GenreDetail";
import PersonDetail from "./PersonDetail";
import SeriesDetail from "./SeriesDetail";
import TagDetail from "./TagDetail";

// Which books request each detail page sends: tag, genre, and person pages
// list through /books with a filter and the newest-release default, and the
// series page keeps its own endpoint, which keeps series order until a sort
// is chosen.

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
vi.mock("@/components/library/LibraryBreadcrumbs", () => ({
  default: () => <nav />,
}));

beforeAll(() => {
  window.matchMedia = vi.fn(() => ({
    matches: true,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })) as unknown as typeof window.matchMedia;
});

let request: MockInstance<typeof API.request>;

beforeEach(() => {
  setAuth({ permissions: ALL_PERMISSIONS });
  vi.restoreAllMocks();
  request = vi
    .spyOn(API, "request")
    .mockImplementation(async (_method, path) => {
      if (path === "/settings/user") return { gallery_size: "m" };
      if (/^\/(tags|genres|people|series)\/\d+$/.test(path)) {
        return {
          id: 5,
          library_id: 1,
          name: "Entity",
          aliases: [],
          book_count: 0,
        };
      }
      return { items: [], total: 0 };
    });
});

const renderAt = (Page: ComponentType, route: string, entry: string) =>
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter initialEntries={[entry]}>
        <Routes>
          <Route element={<Page />} path={route} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );

const booksQuery = async (path: string) => {
  await waitFor(() =>
    expect(request.mock.calls.some((call) => call[1] === path)).toBe(true),
  );
  return request.mock.calls.find((call) => call[1] === path)?.[3];
};

describe("detail page book sort", () => {
  it.each([
    ["TagDetail", TagDetail, "tags", { tag_ids: [5] }],
    ["GenreDetail", GenreDetail, "genres", { genre_ids: [5] }],
    ["PersonDetail", PersonDetail, "people", { person_id: 5 }],
  ] as const)(
    "%s lists /books newest release first by default",
    async (_name, Page, segment, filter) => {
      renderAt(
        Page,
        `/libraries/:libraryId/${segment}/:id`,
        `/libraries/1/${segment}/5`,
      );
      expect(await booksQuery("/books")).toEqual(
        expect.objectContaining({
          ...filter,
          library_id: 1,
          sort: "date_released:desc,title:asc",
        }),
      );
    },
  );

  it("sends the sort from the URL", async () => {
    renderAt(
      TagDetail,
      "/libraries/:libraryId/tags/:id",
      "/libraries/1/tags/5?sort=page_count:desc",
    );
    expect(await booksQuery("/books")).toEqual(
      expect.objectContaining({ tag_ids: [5], sort: "page_count:desc" }),
    );
  });

  it("keeps series order on a series page until a sort is chosen", async () => {
    renderAt(
      SeriesDetail,
      "/libraries/:libraryId/series/:id",
      "/libraries/1/series/5",
    );
    const query = await booksQuery("/series/5/books");
    expect(query).not.toHaveProperty("sort", expect.anything());
  });

  it("sends a chosen sort on a series page", async () => {
    renderAt(
      SeriesDetail,
      "/libraries/:libraryId/series/:id",
      "/libraries/1/series/5?sort=date_released:asc",
    );
    expect(await booksQuery("/series/5/books")).toEqual(
      expect.objectContaining({ sort: "date_released:asc" }),
    );
  });
});
