import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import SeriesDetail from "./SeriesDetail";

// Resources the signed-in user may write. Series mutations require Series
// Write; Books Write alone must not unlock them.
let writableResources: string[] = ["books", "series", "people"];

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    canWrite: (resource: string) => writableResources.includes(resource),
  }),
}));

beforeEach(() => {
  writableResources = ["books", "series", "people"];
});

const { idle } = vi.hoisted(() => ({
  idle: () => ({ mutateAsync: vi.fn(), mutate: vi.fn(), isPending: false }),
}));

vi.mock("@/hooks/queries/series", () => ({
  useSeries: () => ({
    data: {
      id: 3,
      library_id: 1,
      name: "Discworld",
      sort_name: "Discworld",
      book_count: 0,
      aliases: [],
    },
    isLoading: false,
    isSuccess: true,
  }),
  useSeriesBooks: () => ({ data: { items: [], total: 0 }, isLoading: false }),
  useSeriesList: () => ({ data: { items: [] }, isLoading: false }),
  useUpdateSeries: idle,
  useMergeSeries: idle,
  useDeleteSeries: idle,
}));
vi.mock("@/hooks/queries/libraries", () => ({
  useLibrary: () => ({ data: { id: 1, name: "Lib" } }),
}));
vi.mock("@/hooks/queries/settings", () => ({
  useUserSettings: () => ({
    data: { gallery_size: "medium" },
    isSuccess: true,
    isError: false,
  }),
}));
vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
vi.mock("@/components/library/LibraryBreadcrumbs", () => ({
  default: () => <nav />,
}));
vi.mock("@/components/library/BookGallerySection", () => ({
  BookGallerySection: () => <div>books</div>,
}));

const renderPage = () =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={["/libraries/1/series/3"]}>
        <Routes>
          <Route
            element={<SeriesDetail />}
            path="/libraries/:libraryId/series/:id"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("SeriesDetail write controls", () => {
  it("shows Edit, Merge, and Delete with Series Write", () => {
    renderPage();
    expect(screen.getByRole("button", { name: /Edit/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Merge/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Delete/ })).toBeInTheDocument();
  });

  it("hides them without Series Write, even when the user has Books Write", () => {
    writableResources = ["books", "people"];
    renderPage();
    expect(
      screen.getByRole("heading", { level: 1, name: "Discworld" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Edit/ }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Merge/ }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Delete/ }),
    ).not.toBeInTheDocument();
  });
});
