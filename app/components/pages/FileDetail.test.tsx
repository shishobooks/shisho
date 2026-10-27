import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ChaptersActionState } from "@/components/files/FileChaptersEditor";
import { useBook } from "@/hooks/queries/books";
import { setAuth } from "@/testing/auth";
import { mockCoverDimensions } from "@/testing/coverDimensions";
import type { Book, File } from "@/types";

import FileDetail from "./FileDetail";

mockCoverDimensions();

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/hooks/queries/books", async () => {
  const actual = await vi.importActual<typeof import("@/hooks/queries/books")>(
    "@/hooks/queries/books",
  );
  return { ...actual, useBook: vi.fn() };
});
vi.mock("@/hooks/queries/libraries", async () => {
  const actual = await vi.importActual<
    typeof import("@/hooks/queries/libraries")
  >("@/hooks/queries/libraries");
  return {
    ...actual,
    useUserLibrary: () => ({ data: { id: 1, name: "Lib" } }),
  };
});
vi.mock("@/hooks/queries/delivery", () => ({
  useDeliveryRules: () => ({ data: { rules: [] }, isPending: false }),
}));
vi.mock("@/hooks/queries/plugins", () => ({
  usePluginIdentifierTypes: () => ({ data: [] }),
}));
vi.mock("@/hooks/queries/review", () => ({
  useReviewCriteria: () => ({ data: { book_fields: [], audio_fields: [] } }),
}));
vi.mock("@/hooks/queries/chapters", () => ({
  useFileChapters: () => ({ data: chapters.list }),
}));

// The app shell needs an AuthProvider and the dialogs their own queries;
// none of that is what these tests are about.
vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
vi.mock("@/components/library/LibraryBreadcrumbs", () => ({
  default: () => <nav />,
}));
vi.mock("@/components/library/FileEditDialog", () => ({
  FileEditDialog: () => <div role="dialog">Edit file</div>,
}));
vi.mock("@/components/library/BookEditDialog", () => ({
  BookEditDialog: ({ open }: { open: boolean }) =>
    open ? <div role="dialog">Edit book</div> : null,
}));
vi.mock("@/components/library/MoveFilesDialog", () => ({
  MoveFilesDialog: () => null,
}));

// FileChaptersSection.test.tsx covers the chapter bar; here the editor only
// reports what the real one would, so the page's guard can be checked.
const chapters = vi.hoisted(() => ({
  list: [] as unknown[],
  report: undefined as undefined | ((state: ChaptersActionState) => void),
}));
vi.mock("@/components/files/FileChaptersEditor", () => ({
  default: ({
    onActionStateChange,
  }: {
    onActionStateChange?: (state: ChaptersActionState) => void;
  }) => {
    chapters.report = onActionStateChange;
    return <div>chapter list</div>;
  },
}));

const file = (overrides: Partial<File> = {}): File =>
  ({
    id: 42,
    book_id: 7,
    library_id: 1,
    file_type: "epub",
    file_role: "main",
    filepath: "/lib/Ada Author/Book/book.epub",
    filesize_bytes: 1000,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    display_name: "Book",
    reachable_file_types: ["epub"],
    is_preferred_cover: false,
    reviewed: true,
    ...overrides,
  }) as File;

const book = (files: File[]): Book =>
  ({
    id: 7,
    library_id: 1,
    title: "Book",
    authors: [{ id: 1, person_id: 10, person: { id: 10, name: "Ada Author" } }],
    files,
  }) as unknown as Book;

const renderPage = (
  files: File[] = [file()],
  path = "/libraries/1/books/7/files/42",
) => {
  vi.mocked(useBook).mockReturnValue({
    data: book(files),
    isLoading: false,
    isSuccess: true,
  } as never);
  const router = createMemoryRouter(
    [
      {
        path: "/libraries/:libraryId/books/:bookId/files/:fileId/:tab?",
        element: <FileDetail />,
      },
      { path: "/libraries/:libraryId/books/:bookId", element: <p>Book</p> },
    ],
    { initialEntries: [path] },
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
};

const user = () => userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

beforeEach(() => {
  setAuth({ permissions: ["books:read", "books:write", "people:read"] });
  chapters.list = [];
});

describe("FileDetail header", () => {
  it("names the file and links back to its book and author", () => {
    renderPage();

    expect(screen.getByRole("heading", { name: "Book" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Book" })).toHaveAttribute(
      "href",
      "/libraries/1/books/7",
    );
    expect(screen.getByRole("link", { name: "Ada Author" })).toHaveAttribute(
      "href",
      "/libraries/1/people/10",
    );
  });

  it("calls a supplement a supplement to its book", () => {
    renderPage([
      file({ id: 41 }),
      file({ file_role: "supplement", file_type: "pdf", display_name: "Map" }),
    ]);

    expect(screen.getByText("Supplement to")).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Ada Author" }),
    ).not.toBeInTheDocument();
  });

  it("shows a Listen action for an audiobook and Read for an ebook", () => {
    renderPage([file({ file_type: "m4b" })]);
    expect(screen.getByRole("link", { name: "Listen" })).toHaveAttribute(
      "href",
      "/libraries/1/books/7/files/42/read",
    );
  });

  it("puts the file's on-disk facts in their own section", () => {
    renderPage();

    const disk = screen.getByRole("region", { name: "On disk" });
    expect(within(disk).getByText("book.epub")).toBeInTheDocument();
    expect(within(disk).getByText("/lib/Ada Author/Book")).toBeInTheDocument();
  });
});

describe("FileDetail actions", () => {
  it("offers Edit and the file actions for Books Write", async () => {
    renderPage();

    expect(screen.getByRole("button", { name: "Edit" })).toBeInTheDocument();
    await user().click(screen.getByRole("button", { name: "File actions" }));
    for (const name of ["Rescan file", "Move to another book", "Delete file"]) {
      expect(screen.getByRole("menuitem", { name })).toBeInTheDocument();
    }
    // The page lists everything the File details dialog would.
    expect(
      screen.queryByRole("menuitem", { name: "File details" }),
    ).not.toBeInTheDocument();
  });

  it("keeps a supplement with its book", async () => {
    renderPage([
      file({ id: 41 }),
      file({ file_role: "supplement", file_type: "pdf" }),
    ]);

    await user().click(screen.getByRole("button", { name: "File actions" }));
    expect(
      screen.queryByRole("menuitem", { name: "Move to another book" }),
    ).not.toBeInTheDocument();
  });

  it("hides every write control for a read-only role", () => {
    setAuth({ permissions: ["books:read"] });
    renderPage([file({ file_type: "m4b" })]);

    expect(screen.getByRole("link", { name: "Listen" })).toBeInTheDocument();
    for (const name of [
      "Edit",
      "File actions",
      "Edit chapters",
      "Fetch from Audible",
    ]) {
      expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    }
  });
});

describe("FileDetail review badge", () => {
  it("speaks for this file and opens its editor", async () => {
    renderPage([file({ reviewed: false })]);

    await user().click(screen.getByRole("button", { name: "Needs review" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("Edit file");
  });

  it("does not show for a reviewed file beside an unreviewed one", () => {
    renderPage([file(), file({ id: 43, reviewed: false })]);

    expect(
      screen.queryByRole("button", { name: "Needs review" }),
    ).not.toBeInTheDocument();
  });
});

describe("FileDetail files list", () => {
  it("lists the book's files, this one marked instead of linked", () => {
    renderPage([
      file(),
      file({ id: 43, file_type: "m4b", display_name: "Audiobook" }),
    ]);

    // One list beside the header from 1280px, one below it otherwise.
    const [list] = screen.getAllByRole("region", {
      name: "2 files in this book",
    });
    expect(within(list).getByText("EPUB · This file")).toBeInTheDocument();
    expect(
      within(list).getByRole("link", { name: /Audiobook/ }),
    ).toHaveAttribute("href", "/libraries/1/books/7/files/43");
  });

  it("leaves the list out for a book's only file", () => {
    renderPage();

    expect(
      screen.queryByRole("region", { name: /in this book/ }),
    ).not.toBeInTheDocument();
  });
});

describe("FileDetail old tab URLs", () => {
  it("sends the old Chapters tab to the page", async () => {
    const router = renderPage(
      undefined,
      "/libraries/1/books/7/files/42/chapters",
    );

    await vi.waitFor(() =>
      expect(router.state.location.pathname).toBe(
        "/libraries/1/books/7/files/42",
      ),
    );
    expect(router.state.historyAction).toBe("REPLACE");
  });
});

describe("FileDetail unsaved chapter edits", () => {
  it("asks before leaving with unsaved chapter edits", async () => {
    const router = renderPage();

    act(() =>
      chapters.report?.({ isSaving: false, canSave: true, hasChanges: true }),
    );
    await user().click(screen.getByRole("link", { name: "Book" }));

    expect(
      screen.getByRole("dialog", { name: "Unsaved Changes" }),
    ).toBeInTheDocument();
    expect(router.state.location.pathname).toBe(
      "/libraries/1/books/7/files/42",
    );
  });
});
