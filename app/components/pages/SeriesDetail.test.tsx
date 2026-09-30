import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ShishoAPIError } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { Permission } from "@/utils/permissions";

import SeriesDetail from "./SeriesDetail";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

// Series mutations require Series Write; Books Write alone must not unlock
// them.
const READS: Permission[] = ["books:read", "series:read", "people:read"];

beforeEach(() =>
  setAuth({
    permissions: [...READS, "books:write", "series:write", "people:write"],
  }),
);

// A mutation (delete, merge, or edit) that succeeds the way TanStack Query
// reports it: the call's own onSuccess runs before the promise resolves.
const succeedingMutation = vi.hoisted(() => ({
  isPending: false,
  mutate: () => undefined,
  mutateAsync: (_vars: unknown, options?: { onSuccess?: () => void }) => {
    options?.onSuccess?.();
    return Promise.resolve();
  },
}));

// Each test starts from a loaded series; the load-failure tests replace it.
const seriesQuery = vi.hoisted(() => ({
  current: {} as Record<string, unknown>,
}));
const failedSeries = (error: unknown) => ({
  data: undefined,
  error,
  isLoading: false,
  isSuccess: false,
  isError: true,
  isFetching: false,
  isEnabled: true,
  refetch: vi.fn(),
});
beforeEach(() => {
  seriesQuery.current = {
    data: {
      id: 3,
      library_id: 1,
      name: "Discworld",
      sort_name: "Discworld",
      book_count: 0,
      aliases: [],
    },
    error: null,
    isLoading: false,
    isSuccess: true,
    isFetching: false,
    isEnabled: true,
    refetch: vi.fn(),
  };
});

vi.mock("@/hooks/queries/series", () => ({
  useSeries: () => seriesQuery.current,
  useSeriesBooks: () => ({ data: { items: [], total: 0 }, isLoading: false }),
  useSeriesList: () => ({
    data: { items: [{ id: 9, name: "Rincewind", book_count: 2 }] },
    isLoading: false,
  }),
  useUpdateSeries: () => succeedingMutation,
  useMergeSeries: () => succeedingMutation,
  useDeleteSeries: () => succeedingMutation,
}));
vi.mock("@/hooks/queries/libraries", () => ({
  useUserLibrary: () => ({ data: { id: 1, name: "Lib" } }),
}));
vi.mock("@/hooks/queries/settings", () => ({
  useUserSettings: () => ({
    data: { gallery_size: "medium" },
    isSuccess: true,
    isError: false,
  }),
  useUpdateUserSettings: () => ({ mutate: vi.fn(), isPending: false }),
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
          <Route
            element={<h1>Series list</h1>}
            path="/libraries/:libraryId/series"
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
    setAuth({ permissions: [...READS, "books:write", "people:write"] });
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

describe("SeriesDetail load failure", () => {
  it("shows the fallback and Retry inside the page for a server fault", async () => {
    seriesQuery.current = failedSeries(
      new ShishoAPIError("Internal Server Error", "internal_server_error", 500),
    );
    renderPage();

    expect(screen.getByRole("alert")).toHaveTextContent(
      /^Failed to load series/,
    );
    expect(screen.queryByText(/Not Found/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Internal Server Error/)).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(seriesQuery.current.refetch).toHaveBeenCalledTimes(1);
  });

  it("keeps the Not Found page for a 404", () => {
    seriesQuery.current = failedSeries(
      new ShishoAPIError("Series not found", "not_found", 404),
    );
    renderPage();

    expect(
      screen.getByRole("heading", { name: "Series Not Found" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

describe("SeriesDetail delete", () => {
  it("returns to the series list after a successful delete", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPage();

    await user.click(screen.getByRole("button", { name: /Delete/ }));
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(
      await screen.findByRole("heading", { name: "Series list" }),
    ).toBeInTheDocument();
  });
});

describe("SeriesDetail merge", () => {
  it("closes the merge dialog after a successful merge", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPage();

    await user.click(screen.getByRole("button", { name: /Merge/ }));
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("combobox"));
    await user.click(screen.getByText("Rincewind"));
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));

    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });
});

describe("SeriesDetail edit", () => {
  it("closes the edit dialog after a successful save", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPage();

    await user.click(screen.getByRole("button", { name: /Edit/ }));
    const dialog = screen.getByRole("dialog");
    const name = within(dialog).getByLabelText("Name");
    await user.clear(name);
    await user.type(name, "Discworld Novels");
    await user.click(within(dialog).getByRole("button", { name: /Save/ }));

    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });
});
