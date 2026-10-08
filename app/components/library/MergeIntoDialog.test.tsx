import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Book, LibrarySummary } from "@/types";

import { MergeIntoDialog } from "./MergeIntoDialog";

const makeBook = (id: number, title: string) =>
  ({ id, title, files: [] }) as unknown as Book;

const sourceBook = makeBook(1, "Source");

let mergePending = false;

vi.mock("@/hooks/queries/books", () => ({
  useBooks: () => ({
    data: { items: [sourceBook, makeBook(2, "Target")], total: 2 },
    error: null,
    isLoading: false,
    isFetching: false,
  }),
  useMergeBooks: () => ({ mutateAsync: vi.fn(), isPending: mergePending }),
}));

const library = {
  id: 1,
  name: "Books",
  organize_file_structure: false,
} as LibrarySummary;

const createUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

const renderDialog = () => (
  <MergeIntoDialog
    library={library}
    onOpenChange={vi.fn()}
    open
    sourceBook={sourceBook}
  />
);

const mergeButton = () => screen.getByRole("button", { name: "Merge" });

describe("MergeIntoDialog Merge button", () => {
  beforeEach(() => {
    mergePending = false;
  });

  it("is disabled until a target book is selected", async () => {
    const user = createUser();
    render(renderDialog());
    expect(mergeButton()).toBeDisabled();

    await user.click(screen.getByRole("radio", { name: /Target/ }));

    expect(mergeButton()).toBeEnabled();
  });

  it("is disabled while the merge is pending", async () => {
    const user = createUser();
    const { rerender } = render(renderDialog());
    await user.click(screen.getByRole("radio", { name: /Target/ }));
    expect(mergeButton()).toBeEnabled();

    mergePending = true;
    rerender(renderDialog());

    expect(mergeButton()).toBeDisabled();
  });

  it("names the book list by its label and keeps the search box out of it", () => {
    render(renderDialog());

    const group = screen.getByRole("radiogroup", {
      name: "Select target book",
    });
    expect(within(group).getByRole("radio", { name: /Target/ })).toBeVisible();
    expect(
      within(group).queryByRole("textbox", { name: "Search books" }),
    ).toBeNull();
    expect(
      screen.getByRole("textbox", { name: "Search books" }),
    ).toBeInTheDocument();
  });
});
