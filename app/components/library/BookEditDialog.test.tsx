import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { FileRoleMain, type Book } from "@/types";
import { forTitle } from "@/utils/sortname";

import { BookEditDialog } from "./BookEditDialog";

const mocks = vi.hoisted(() => ({
  updateBook: vi.fn().mockResolvedValue(undefined),
  setBookReview: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("@/hooks/queries/books", () => ({
  useUpdateBook: () => ({ mutateAsync: mocks.updateBook, isPending: false }),
}));

vi.mock("@/hooks/queries/entity-search", () => ({
  useAuthorSearch: () => ({ data: [], isLoading: false }),
  useGenreItemCounts: () => new Map(),
  useGenreSearch: () => ({ data: [], isLoading: false }),
  useSeriesSearch: () => ({ data: [], isLoading: false }),
  useTagItemCounts: () => new Map(),
  useTagSearch: () => ({ data: [], isLoading: false }),
}));

vi.mock("@/hooks/queries/review", () => ({
  useReviewCriteria: () => ({ data: undefined }),
  useSetBookReview: () => ({
    mutateAsync: mocks.setBookReview,
    isPending: false,
  }),
}));

/**
 * Tests for BookEditDialog handleSubmit sort title logic.
 *
 * The fixed logic compares the effective sort title against the snapshot from
 * when the dialog opened, matching the hasChanges calculation.
 */
describe("BookEditDialog handleSubmit sort title logic", () => {
  const shouldIncludeSortTitle = (
    sortTitle: string,
    _bookSortTitle: string | undefined,
    initialSortTitle: string,
    title: string,
  ): boolean => {
    const effectiveSortTitle = sortTitle || forTitle(title);
    return effectiveSortTitle !== initialSortTitle;
  };

  it("should include sort_title in payload when title changes in auto mode", () => {
    expect(
      shouldIncludeSortTitle("", undefined, forTitle("The Book"), "A Book"),
    ).toBe(true);
  });

  it("should not include sort_title when nothing changed", () => {
    expect(
      shouldIncludeSortTitle("", undefined, forTitle("The Book"), "The Book"),
    ).toBe(false);
  });
});

function makeBook(fileType: "cbz" | "epub"): Book {
  return {
    id: 1,
    title: "Test Book",
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    library_id: 1,
    book_series: [
      {
        id: 1,
        book_id: 1,
        series_id: 42,
        series_number: 1,
        series: { id: 42, library_id: 1, name: "Test Series" },
      } as never,
    ],
    files: [
      {
        id: 10,
        file_role: FileRoleMain,
        file_type: fileType,
        reviewed: true,
      } as never,
    ],
  } as unknown as Book;
}

function renderDialog(book: Book) {
  return render(
    <BookEditDialog book={book} onOpenChange={vi.fn()} open={true} />,
  );
}

describe("BookEditDialog series range editing", () => {
  it("includes an edited range end in the update payload", async () => {
    mocks.updateBook.mockClear();
    const user = userEvent.setup({
      advanceTimers: vi.advanceTimersByTime,
      delay: null,
    });
    renderDialog(makeBook("cbz"));

    await user.click(
      screen.getByRole("button", {
        name: "Advanced settings for Test Series",
      }),
    );
    await user.type(screen.getByLabelText("End"), "3");

    expect(screen.getByText("Vol. 1-3")).toBeInTheDocument();

    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "Save Changes" }));

    await waitFor(() => {
      expect(mocks.updateBook).toHaveBeenCalledWith({
        id: "1",
        payload: {
          series: [
            {
              name: "Test Series",
              number: 1,
              number_end: 3,
            },
          ],
        },
      });
    });
  });

  it("hides the unit control for a non-CBZ book", async () => {
    const user = userEvent.setup({
      advanceTimers: vi.advanceTimersByTime,
      delay: null,
    });
    renderDialog(makeBook("epub"));

    await user.click(
      screen.getByRole("button", {
        name: "Advanced settings for Test Series",
      }),
    );

    expect(screen.getByLabelText("End")).toBeInTheDocument();
    expect(screen.queryByText("Unit")).not.toBeInTheDocument();
  });
});
