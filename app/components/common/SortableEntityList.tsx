import { GripVertical, X } from "lucide-react";
import { useRef, type ReactNode } from "react";

import {
  EntityCombobox,
  type EntityComboboxProps,
} from "@/components/common/EntityCombobox";
import {
  StatusBadge,
  type EntityStatus,
} from "@/components/common/StatusBadge";
import { Button } from "@/components/ui/button";
import {
  SortableList,
  type DragHandleProps,
} from "@/components/ui/SortableList";
import { cn } from "@/libraries/utils";

interface SortableEntityListProps<T extends object> {
  items: T[];
  onReorder: (next: T[]) => void;
  onRemove: (index: number) => void;
  onAppend: (next: T | { __create: string }) => void;
  comboboxProps: Pick<
    EntityComboboxProps<T>,
    "hook" | "label" | "getOptionLabel" | "getOptionKey" | "canCreate"
  >;
  renderExtras?: (item: T, index: number) => ReactNode;
  status?: (item: T, index: number) => EntityStatus | undefined;
  pendingCreate?: (item: T) => boolean;
  getItemId?: (item: T, index: number) => string;
}

// Module-level monotonic counter for assigning stable client-side ids to
// list rows. Mirrors the `_editKey` pattern used by chapter editing
// (`pkg/files/FileChaptersTab.tsx`) — index- or label-based ids would
// change after a reorder/remove and confuse dnd-kit's drag tracking.
let rowKeyCounter = 0;
const nextRowKey = () => `row-${++rowKeyCounter}`;

export function SortableEntityList<T extends object>({
  items,
  onReorder,
  onRemove,
  onAppend,
  comboboxProps,
  renderExtras,
  status,
  pendingCreate,
  getItemId,
}: SortableEntityListProps<T>) {
  // WeakMap from item reference to its stable row key. Items survive
  // reorder/remove because the parent reuses references; new items get a
  // fresh key when first seen. Cleared automatically when items are dropped.
  const rowKeysRef = useRef<WeakMap<T, string>>(new WeakMap());

  const itemId = (item: T, index: number): string => {
    if (getItemId) return getItemId(item, index);
    let key = rowKeysRef.current.get(item);
    if (!key) {
      key = nextRowKey();
      rowKeysRef.current.set(item, key);
    }
    return key;
  };

  const excludeAlreadyChosen = (candidate: T) =>
    items.some(
      (existing) =>
        comboboxProps.getOptionLabel(existing).toLowerCase() ===
        comboboxProps.getOptionLabel(candidate).toLowerCase(),
    );

  return (
    <div className="space-y-2">
      <SortableList<T>
        getItemId={itemId}
        items={items}
        onReorder={onReorder}
        renderItem={(item: T, index: number, drag: DragHandleProps) => {
          const rowStatus = status ? status(item, index) : undefined;
          const isPending = pendingCreate ? pendingCreate(item) : false;
          const label = comboboxProps.getOptionLabel(item);

          return (
            <div className="flex items-center gap-2">
              <button
                aria-label={`Drag ${label}`}
                className="cursor-grab touch-none text-muted-foreground hover:text-foreground"
                type="button"
                {...drag.attributes}
                {...drag.listeners}
              >
                <GripVertical className="h-4 w-4" />
              </button>
              <div
                className={cn(
                  "flex-1 truncate rounded-md border px-3 py-2",
                  isPending && "border-dashed",
                )}
                title={label}
              >
                {label}
              </div>
              {renderExtras?.(item, index)}
              {rowStatus && <StatusBadge size="sm" status={rowStatus} />}
              <Button
                aria-label={`Remove ${label}`}
                className="cursor-pointer"
                onClick={() => onRemove(index)}
                size="icon"
                type="button"
                variant="ghost"
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          );
        }}
      />
      <EntityCombobox<T>
        {...comboboxProps}
        exclude={excludeAlreadyChosen}
        onChange={onAppend}
        value={null}
      />
    </div>
  );
}
