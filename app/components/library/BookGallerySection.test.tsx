import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeAll, describe, expect, it, vi } from "vitest";

import type { Book, ResourceListResponse } from "@/types";

import { BookGallerySection } from "./BookGallerySection";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";

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

// Mock mutation hooks — they require a running API
vi.mock("@/hooks/queries/books", () => ({
  useDeleteBook: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useResyncBook: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

vi.mock("@/hooks/queries/settings", () => ({
  useUserSettings: () => ({ data: { gallery_size: "m" }, isSuccess: true }),
  useUpdateUserSettings: () => ({ mutate: vi.fn(), isPending: false }),
}));

function wrap(ui: React.ReactNode, initialEntries = ["/"]) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={initialEntries}>{ui}</MemoryRouter>
    </QueryClientProvider>
  );
}

function makeBook(id: number): Book {
  return {
    id,
    title: `Book ${id}`,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    library_id: 1,
  } as Book;
}

function makeQueryResult(
  books: Book[],
  total: number,
): {
  data: ResourceListResponse<Book>;
  isSuccess: boolean;
  isError: boolean;
} {
  return {
    data: { items: books, total },
    isSuccess: true,
    isError: false,
  };
}

describe("BookGallerySection", () => {
  it("renders book items from query data", () => {
    const books = [makeBook(1), makeBook(2), makeBook(3)];
    render(
      wrap(
        <BookGallerySection
          libraryId="1"
          query={makeQueryResult(books, 3)}
          title="Books"
        />,
      ),
    );
    expect(screen.getByText("Book 1")).toBeInTheDocument();
    expect(screen.getByText("Book 2")).toBeInTheDocument();
    expect(screen.getByText("Book 3")).toBeInTheDocument();
  });

  it("renders section title", () => {
    const books = [makeBook(1)];
    render(
      wrap(
        <BookGallerySection
          libraryId="1"
          query={makeQueryResult(books, 1)}
          title="Books"
        />,
      ),
    );
    expect(
      screen.getByRole("heading", { level: 2, name: "Books" }),
    ).toBeInTheDocument();
  });

  it("renders empty state with section title when no books", () => {
    render(
      wrap(
        <BookGallerySection
          emptyMessage="No books here."
          libraryId="1"
          query={makeQueryResult([], 0)}
          title="Books"
        />,
      ),
    );
    expect(
      screen.getByRole("heading", { level: 2, name: "Books" }),
    ).toBeInTheDocument();
    expect(screen.getByText("No books here.")).toBeInTheDocument();
  });

  it("renders the Size button", () => {
    const books = [makeBook(1)];
    render(
      wrap(
        <BookGallerySection
          libraryId="1"
          query={makeQueryResult(books, 1)}
          title="Books"
        />,
      ),
    );
    // SizeButton renders as a button with "Size" text
    expect(screen.getByRole("button", { name: /Size/ })).toBeInTheDocument();
  });

  it("calls onPageChange with page 1 when size changes from the first page", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onPageChange = vi.fn();
    const books = Array.from({ length: 24 }, (_, i) => makeBook(i + 1));
    render(
      wrap(
        <BookGallerySection
          libraryId="1"
          onPageChange={onPageChange}
          onSizeChange={vi.fn()}
          query={makeQueryResult(books, 50)}
          title="Books"
        />,
      ),
    );

    // Open the SizePopover first, then click a different size
    await user.click(screen.getByRole("button", { name: /Size/ }));
    await user.click(screen.getByRole("button", { name: "L" }));
    expect(onPageChange).toHaveBeenCalledWith(1);
  });

  it("keeps the first visible item in view when size changes on a later page", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onPageChange = vi.fn();
    const books = Array.from({ length: 24 }, (_, i) => makeBook(i + 1));
    render(
      wrap(
        <BookGallerySection
          libraryId="1"
          onPageChange={onPageChange}
          onSizeChange={vi.fn()}
          query={makeQueryResult(books, 100)}
          title="Books"
        />,
        ["/?page=3"],
      ),
    );

    // Page 3 at size M (24/page) starts at offset 48; at size S (33/page)
    // that item lives on page 2, not page 1.
    await user.click(screen.getByRole("button", { name: /Size/ }));
    await user.click(screen.getByRole("button", { name: "S" }));
    expect(onPageChange).toHaveBeenCalledWith(2);
  });

  it("renders loading spinner when loading", () => {
    render(
      wrap(
        <BookGallerySection
          libraryId="1"
          query={{
            data: undefined,
            isSuccess: false,
            isError: false,
          }}
          title="Books"
        />,
      ),
    );
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  it("renders loading spinner, not the empty state, while the query is disabled", () => {
    // A disabled query (waiting on its `enabled` gate, e.g. user settings)
    // is pending but not fetching: data undefined, isSuccess and isError
    // both false. It must not flash the empty state.
    render(
      wrap(
        <BookGallerySection
          emptyMessage="No books here."
          libraryId="1"
          query={{
            data: undefined,
            isSuccess: false,
            isError: false,
          }}
          title="Books"
        />,
      ),
    );
    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(screen.queryByText("No books here.")).not.toBeInTheDocument();
  });

  it("renders empty state when the query errors", () => {
    render(
      wrap(
        <BookGallerySection
          emptyMessage="No books here."
          libraryId="1"
          query={{
            data: undefined,
            isSuccess: false,
            isError: true,
          }}
          title="Books"
        />,
      ),
    );
    expect(screen.getByText("No books here.")).toBeInTheDocument();
  });
});
