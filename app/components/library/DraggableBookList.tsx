import {
  closestCenter,
  DndContext,
  DragEndEvent,
  DragOverEvent,
  DragOverlay,
  DragStartEvent,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  arrayMove,
  rectSortingStrategy,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useCallback, useEffect, useRef, useState } from "react";

import BookItem from "@/components/library/BookItem";
import GalleryGrid from "@/components/library/GalleryGrid";
import { cn } from "@/libraries/utils";
import type { Book } from "@/types";

type InsertPosition = "before" | "after" | null;

interface DraggableBookItemProps {
  book: Book;
  coverAspectRatioFor: (libraryId: number) => string;
  addedByUsername?: string;
  isDragOverlay?: boolean;
  insertPosition?: InsertPosition;
}

const DraggableBookItem = ({
  book,
  coverAspectRatioFor,
  addedByUsername,
  isDragOverlay = false,
  insertPosition = null,
}: DraggableBookItemProps) => {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: book.id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  const coverAspectRatio = coverAspectRatioFor(book.library_id);

  // For the drag overlay, render a clean lifted version with explicit width
  if (isDragOverlay) {
    return (
      <div className="w-full opacity-95 rotate-1 scale-[1.02] drop-shadow-2xl">
        <BookItem
          addedByUsername={addedByUsername}
          book={book}
          coverAspectRatio={coverAspectRatio}
          libraryId={book.library_id.toString()}
        />
      </div>
    );
  }

  return (
    <div
      className={cn(
        "w-full group/drag relative cursor-grab active:cursor-grabbing touch-pan-y",
        isDragging && "opacity-30",
        // Show indicator on left when inserting before this item
        insertPosition === "before" &&
          "before:absolute before:-left-3 before:top-0 before:bottom-0 before:w-1 before:bg-primary before:rounded-full",
        // Show indicator on right when inserting after this item
        insertPosition === "after" &&
          "after:absolute after:-right-3 after:top-0 after:bottom-0 after:w-1 after:bg-primary after:rounded-full",
      )}
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
    >
      <BookItem
        addedByUsername={addedByUsername}
        book={book}
        coverAspectRatio={coverAspectRatio}
        libraryId={book.library_id.toString()}
      />
    </div>
  );
};

interface DraggableBookListProps {
  /** In order. Each Book appears once, so its id keys its row. */
  books: Book[];
  /** The cover aspect ratio of a book's library, which shapes its cover. */
  coverAspectRatioFor: (libraryId: number) => string;
  /** From `useGallerySizeParam`, matching the gallery the list replaces. */
  columns: number;
  /** Who added a book, shown on its card, or undefined to show no one. */
  addedByUsernameFor?: (bookId: number) => string | undefined;
  // Rejecting restores the order from before the drag.
  onReorder: (bookIds: number[]) => Promise<unknown>;
}

export const DraggableBookList = ({
  books,
  coverAspectRatioFor,
  columns,
  addedByUsernameFor,
  onReorder,
}: DraggableBookListProps) => {
  const [items, setItems] = useState(books);
  const [activeId, setActiveId] = useState<number | null>(null);
  const [overId, setOverId] = useState<number | null>(null);
  const isDraggingRef = useRef(false);
  // The server order, read when a save fails so overlapping drags fall back
  // to what the server holds rather than an order captured by an older drag.
  const booksRef = useRef(books);
  // Counts drags so a rejection only rolls back if no newer drag followed it.
  const reorderSeqRef = useRef(0);
  useEffect(() => {
    booksRef.current = books;
  }, [books]);

  // Sync items from props, but only when not actively dragging
  // This prevents the flash/glitch when server responds after reorder
  useEffect(() => {
    if (isDraggingRef.current) return;

    // By identity, not id: a refetch that changes a book (its status after
    // a To Read change) but not the order must still reach its card. Query
    // results share unchanged objects, so an unchanged refetch matches.
    const booksChanged =
      books.length !== items.length || books.some((b, i) => b !== items[i]);

    if (booksChanged) {
      setItems(books);
    }
  }, [books, items]);

  const sensors = useSensors(
    useSensor(MouseSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
    useSensor(TouchSensor, {
      activationConstraint: {
        delay: 200,
        tolerance: 25,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const handleDragStart = useCallback((event: DragStartEvent) => {
    isDraggingRef.current = true;
    setActiveId(event.active.id as number);
  }, []);

  const handleDragOver = useCallback((event: DragOverEvent) => {
    const { over } = event;
    setOverId(over ? (over.id as number) : null);
  }, []);

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;

      // Clear drag state
      setActiveId(null);
      setOverId(null);

      if (over && active.id !== over.id) {
        const oldIndex = items.findIndex((item) => item.id === active.id);
        const newIndex = items.findIndex((item) => item.id === over.id);

        const newItems = arrayMove(items, oldIndex, newIndex);
        setItems(newItems);
        // A rejected save (including a Demo Mode rejection) leaves the server
        // order unchanged, and an unchanged refetch would not resync the
        // props, so put the server order back here.
        const seq = ++reorderSeqRef.current;
        onReorder(newItems.map((item) => item.id)).catch(() => {
          if (seq === reorderSeqRef.current) setItems(booksRef.current);
        });
      }

      // Allow prop sync after a short delay to let optimistic update settle
      setTimeout(() => {
        isDraggingRef.current = false;
      }, 500);
    },
    [items, onReorder],
  );

  const handleDragCancel = useCallback(() => {
    setActiveId(null);
    setOverId(null);
    isDraggingRef.current = false;
  }, []);

  const activeItem = activeId
    ? items.find((item) => item.id === activeId)
    : null;

  // Calculate insert position for each item
  const getInsertPosition = (bookId: number): InsertPosition => {
    if (!activeId || !overId || activeId === overId) return null;
    if (bookId !== overId) return null;

    const activeIndex = items.findIndex((item) => item.id === activeId);
    const overIndex = items.findIndex((item) => item.id === overId);

    // If dragging from earlier position to later, show indicator after
    // If dragging from later position to earlier, show indicator before
    return activeIndex < overIndex ? "after" : "before";
  };

  return (
    <DndContext
      collisionDetection={closestCenter}
      onDragCancel={handleDragCancel}
      onDragEnd={handleDragEnd}
      onDragOver={handleDragOver}
      onDragStart={handleDragStart}
      sensors={sensors}
    >
      <SortableContext
        items={items.map((item) => item.id)}
        strategy={rectSortingStrategy}
      >
        <GalleryGrid className="mb-0 md:mb-0" columns={columns}>
          {items.map((book) => (
            <DraggableBookItem
              addedByUsername={addedByUsernameFor?.(book.id)}
              book={book}
              coverAspectRatioFor={coverAspectRatioFor}
              insertPosition={getInsertPosition(book.id)}
              key={book.id}
            />
          ))}
        </GalleryGrid>
      </SortableContext>
      <DragOverlay>
        {activeItem ? (
          <DraggableBookItem
            addedByUsername={addedByUsernameFor?.(activeItem.id)}
            book={activeItem}
            coverAspectRatioFor={coverAspectRatioFor}
            isDragOverlay
          />
        ) : null}
      </DragOverlay>
    </DndContext>
  );
};
