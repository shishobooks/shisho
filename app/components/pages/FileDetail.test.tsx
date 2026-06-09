import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import { useBook, useDeleteFile } from "@/hooks/queries/books";
import { useLibrary } from "@/hooks/queries/libraries";

import FileDetail from "./FileDetail";

vi.mock("@/hooks/queries/books", async () => {
  const actual = await vi.importActual<typeof import("@/hooks/queries/books")>(
    "@/hooks/queries/books",
  );
  return { ...actual, useBook: vi.fn(), useDeleteFile: vi.fn() };
});
vi.mock("@/hooks/queries/libraries", async () => {
  const actual = await vi.importActual<
    typeof import("@/hooks/queries/libraries")
  >("@/hooks/queries/libraries");
  return { ...actual, useLibrary: vi.fn() };
});

// useUnsavedChanges calls react-router's useBlocker, which requires a data
// router. We don't exercise navigation blocking here, so stub it.
vi.mock("@/hooks/useUnsavedChanges", () => ({
  useUnsavedChanges: () => ({
    showBlockerDialog: false,
    proceedNavigation: vi.fn(),
    cancelNavigation: vi.fn(),
  }),
}));

// LibraryLayout renders the app shell (sidebar/top nav) which needs an
// AuthProvider. Stub it to a passthrough so we can render the page content
// without standing up the whole shell.
vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
vi.mock("@/components/library/LibraryBreadcrumbs", () => ({
  default: () => <nav />,
}));

// The tabs pull their own queries; we only care about the action buttons here.
vi.mock("@/components/files/FileDetailsTab", () => ({
  default: () => <div>details-tab</div>,
}));
vi.mock("@/components/files/FileChaptersTab", () => ({
  default: () => <div>chapters-tab</div>,
}));

const renderForFileType = (fileType: string) => {
  vi.mocked(useBook).mockReturnValue({
    data: {
      id: 7,
      title: "Book",
      files: [
        {
          id: 42,
          book_id: 7,
          file_type: fileType,
          filepath: "/lib/book." + fileType,
        },
      ],
    },
    isLoading: false,
    isSuccess: true,
  } as never);
  vi.mocked(useLibrary).mockReturnValue({ data: { name: "Lib" } } as never);
  vi.mocked(useDeleteFile).mockReturnValue({
    mutateAsync: vi.fn(),
    isPending: false,
  } as never);

  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={["/libraries/1/books/7/files/42"]}>
        <Routes>
          <Route
            element={<FileDetail />}
            path="/libraries/:libraryId/books/:bookId/files/:fileId/:tab?"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

describe("FileDetail reading action", () => {
  it("shows a Listen action for m4b files and not a Read action", () => {
    renderForFileType("m4b");
    const listen = screen.getByRole("link", { name: /listen/i });
    expect(listen).toBeInTheDocument();
    expect(listen).toHaveAttribute(
      "href",
      "/libraries/1/books/7/files/42/read",
    );
    expect(
      screen.queryByRole("link", { name: /read/i }),
    ).not.toBeInTheDocument();
  });

  it("shows a Read action for epub files and not a Listen action", () => {
    renderForFileType("epub");
    expect(screen.getByRole("link", { name: /read/i })).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: /listen/i }),
    ).not.toBeInTheDocument();
  });

  it("shows no reading action for unsupported file types", () => {
    renderForFileType("txt");
    expect(
      screen.queryByRole("link", { name: /listen/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: /read/i }),
    ).not.toBeInTheDocument();
  });
});
