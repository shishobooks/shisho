import type { DndContextProps } from "@dnd-kit/core";
import { act, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ListBook } from "@/types";

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
  default: ({ book }: { book: { title: string } }) => (
    <span data-testid="title">{book.title}</span>
  ),
}));

const listBook = (id: number, title: string) =>
  ({
    id,
    book_id: id,
    book: { id, title, library_id: 1, cover_cache_key: "" },
  }) as unknown as ListBook;

const books = [listBook(1, "First"), listBook(2, "Second")];
const threeBooks = [...books, listBook(3, "Third")];

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
  it("keeps the new order when the reorder succeeds", async () => {
    const onReorder = vi.fn().mockResolvedValue(undefined);
    render(<DraggableBookList books={books} isOwner onReorder={onReorder} />);

    await dragFirstOntoSecond();

    expect(onReorder).toHaveBeenCalledWith([2, 1]);
    expect(titles()).toEqual(["Second", "First"]);
  });

  it("restores the previous order when the reorder is rejected", async () => {
    const onReorder = vi.fn().mockRejectedValue(new Error("rejected"));
    render(<DraggableBookList books={books} isOwner onReorder={onReorder} />);

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
      <DraggableBookList books={threeBooks} isOwner onReorder={onReorder} />,
    );

    await drag(1, 2); // Second, First, Third (still saving)
    await drag(3, 2); // Third, Second, First (saved)
    await act(async () => rejectFirst(new Error("rejected")));

    expect(titles()).toEqual(["Third", "Second", "First"]);
  });
});
