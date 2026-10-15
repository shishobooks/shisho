import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";

import PersonDetail from "./PersonDetail";

// A person's page shows only the sections that have something in them.

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
setAuth({ permissions: ["books:read", "people:read"] });

const counts = vi.hoisted(() => ({
  authored: 0,
  narrated: 0,
  // What the list queries return, which can disagree with the stored counts.
  authoredList: { total: 0, failed: false },
  narratedList: { total: 0, failed: false },
}));

const listQuery = (list: { total: number; failed: boolean }) =>
  list.failed
    ? {
        data: undefined,
        error: new Error("boom"),
        isError: true,
        isSuccess: false,
      }
    : {
        data: { items: [], total: list.total },
        error: null,
        isError: false,
        isSuccess: true,
      };

vi.mock("@/hooks/queries/people", () => ({
  usePerson: () => ({
    data: {
      id: 5,
      library_id: 1,
      name: "Jane Doe",
      sort_name: "Doe, Jane",
      aliases: [],
      authored_book_count: counts.authored,
      narrated_file_count: counts.narrated,
    },
    error: null,
    isLoading: false,
    isFetching: false,
    isEnabled: true,
    refetch: vi.fn(),
  }),
  usePersonNarratedFiles: () => listQuery(counts.narratedList),
  usePeopleList: () => ({ data: { items: [], total: 0 } }),
  useUpdatePerson: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useMergePerson: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeletePerson: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));
vi.mock("@/hooks/queries/books", () => ({
  useBooks: () => listQuery(counts.authoredList),
}));
vi.mock("@/hooks/useGallerySizeParam", () => ({
  useGallerySizeParam: () => ({
    itemsPerPage: 24,
    offset: 0,
    settingsResolved: true,
  }),
}));
vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
vi.mock("@/components/library/LibraryBreadcrumbs", () => ({
  default: () => <nav />,
}));
vi.mock("@/components/library/BookGallerySection", () => ({
  BookGallerySection: ({ title }: { title: string }) => <h2>{title}</h2>,
}));
vi.mock("@/components/library/FileListSection", () => ({
  FILE_LIST_ITEMS_PER_PAGE: 50,
  FileListSection: ({ title }: { title: string }) => <h2>{title}</h2>,
}));

const NEITHER = "This person has no authored books or narrated files.";

const renderPerson = (
  authored: number,
  narrated: number,
  lists: {
    authored?: { total: number; failed: boolean };
    narrated?: { total: number; failed: boolean };
  } = {},
) => {
  counts.authored = authored;
  counts.narrated = narrated;
  counts.authoredList = lists.authored ?? { total: authored, failed: false };
  counts.narratedList = lists.narrated ?? { total: narrated, failed: false };
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={["/libraries/1/people/5"]}>
        <Routes>
          <Route
            element={<PersonDetail />}
            path="/libraries/:libraryId/people/:id"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

describe("PersonDetail sections", () => {
  it("shows both sections when the person authored and narrated", () => {
    renderPerson(2, 3);
    expect(screen.getByText("Books Authored")).toBeInTheDocument();
    expect(screen.getByText("Files Narrated")).toBeInTheDocument();
  });

  it("omits Files Narrated for an author who narrated nothing", () => {
    renderPerson(2, 0);
    expect(screen.getByText("Books Authored")).toBeInTheDocument();
    expect(screen.queryByText("Files Narrated")).not.toBeInTheDocument();
    expect(screen.queryByText(NEITHER)).not.toBeInTheDocument();
  });

  it("omits Books Authored for a narrator who authored nothing", () => {
    renderPerson(0, 3);
    expect(screen.queryByText("Books Authored")).not.toBeInTheDocument();
    expect(screen.getByText("Files Narrated")).toBeInTheDocument();
    expect(screen.queryByText(NEITHER)).not.toBeInTheDocument();
  });

  it("says so once when the person has neither", () => {
    renderPerson(0, 0);
    expect(screen.queryByText("Books Authored")).not.toBeInTheDocument();
    expect(screen.queryByText("Files Narrated")).not.toBeInTheDocument();
    expect(screen.getByText(NEITHER)).toBeInTheDocument();
  });

  // The person's counts refresh less often than the lists (a scan refreshes
  // the books list but not the person), so a list with items still shows.
  it("shows a section whose list has items even when the count is stale", () => {
    renderPerson(0, 0, { authored: { total: 2, failed: false } });
    expect(screen.getByText("Books Authored")).toBeInTheDocument();
    expect(screen.queryByText("Files Narrated")).not.toBeInTheDocument();
    expect(screen.queryByText(NEITHER)).not.toBeInTheDocument();
  });

  it("shows a section whose list failed so the error is reported", () => {
    renderPerson(2, 0, { narrated: { total: 0, failed: true } });
    expect(screen.getByText("Files Narrated")).toBeInTheDocument();
    expect(screen.queryByText(NEITHER)).not.toBeInTheDocument();
  });
});
