import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeAll, describe, expect, it, vi } from "vitest";

import GenreDetail from "./GenreDetail";

vi.mock("@/hooks/queries/libraries", () => ({
  useLibrary: () => ({ data: { name: "My Library" } }),
}));

vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

vi.mock("@/components/library/BookGallerySection", () => ({
  BookGallerySection: () => <div>Books</div>,
}));

vi.mock("@/hooks/usePageTitle", () => ({
  usePageTitle: vi.fn(),
}));

vi.mock("@/hooks/queries/settings", () => ({
  useUserSettings: () => ({
    data: { gallery_size: "m" },
    isSuccess: true,
    isError: false,
  }),
}));

const allGenres = [
  { id: 5, name: "Science Fiction", book_count: 3 },
  { id: 10, name: "Dystopian", book_count: 5 },
  { id: 20, name: "Horror", book_count: 2 },
];

vi.mock("@/hooks/queries/genres", () => ({
  useGenre: () => ({
    data: {
      aliases: [],
      book_count: 3,
      id: 5,
      library_id: 1,
      name: "Science Fiction",
    },
    isLoading: false,
    isSuccess: true,
  }),
  useGenreBooks: () => ({
    data: { items: [], total: 0 },
    dataUpdatedAt: 0,
    isLoading: false,
    isSuccess: true,
  }),
  useGenresList: ({ search }: { search?: string }) => ({
    data: {
      items: allGenres.filter((genre) =>
        search ? genre.name.toLowerCase().includes(search.toLowerCase()) : true,
      ),
    },
    isLoading: false,
  }),
  useUpdateGenre: () => ({ isPending: false, mutateAsync: vi.fn() }),
  useMergeGenre: () => ({ isPending: false, mutateAsync: vi.fn() }),
  useDeleteGenre: () => ({ isPending: false, mutateAsync: vi.fn() }),
}));

beforeAll(() => {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });
});

function renderAt(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route
            element={<GenreDetail />}
            path="/libraries/:libraryId/genres/:id"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("GenreDetail", () => {
  it("shows an unfiltered merge list immediately after a fast close and reopen", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    renderAt("/libraries/1/genres/5");

    await user.click(screen.getByRole("button", { name: /^merge$/i }));
    await user.click(screen.getByRole("combobox"));
    await user.type(screen.getByPlaceholderText("Search genres..."), "Dys");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(200);
    });

    expect(screen.getByText("Dystopian")).toBeInTheDocument();
    expect(screen.queryByText("Horror")).not.toBeInTheDocument();

    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    await user.click(screen.getByRole("button", { name: /^merge$/i }));
    await user.click(screen.getByRole("combobox"));

    expect(screen.getByText("Dystopian")).toBeInTheDocument();
    expect(screen.getByText("Horror")).toBeInTheDocument();
  });
});
