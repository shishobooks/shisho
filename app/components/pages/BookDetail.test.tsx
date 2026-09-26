import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import BookDetail from "./BookDetail";

beforeAll(() => {
  vi.stubGlobal("__APP_VERSION__", "test");
});

let canWriteBooks = true;

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    demoMode: false,
    canWrite: (resource: string) => resource === "books" && canWriteBooks,
  }),
}));

beforeEach(() => {
  canWriteBooks = true;
});

const { book } = vi.hoisted(() => {
  const files = [
    {
      id: 42,
      book_id: 7,
      library_id: 1,
      file_type: "epub",
      file_role: "main",
      filepath: "/lib/book/book.epub",
      filesize_bytes: 1000,
      reviewed: true,
      updated_at: "2024-01-01T00:00:00Z",
      created_at: "2024-01-01T00:00:00Z",
    },
    {
      id: 43,
      book_id: 7,
      library_id: 1,
      file_type: "cbz",
      file_role: "main",
      filepath: "/lib/book/book.cbz",
      filesize_bytes: 2000,
      reviewed: true,
      updated_at: "2024-01-01T00:00:00Z",
      created_at: "2024-01-01T00:00:00Z",
    },
  ];

  return {
    book: {
      id: 7,
      library_id: 1,
      title: "Test Book",
      filepath: "/lib/book",
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      files,
    },
  };
});

const { idle } = vi.hoisted(() => ({
  idle: () => ({ mutateAsync: vi.fn(), mutate: vi.fn(), isPending: false }),
}));

vi.mock("@/hooks/queries/books", () => ({
  useBook: () => ({ data: book, isLoading: false, isSuccess: true }),
  useDeleteBook: idle,
  useDeleteFile: idle,
  useResyncBook: idle,
  useResyncFile: idle,
}));
vi.mock("@/hooks/queries/libraries", () => ({
  useLibrary: () => ({ data: { id: 1, name: "Lib" } }),
}));
vi.mock("@/hooks/queries/plugins", () => ({
  usePluginIdentifierTypes: () => ({ data: [] }),
}));
vi.mock("@/hooks/queries/review", () => ({
  useSetBookReview: idle,
  useReviewCriteria: () => ({
    data: { book_fields: [], audio_fields: [] },
  }),
}));

// The app shell and the dialogs pull in their own providers and queries; the
// page's action controls are what this suite is about.
vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
vi.mock("@/components/library/LibraryBreadcrumbs", () => ({
  default: () => <nav />,
}));
vi.mock("@/components/library/AddToListPopover", () => ({
  default: ({ trigger }: { trigger: ReactNode }) => (
    <div data-testid="add-to-list">{trigger}</div>
  ),
}));
vi.mock("@/components/library/CoverGalleryTabs", () => ({
  default: () => <div>covers</div>,
}));
vi.mock("@/components/library/BookEditDialog", () => ({
  BookEditDialog: () => null,
}));
vi.mock("@/components/library/IdentifyBookDialog", () => ({
  IdentifyBookDialog: () => null,
}));
vi.mock("@/components/library/MergeIntoDialog", () => ({
  MergeIntoDialog: () => null,
}));
vi.mock("@/components/library/MoveFilesDialog", () => ({
  MoveFilesDialog: () => null,
}));
vi.mock("@/components/library/FileEditDialog", () => ({
  FileEditDialog: () => null,
}));

const renderPage = () =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={["/libraries/1/books/7"]}>
        <Routes>
          <Route
            element={<BookDetail />}
            path="/libraries/:libraryId/books/:id"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("BookDetail write controls", () => {
  it("shows the book menu, review toggle, file actions, and Select for Books Write", () => {
    renderPage();
    expect(screen.getByLabelText("Book actions")).toBeInTheDocument();
    expect(screen.getByRole("switch")).toBeInTheDocument();
    expect(screen.getAllByLabelText("File actions").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Select" })).toBeInTheDocument();
  });

  it("hides every Books Write control for a read-only user but keeps Add to list, reading, and downloads", () => {
    canWriteBooks = false;
    renderPage();

    expect(
      screen.getByRole("heading", { level: 1, name: "Test Book" }),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("Book actions")).not.toBeInTheDocument();
    expect(screen.getByTestId("add-to-list")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /add to list/i }),
    ).toBeInTheDocument();

    // Review state is visible but not editable.
    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
    expect(screen.getByText("Reviewed")).toBeInTheDocument();

    // Per-file mutation menus and the move/select flow are gone.
    expect(screen.queryByLabelText("File actions")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Select" }),
    ).not.toBeInTheDocument();

    // Read-only actions remain.
    const readLinks = screen
      .getAllByRole("link")
      .filter((link) => link.getAttribute("href")?.endsWith("/read"));
    expect(readLinks.length).toBeGreaterThan(0);
    expect(
      screen.getAllByRole("button", { name: "Download" }).length,
    ).toBeGreaterThan(0);
  });
});
