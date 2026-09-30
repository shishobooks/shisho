import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentType, ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";

import GenreDetail from "./GenreDetail";
import PersonDetail from "./PersonDetail";
import PublisherDetail from "./PublisherDetail";
import TagDetail from "./TagDetail";

// The genre, tag, person, and publisher pages return their delete promise to
// MetadataDeleteDialog and navigate in the call's onSuccess. These check the
// navigation still happens on each page.

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

setAuth({ permissions: ["books:read", "books:write", "people:write"] });

const { idle, succeeding, loaded, emptyList } = vi.hoisted(() => {
  const entity = {
    id: 5,
    library_id: 1,
    name: "Entity",
    aliases: [],
    book_count: 0,
    file_count: 0,
    authored_book_count: 0,
    narrated_file_count: 0,
    children: [],
    ancestors: [],
    descendant_ids: [],
  };
  return {
    emptyList: () => ({ data: { items: [], total: 0 }, isSuccess: true }),
    idle: () => ({ mutateAsync: vi.fn(), mutate: vi.fn(), isPending: false }),
    // A mutation that succeeds the way TanStack Query reports it: the call's
    // own onSuccess runs before the promise resolves.
    succeeding: () => ({
      isPending: false,
      mutate: () => undefined,
      mutateAsync: (_vars: unknown, options?: { onSuccess?: () => void }) => {
        options?.onSuccess?.();
        return Promise.resolve();
      },
    }),
    loaded: () => ({
      data: entity,
      error: null,
      isLoading: false,
      isFetching: false,
      isEnabled: true,
      refetch: vi.fn(),
    }),
  };
});

vi.mock("@/hooks/queries/genres", () => ({
  useGenre: loaded,
  useGenreBooks: emptyList,
  useGenresList: emptyList,
  useUpdateGenre: idle,
  useMergeGenre: idle,
  useDeleteGenre: succeeding,
}));
vi.mock("@/hooks/queries/tags", () => ({
  useTag: loaded,
  useTagBooks: emptyList,
  useTagsList: emptyList,
  useUpdateTag: idle,
  useMergeTag: idle,
  useDeleteTag: succeeding,
}));
vi.mock("@/hooks/queries/people", () => ({
  usePerson: loaded,
  usePersonAuthoredBooks: emptyList,
  usePersonNarratedFiles: emptyList,
  usePeopleList: emptyList,
  useUpdatePerson: idle,
  useMergePerson: idle,
  useDeletePerson: succeeding,
}));
vi.mock("@/hooks/queries/publishers", () => ({
  usePublisher: loaded,
  usePublisherFiles: emptyList,
  usePublishersList: emptyList,
  useUpdatePublisher: idle,
  useMergePublisher: idle,
  useSetChildPublisher: idle,
  useDeletePublisher: succeeding,
}));
vi.mock("@/hooks/queries/entity-search", () => ({
  useParentPublisherSearch: () => ({ data: undefined, isLoading: false }),
}));
vi.mock("@/hooks/queries/libraries", () => ({
  useUserLibrary: () => ({ data: { id: 1, name: "Lib" } }),
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
  BookGallerySection: () => <div>books</div>,
}));
vi.mock("@/components/library/FileListSection", () => ({
  FILE_LIST_ITEMS_PER_PAGE: 50,
  FileListSection: () => <div>files</div>,
}));
vi.mock("@/components/library/PublisherEditDialog", () => ({
  PublisherEditDialog: () => null,
}));

describe.each<[string, ComponentType, string]>([
  ["genre", GenreDetail, "genres"],
  ["tag", TagDetail, "tags"],
  ["person", PersonDetail, "people"],
  ["publisher", PublisherDetail, "publishers"],
])("%s detail delete", (_, Page, segment) => {
  it("returns to the list after a successful delete", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={[`/libraries/1/${segment}/5`]}>
          <Routes>
            <Route
              element={<Page />}
              path={`/libraries/:libraryId/${segment}/:id`}
            />
            <Route
              element={<h1>List page</h1>}
              path={`/libraries/:libraryId/${segment}`}
            />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    await user.click(screen.getByRole("button", { name: /Delete/ }));
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(
      await screen.findByRole("heading", { name: "List page" }),
    ).toBeInTheDocument();
  });
});
