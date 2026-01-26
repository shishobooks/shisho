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
import { cn } from "@/libraries/utils";
import type { ListBook } from "@/types";

type InsertPosition = "before" | "after" | null;

interface DraggableBookItemProps {
  listBook: ListBook;
  addedByUsername?: string;
  isDragOverlay?: boolean;
  insertPosition?: InsertPosition;
}

const DraggableBookItem = ({
  listBook,
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
  } = useSortable({ id: listBook.book_id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  if (!listBook.book) return null;

  // For the drag overlay, render a clean lifted version with explicit width
  if (isDragOverlay) {
    return (
      <div className="w-[calc(50vw-1.5rem)] sm:w-32 opacity-95 rotate-1 scale-[1.02] drop-shadow-2xl [&>*]:w-full [&>*]:sm:w-full">
        <BookItem
          addedByUsername={addedByUsername}
          book={listBook.book}
          libraryId={listBook.book.library_id.toString()}
        />
      </div>
    );
  }

  return (
    <div
      className={cn(
        "w-[calc(50%-0.5rem)] sm:w-32 group/drag relative cursor-grab active:cursor-grabbing touch-pan-y [&>*]:w-full [&>*]:sm:w-full",
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
        book={listBook.book}
        libraryId={listBook.book.library_id.toString()}
      />
    </div>
  );
};

interface DraggableBookListProps {
  books: ListBook[];
  isOwner: boolean;
  onReorder: (bookIds: number[]) => void;
}

export const DraggableBookList = ({
  books,
  isOwner,
  onReorder,
}: DraggableBookListProps) => {
  const [items, setItems] = useState(books);
  const [activeId, setActiveId] = useState<number | null>(null);
  const [overId, setOverId] = useState<number | null>(null);
  const isDraggingRef = useRef(false);

  // Sync items from props, but only when not actively dragging
  // This prevents the flash/glitch when server responds after reorder
  useEffect(() => {
    if (isDraggingRef.current) return;

    const booksChanged =
      books.length !== items.length ||
      books.some((b, i) => b.book_id !== items[i]?.book_id);

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
        const oldIndex = items.findIndex((item) => item.book_id === active.id);
        const newIndex = items.findIndex((item) => item.book_id === over.id);

        const newItems = arrayMove(items, oldIndex, newIndex);
        setItems(newItems);
        onReorder(newItems.map((item) => item.book_id));
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
    ? items.find((item) => item.book_id === activeId)
    : null;

  // Calculate insert position for each item
  const getInsertPosition = (bookId: number): InsertPosition => {
    if (!activeId || !overId || activeId === overId) return null;
    if (bookId !== overId) return null;

    const activeIndex = items.findIndex((item) => item.book_id === activeId);
    const overIndex = items.findIndex((item) => item.book_id === overId);

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
        items={items.map((item) => item.book_id)}
        strategy={rectSortingStrategy}
      >
        <div className="flex flex-wrap gap-4">
          {items.map((listBook) => (
            <DraggableBookItem
              addedByUsername={
                !isOwner ? listBook.added_by_user?.username : undefined
              }
              insertPosition={getInsertPosition(listBook.book_id)}
              key={listBook.id}
              listBook={listBook}
            />
          ))}
        </div>
      </SortableContext>
      <DragOverlay>
        {activeItem ? (
          <DraggableBookItem
            addedByUsername={
              !isOwner ? activeItem.added_by_user?.username : undefined
            }
            isDragOverlay
            listBook={activeItem}
          />
        ) : null}
      </DragOverlay>
    </DndContext>
  );
};
