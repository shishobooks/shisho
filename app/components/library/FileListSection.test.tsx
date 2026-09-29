import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeAll, describe, expect, it, vi } from "vitest";

import { ShishoAPIError } from "@/libraries/api";
import type { File, ResourceListResponse } from "@/types";

import { FileListSection } from "./FileListSection";

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

function makeFile(id: number, overrides: Partial<File> = {}): File {
  return {
    id,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    library_id: 1,
    book_id: id * 10,
    filepath: `/library/book-${id}.epub`,
    file_type: "epub",
    file_role: "main",
    filesize_bytes: 1000,
    cover_image_filename: "cover.jpg",
    name: `Edition ${id}`,
    display_name: `Edition ${id}`,
    book: {
      id: id * 10,
      title: `Book Title ${id}`,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      library_id: 1,
    },
    ...overrides,
  } as File;
}

function makeQueryResult(files: File[], total: number) {
  return {
    data: { items: files, total } as ResourceListResponse<File> | undefined,
    error: null as unknown,
    isLoading: false,
    isSuccess: true,
    isFetching: false,
    isEnabled: true,
    refetch: vi.fn(),
  };
}

describe("FileListSection", () => {
  it("flags files whose last scan could not read them", () => {
    render(
      wrap(
        <FileListSection
          libraryId="1"
          query={makeQueryResult(
            [
              makeFile(1, { scan_error: "zip: not a valid zip file" }),
              makeFile(2),
            ],
            2,
          )}
          title="Files"
        />,
      ),
    );

    expect(screen.getAllByText("Unreadable")).toHaveLength(1);
  });

  it("keys each file cover on updated_at in epoch milliseconds", () => {
    const { container } = render(
      wrap(
        <FileListSection
          libraryId="1"
          query={makeQueryResult([makeFile(1)], 1)}
          title="Files"
        />,
      ),
    );

    expect(container.querySelector("img")?.getAttribute("src")).toBe(
      "/api/books/files/1/cover?v=1704067200000",
    );
  });

  it("renders section title", () => {
    const files = [makeFile(1)];
    render(
      wrap(
        <FileListSection
          libraryId="1"
          query={makeQueryResult(files, 1)}
          title="Files"
        />,
      ),
    );
    expect(
      screen.getByRole("heading", { level: 2, name: "Files" }),
    ).toBeInTheDocument();
  });

  it("shows author names as subtitle when authors are available", () => {
    const files = [
      makeFile(1, {
        book: {
          id: 10,
          title: "Book Title 1",
          created_at: "2024-01-01T00:00:00Z",
          updated_at: "2024-01-01T00:00:00Z",
          library_id: 1,
          authors: [
            {
              id: 1,
              book_id: 10,
              person_id: 1,
              person: {
                id: 1,
                created_at: "2024-01-01T00:00:00Z",
                updated_at: "2024-01-01T00:00:00Z",
                name: "Alice",
                sort_name: "Alice",
              },
              sort_order: 0,
            },
            {
              id: 2,
              book_id: 10,
              person_id: 2,
              person: {
                id: 2,
                created_at: "2024-01-01T00:00:00Z",
                updated_at: "2024-01-01T00:00:00Z",
                name: "Bob",
                sort_name: "Bob",
              },
              sort_order: 1,
            },
          ],
        } as File["book"],
      }),
    ];
    render(
      wrap(
        <FileListSection
          libraryId="1"
          query={makeQueryResult(files, 1)}
          title="Files"
        />,
      ),
    );
    expect(screen.getByText("Edition 1")).toBeInTheDocument();
    expect(screen.getByText("Alice, Bob")).toBeInTheDocument();
    expect(screen.queryByText("Book Title 1")).not.toBeInTheDocument();
  });

  it("falls back to book title when no authors are available", () => {
    const files = [makeFile(1), makeFile(2)];
    render(
      wrap(
        <FileListSection
          libraryId="1"
          query={makeQueryResult(files, 2)}
          title="Files"
        />,
      ),
    );
    expect(screen.getByText("Edition 1")).toBeInTheDocument();
    expect(screen.getByText("Edition 2")).toBeInTheDocument();
    expect(screen.getByText("Book Title 1")).toBeInTheDocument();
    expect(screen.getByText("Book Title 2")).toBeInTheDocument();
  });

  it("renders file type badge", () => {
    const files = [makeFile(1, { file_type: "epub" })];
    render(
      wrap(
        <FileListSection
          libraryId="1"
          query={makeQueryResult(files, 1)}
          title="Files"
        />,
      ),
    );
    expect(screen.getByText("EPUB")).toBeInTheDocument();
  });

  it("renders duration for audio files", () => {
    const files = [
      makeFile(1, {
        file_type: "m4b",
        audiobook_duration_seconds: 3661,
      }),
    ];
    render(
      wrap(
        <FileListSection
          libraryId="1"
          query={makeQueryResult(files, 1)}
          title="Files"
        />,
      ),
    );
    expect(screen.getByText("1h 1m")).toBeInTheDocument();
  });

  it("renders page count for CBZ files", () => {
    const files = [makeFile(1, { file_type: "cbz", page_count: 42 })];
    render(
      wrap(
        <FileListSection
          libraryId="1"
          query={makeQueryResult(files, 1)}
          title="Files"
        />,
      ),
    );
    expect(screen.getByText("42 pages")).toBeInTheDocument();
  });

  it("renders page count for PDF files", () => {
    const files = [makeFile(1, { file_type: "pdf", page_count: 100 })];
    render(
      wrap(
        <FileListSection
          libraryId="1"
          query={makeQueryResult(files, 1)}
          title="Files"
        />,
      ),
    );
    expect(screen.getByText("100 pages")).toBeInTheDocument();
  });

  it("renders empty state when no files", () => {
    render(
      wrap(
        <FileListSection
          emptyMessage="No files here."
          libraryId="1"
          query={makeQueryResult([], 0)}
          title="Files"
        />,
      ),
    );
    expect(screen.getByText("No files here.")).toBeInTheDocument();
  });

  it("renders loading spinner when loading", () => {
    render(
      wrap(
        <FileListSection
          libraryId="1"
          query={{
            ...makeQueryResult([], 0),
            isLoading: true,
            isSuccess: false,
          }}
          title="Files"
        />,
      ),
    );
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  it("renders showing count text", () => {
    const files = [makeFile(1), makeFile(2)];
    render(
      wrap(
        <FileListSection
          libraryId="1"
          query={makeQueryResult(files, 2)}
          title="Files"
        />,
      ),
    );
    expect(screen.getByText(/Showing 1-2 of 2 files/)).toBeInTheDocument();
  });

  it("renders each row as a link to the parent book", () => {
    const files = [makeFile(1)];
    render(
      wrap(
        <FileListSection
          libraryId="1"
          query={makeQueryResult(files, 1)}
          title="Files"
        />,
      ),
    );
    const links = screen.getAllByRole("link");
    const rowLink = links.find((link) =>
      link.getAttribute("href")?.includes("/books/10"),
    );
    expect(rowLink).toBeDefined();
  });

  it("shows the server's display name, the filename when the file has no name", () => {
    const files = [
      makeFile(1, {
        name: undefined,
        filepath: "/library/mybook.epub",
        display_name: "mybook.epub",
      }),
    ];
    render(
      wrap(
        <FileListSection
          libraryId="1"
          query={makeQueryResult(files, 1)}
          title="Files"
        />,
      ),
    );
    expect(screen.getByText("mybook.epub")).toBeInTheDocument();
  });

  it("reports a failed query under the heading with Retry", async () => {
    const query = {
      ...makeQueryResult([], 0),
      data: undefined,
      error: new ShishoAPIError(
        "Internal Server Error",
        "internal_server_error",
        500,
      ),
      isSuccess: false,
    };
    render(
      wrap(
        <FileListSection
          emptyMessage="No files here."
          libraryId="1"
          query={query}
          title="Files"
        />,
      ),
    );

    expect(screen.getByRole("heading", { name: "Files" })).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent(
      /^Failed to load files/,
    );
    expect(screen.queryByText("No files here.")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(query.refetch).toHaveBeenCalledTimes(1);
  });
});
