import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, waitFor } from "@testing-library/react";
import type { ComponentType, ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { Permission } from "@/utils/permissions";

import GenreDetail from "./GenreDetail";
import PersonDetail from "./PersonDetail";
import PublisherDetail from "./PublisherDetail";
import SeriesDetail from "./SeriesDetail";
import TagDetail from "./TagDetail";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

// Each detail page prefetches its merge dialog's candidate list, which only a
// role that can merge needs.
const READ_ALL: Permission[] = ["books:read", "series:read", "people:read"];
const PAGES: Array<{
  name: string;
  Page: ComponentType;
  segment: string;
  listPath: string;
  write: Permission;
}> = [
  {
    name: "GenreDetail",
    Page: GenreDetail,
    segment: "genres",
    listPath: "/genres",
    write: "books:write",
  },
  {
    name: "TagDetail",
    Page: TagDetail,
    segment: "tags",
    listPath: "/tags",
    write: "books:write",
  },
  {
    name: "PersonDetail",
    Page: PersonDetail,
    segment: "people",
    listPath: "/people",
    write: "people:write",
  },
  {
    name: "PublisherDetail",
    Page: PublisherDetail,
    segment: "publishers",
    listPath: "/publishers",
    write: "books:write",
  },
  {
    name: "SeriesDetail",
    Page: SeriesDetail,
    segment: "series",
    listPath: "/series",
    write: "series:write",
  },
];

const stubRequests = () =>
  vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
    if (/^\/[a-z]+\/1$/.test(String(path))) {
      return { id: 1, name: "Entity", library_id: 1, aliases: [] };
    }
    return { items: [], total: 0 };
  });

const renderPage = (Page: ComponentType, segment: string) => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/libraries/1/${segment}/1`]}>
        <Routes>
          <Route
            element={<Page />}
            path={`/libraries/:libraryId/${segment}/:id`}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

describe("detail page merge candidates", () => {
  afterEach(() => vi.restoreAllMocks());

  it.each(PAGES)(
    "$name requests no merge candidates for a read-only role",
    async ({ Page, segment, listPath }) => {
      setAuth({ permissions: READ_ALL });
      const request = stubRequests();

      renderPage(Page, segment);
      // The entity loads before the candidate list would.
      await waitFor(() =>
        expect(request.mock.calls.map((call) => call[1])).toContain(
          `/${segment}/1`,
        ),
      );
      await new Promise((resolve) => setTimeout(resolve, 50));

      const paths = request.mock.calls.map((call) => call[1]);
      expect(paths).not.toContain(listPath);
    },
  );

  it.each(PAGES)(
    "$name requests the merge candidates for a role that can merge",
    async ({ Page, segment, listPath, write }) => {
      setAuth({ permissions: [...READ_ALL, write] });
      const request = stubRequests();

      renderPage(Page, segment);

      await waitFor(() =>
        expect(request.mock.calls.map((call) => call[1])).toContain(listPath),
      );
    },
  );
});
