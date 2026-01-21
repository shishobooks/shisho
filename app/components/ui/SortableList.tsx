import { SortableItem, type DragHandleProps } from "./SortableItem";
import {
  closestCenter,
  DndContext,
  DragEndEvent,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { ReactNode, useCallback } from "react";

export type { DragHandleProps };

interface SortableListProps<T> {
  items: T[];
  getItemId: (item: T, index: number) => string;
  onReorder: (items: T[]) => void;
  renderItem: (
    item: T,
    index: number,
    dragHandleProps: DragHandleProps,
  ) => ReactNode;
}

export function SortableList<T>({
  items,
  getItemId,
  onReorder,
  renderItem,
}: SortableListProps<T>) {
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;

      if (over && active.id !== over.id) {
        const oldIndex = items.findIndex(
          (item, idx) => getItemId(item, idx) === active.id,
        );
        const newIndex = items.findIndex(
          (item, idx) => getItemId(item, idx) === over.id,
        );

        const newItems = arrayMove(items, oldIndex, newIndex);
        onReorder(newItems);
      }
    },
    [items, getItemId, onReorder],
  );

  const itemIds = items.map((item, index) => getItemId(item, index));

  return (
    <DndContext
      collisionDetection={closestCenter}
      onDragEnd={handleDragEnd}
      sensors={sensors}
    >
      <SortableContext items={itemIds} strategy={verticalListSortingStrategy}>
        <div className="space-y-2">
          {items.map((item, index) => (
            <SortableItem
              id={getItemId(item, index)}
              key={getItemId(item, index)}
            >
              {(dragHandleProps) => renderItem(item, index, dragHandleProps)}
            </SortableItem>
          ))}
        </div>
      </SortableContext>
    </DndContext>
  );
}
