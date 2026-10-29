import type { DndContextProps } from "@dnd-kit/core";
import { act, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Book } from "@/types";

import { DraggableBookList } from "./DraggableBookList";

// Capture the drag handlers so the test can finish a drag directly; jsdom has
// no layout for dnd-kit's sensors and collision detection to work with.
let dnd: DndContextProps;
vi.mock("@dnd-kit/core", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@dnd-kit/core")>();
  return {
    ...actual,
    DndContext: (props: DndContextProps) => {
      dnd = props;
      return <>{props.children}</>;
    },
    DragOverlay: () => null,
  };
});

vi.mock("@/components/library/BookItem", () => ({
  default: ({
    addedByUsername,
    book,
    coverAspectRatio,
  }: {
    addedByUsername?: string;
    book: { title: string; reading_status?: string };
    coverAspectRatio?: string;
  }) => (
    <span
      data-added-by={addedByUsername}
      data-aspect={coverAspectRatio}
      data-status={book.reading_status}
      data-testid="title"
    >
      {book.title}
    </span>
  ),
}));

const book = (id: number, title: string, library_id = 1) =>
  ({ id, title, library_id, cover_cache_key: "" }) as unknown as Book;

const books = [book(1, "First"), book(2, "Second")];
const threeBooks = [...books, book(3, "Third")];

const titles = () =>
  screen.getAllByTestId("title").map((node) => node.textContent);

const drag = async (active: number, over: number) => {
  await act(async () => {
    dnd.onDragStart?.({ active: { id: active } } as never);
    dnd.onDragEnd?.({ active: { id: active }, over: { id: over } } as never);
  });
};
const dragFirstOntoSecond = () => drag(1, 2);

describe("DraggableBookList", () => {
  it("shapes each book's cover by its own library's cover aspect ratio", () => {
    render(
      <DraggableBookList
        books={[book(1, "First"), book(2, "Second", 2)]}
        columns={4}
        coverAspectRatioFor={(libraryId) =>
          libraryId === 2 ? "audiobook" : "book"
        }
        onReorder={vi.fn()}
      />,
    );

    expect(
      screen.getAllByTestId("title").map((node) => node.dataset.aspect),
    ).toEqual(["book", "audiobook"]);
  });

  it("keeps the new order when the reorder succeeds", async () => {
    const onReorder = vi.fn().mockResolvedValue(undefined);
    render(
      <DraggableBookList
        books={books}
        columns={4}
        coverAspectRatioFor={() => "book"}
        onReorder={onReorder}
      />,
    );

    await dragFirstOntoSecond();

    expect(onReorder).toHaveBeenCalledWith([2, 1]);
    expect(titles()).toEqual(["Second", "First"]);
  });

  it("restores the previous order when the reorder is rejected", async () => {
    const onReorder = vi.fn().mockRejectedValue(new Error("rejected"));
    render(
      <DraggableBookList
        books={books}
        columns={4}
        coverAspectRatioFor={() => "book"}
        onReorder={onReorder}
      />,
    );

    await dragFirstOntoSecond();

    expect(onReorder).toHaveBeenCalledWith([2, 1]);
    expect(titles()).toEqual(["First", "Second"]);
  });

  it("ignores a late rejection from a drag that a newer drag replaced", async () => {
    let rejectFirst: (error: Error) => void = () => {};
    const onReorder = vi
      .fn()
      .mockReturnValueOnce(
        new Promise((_resolve, reject) => {
          rejectFirst = reject;
        }),
      )
      .mockResolvedValueOnce(undefined);
    render(
      <DraggableBookList
        books={threeBooks}
        columns={4}
        coverAspectRatioFor={() => "book"}
        onReorder={onReorder}
      />,
    );

    await drag(1, 2); // Second, First, Third (still saving)
    await drag(3, 2); // Third, Second, First (saved)
    await act(async () => rejectFirst(new Error("rejected")));

    expect(titles()).toEqual(["Third", "Second", "First"]);
  });

  // A refetch after adding a card's book to To Read changes its status but
  // not the order, and the card has to show it.
  it("shows a refetched book's new status in the same order", () => {
    const props = {
      columns: 4,
      coverAspectRatioFor: () => "book",
      onReorder: vi.fn(),
    };
    const { rerender } = render(<DraggableBookList books={books} {...props} />);

    rerender(
      <DraggableBookList
        books={[{ ...books[0], reading_status: "to_read" } as Book, books[1]]}
        {...props}
      />,
    );

    expect(
      screen.getAllByTestId("title").map((node) => node.dataset.status),
    ).toEqual(["to_read", undefined]);
  });

  it("names who added each book", () => {
    render(
      <DraggableBookList
        addedByUsernameFor={(bookId) => (bookId === 2 ? "friend" : undefined)}
        books={books}
        columns={4}
        coverAspectRatioFor={() => "book"}
        onReorder={vi.fn()}
      />,
    );

    expect(
      screen.getAllByTestId("title").map((node) => node.dataset.addedBy),
    ).toEqual([undefined, "friend"]);
  });
});
