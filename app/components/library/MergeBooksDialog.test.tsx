import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Book, LibrarySummary } from "@/types";

import { MergeBooksDialog } from "./MergeBooksDialog";

const makeBook = (id: number, title: string) =>
  ({ id, title, files: [] }) as unknown as Book;

// The books the dialog's queries have loaded.
let loadedBooks: Book[] = [];

vi.mock("@/hooks/queries/books", () => ({
  useBooksByIds: () =>
    loadedBooks.map((data) => ({
      data,
      error: null,
      isLoading: false,
      isFetching: false,
    })),
  useMergeBooks: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

const library = {
  id: 1,
  name: "Books",
  organize_file_structure: false,
} as LibrarySummary;

const renderDialog = () => (
  <MergeBooksDialog
    bookIds={[1, 2]}
    library={library}
    onOpenChange={vi.fn()}
    open
  />
);

describe("MergeBooksDialog Continue button", () => {
  beforeEach(() => {
    loadedBooks = [];
  });

  it("is disabled until a target book is selected", () => {
    const { rerender } = render(renderDialog());
    expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();

    // Loading the books selects the first one as the target.
    loadedBooks = [makeBook(1, "First"), makeBook(2, "Second")];
    rerender(renderDialog());

    expect(screen.getByRole("radio", { name: /First/ })).toBeChecked();
    expect(screen.getByRole("button", { name: "Continue" })).toBeEnabled();
  });
});
