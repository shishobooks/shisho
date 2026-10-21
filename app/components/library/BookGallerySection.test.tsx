import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  createMemoryRouter,
  MemoryRouter,
  RouterProvider,
  useLocation,
} from "react-router-dom";
import { toast } from "sonner";
import { beforeAll, describe, expect, it, vi } from "vitest";

import { QueryKey as LibrariesQueryKey } from "@/hooks/queries/libraries";
import { useSortParam } from "@/hooks/useSortParam";
import { ShishoAPIError } from "@/libraries/api";
import {
  BOOK_DETAIL_SORT,
  SERIES_BOOKS_SORT,
  type SortConfig,
} from "@/libraries/sortSpec";
import { ALL_PERMISSIONS, setAuth } from "@/testing/auth";
import { rejectingMutate, REJECTION_MESSAGE } from "@/testing/mutations";
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
vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

setAuth({ permissions: ALL_PERMISSIONS });

vi.mock("@/hooks/queries/books", () => ({
  useDeleteBook: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useResyncBook: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

const settings = vi.hoisted(() => ({
  mutate: undefined as undefined | ((...args: never[]) => void),
}));

vi.mock("@/hooks/queries/settings", () => ({
  useUserSettings: () => ({ data: { gallery_size: "m" }, isSuccess: true }),
  useUpdateUserSettings: () => ({
    mutate: settings.mutate ?? vi.fn(),
    isPending: false,
  }),
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

const QUERY_STATE = {
  error: null,
  isFetching: false,
  isEnabled: true,
  isPlaceholderData: false,
  refetch: () => undefined,
};

function makeQueryResult(books: Book[], total: number) {
  return {
    ...QUERY_STATE,
    data: { items: books, total } as ResourceListResponse<Book>,
    isSuccess: true,
    isError: false,
  };
}

describe("BookGallerySection", () => {
  it("toasts when saving the size as the default fails", async () => {
    const error = vi.spyOn(toast, "error");
    settings.mutate = rejectingMutate();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(
      wrap(
        <BookGallerySection
          libraryId="1"
          query={makeQueryResult([makeBook(1)], 1)}
          title="Books"
        />,
        ["/?size=l"],
      ),
    );

    await user.click(screen.getByRole("button", { name: /Size/ }));
    await user.click(
      screen.getByRole("button", { name: /save as my default everywhere/i }),
    );

    expect(error).toHaveBeenCalledWith(REJECTION_MESSAGE, undefined);
    settings.mutate = undefined;
    error.mockRestore();
  });

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

  it("replaces a page past the end with the last page", async () => {
    const router = createMemoryRouter(
      [
        {
          path: "/",
          element: (
            <BookGallerySection
              libraryId="1"
              query={makeQueryResult([], 100)}
              title="Books"
            />
          ),
        },
      ],
      { initialEntries: ["/?start=999"] },
    );
    render(
      <QueryClientProvider client={new QueryClient()}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );

    // 100 books at size M (24 per page) is five pages; the last starts at 96.
    await waitFor(() => expect(router.state.location.search).toBe("?start=96"));
    expect(router.state.historyAction).toBe("REPLACE");
  });

  it("keeps the page while the total is another query's placeholder", async () => {
    const router = createMemoryRouter(
      [
        {
          path: "/",
          element: (
            <BookGallerySection
              libraryId="1"
              query={{ ...makeQueryResult([], 10), isPlaceholderData: true }}
              title="Books"
            />
          ),
        },
      ],
      { initialEntries: ["/?start=72"] },
    );
    render(
      <QueryClientProvider client={new QueryClient()}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );

    await screen.findByText("Books");
    expect(router.state.location.search).toBe("?start=72");
  });

  it("keeps the first book of the page in the URL when the size changes", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    // The page math only reads `total`, so one card is enough. Rendering a
    // full page of cards made these clicks slow enough to time out when the
    // whole check suite runs in parallel.
    const books = [makeBook(1)];
    const router = createMemoryRouter(
      [
        {
          path: "/",
          element: (
            <BookGallerySection
              libraryId="1"
              query={makeQueryResult(books, 100)}
              title="Books"
            />
          ),
        },
      ],
      { initialEntries: ["/?start=48"] },
    );
    render(
      <QueryClientProvider client={new QueryClient()}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );

    await user.click(screen.getByRole("button", { name: /Size/ }));
    await user.click(screen.getByRole("button", { name: "S" }));

    // The URL keeps naming book 49; at S (33 per page) it is on the page
    // starting at 33, which useGallerySizeParam.test.tsx covers.
    const params = new URLSearchParams(router.state.location.search);
    expect(params.get("size")).toBe("s");
    expect(params.get("start")).toBe("48");
  });

  it("renders loading spinner when loading", () => {
    render(
      wrap(
        <BookGallerySection
          libraryId="1"
          query={{
            ...QUERY_STATE,
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
            ...QUERY_STATE,
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

  it("reports a failed query under the heading with Retry", async () => {
    const refetch = vi.fn();
    render(
      wrap(
        <BookGallerySection
          emptyMessage="No books here."
          libraryId="1"
          query={{
            ...QUERY_STATE,
            data: undefined,
            error: new ShishoAPIError(
              "Internal Server Error",
              "internal_server_error",
              500,
            ),
            isSuccess: false,
            isError: true,
            refetch,
          }}
          title="Books"
        />,
      ),
    );
    expect(screen.getByRole("heading", { name: "Books" })).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent(
      /^Failed to load books/,
    );
    expect(screen.queryByText("No books here.")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetch).toHaveBeenCalledTimes(1);
  });

  describe("sort", () => {
    const Sorted = <F extends string>({
      config,
    }: {
      config: SortConfig<F>;
    }) => {
      const sort = useSortParam(config);
      return (
        <>
          <BookGallerySection
            libraryId="1"
            query={makeQueryResult([makeBook(1)], 1)}
            sort={sort}
            title="Books"
          />
          <output data-testid="location">{useLocation().search}</output>
        </>
      );
    };

    it("has no Sort control without a sort", () => {
      render(
        wrap(
          <BookGallerySection
            libraryId="1"
            query={makeQueryResult([makeBook(1)], 1)}
            title="Books"
          />,
        ),
      );
      expect(
        screen.queryByRole("button", { name: /^Sort/ }),
      ).not.toBeInTheDocument();
    });

    it("writes a chosen field to the URL and shows it as a chip", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      render(wrap(<Sorted config={BOOK_DETAIL_SORT} />));
      expect(screen.queryByText("Sorted by:")).not.toBeInTheDocument();

      await user.click(screen.getByRole("button", { name: /^Sort/ }));
      await user.click(screen.getByRole("button", { name: /Page count/ }));

      expect(screen.getByTestId("location").textContent).toContain(
        "sort=date_released%3Adesc%2Ctitle%3Aasc%2Cpage_count%3Aasc",
      );
      expect(screen.getByText("Sorted by:")).toBeInTheDocument();
    });

    it("names the endpoint's own order when the default has no levels", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      render(wrap(<Sorted config={SERIES_BOOKS_SORT} />));

      await user.click(screen.getByRole("button", { name: /^Sort/ }));
      expect(screen.getByText("Sorted by series order.")).toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /Series$/ }),
      ).not.toBeInTheDocument();
    });
  });

  it("shapes covers by the library's cover aspect ratio, like its gallery", () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    queryClient.setQueryData(
      [LibrariesQueryKey.UserLibraries],
      [{ id: 1, name: "Audio", cover_aspect_ratio: "audiobook" }],
    );
    const { container } = render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <BookGallerySection
            libraryId="1"
            query={makeQueryResult([makeBook(1)], 1)}
            title="Books"
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(container.querySelector(".aspect-square")).not.toBeNull();
    expect(container.querySelector(".aspect-\\[2\\/3\\]")).toBeNull();
  });
});
