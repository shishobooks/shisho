import {
  closestCenter,
  DndContext,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  ArrowDown,
  ArrowDownUp,
  ArrowUp,
  GripVertical,
  Plus,
  Save,
  X,
} from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@/components/ui/drawer";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import {
  SORT_FIELD_LABELS,
  SORT_FIELDS,
  type SortField,
  type SortLevel,
} from "@/libraries/sortSpec";

interface SortSheetProps {
  levels: readonly SortLevel[];
  onChange: (levels: SortLevel[]) => void;
  onSaveAsDefault: () => void;
  isDirty: boolean;
  isSaving: boolean;
}

interface SortLevelRowProps {
  level: SortLevel;
  index: number;
  usedFields: SortField[];
  onChangeField: (index: number, field: SortField) => void;
  onToggleDirection: (index: number) => void;
  onRemove: (index: number) => void;
}

const SortLevelRow = ({
  level,
  index,
  usedFields,
  onChangeField,
  onToggleDirection,
  onRemove,
}: SortLevelRowProps) => {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: level.field });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };

  // Available fields for this row: fields not used by other rows + this row's current field
  const availableFields = SORT_FIELDS.filter(
    (f) => f === level.field || !usedFields.includes(f),
  );

  const fieldLabel = SORT_FIELD_LABELS[level.field];
  const isAsc = level.direction === "asc";

  return (
    <div
      className="flex items-center gap-2 py-1"
      ref={setNodeRef}
      style={style}
    >
      <Button
        aria-label={`Reorder sort level ${index + 1}`}
        className="cursor-grab"
        size="icon"
        variant="ghost"
        {...attributes}
        {...listeners}
      >
        <GripVertical className="h-4 w-4" />
      </Button>

      <Select
        onValueChange={(value) => onChangeField(index, value as SortField)}
        value={level.field}
      >
        <SelectTrigger className="flex-1">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {availableFields.map((f) => (
            <SelectItem key={f} value={f}>
              {SORT_FIELD_LABELS[f]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Button
        aria-label={
          isAsc
            ? "Direction: ascending. Click to toggle."
            : "Direction: descending. Click to toggle."
        }
        onClick={() => onToggleDirection(index)}
        size="icon"
        variant="outline"
      >
        {isAsc ? (
          <ArrowUp className="h-4 w-4" />
        ) : (
          <ArrowDown className="h-4 w-4" />
        )}
      </Button>

      <Button
        aria-label={`Remove ${fieldLabel} sort level`}
        onClick={() => onRemove(index)}
        size="icon"
        variant="ghost"
      >
        <X className="h-4 w-4" />
      </Button>
    </div>
  );
};

const SortSheetContent = ({
  levels,
  onChange,
  onSaveAsDefault,
  isDirty,
  isSaving,
}: SortSheetProps) => {
  const usedFields = levels.map((l) => l.field);
  const unusedFields = SORT_FIELDS.filter((f) => !usedFields.includes(f));

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = levels.findIndex((l) => l.field === active.id);
    const newIndex = levels.findIndex((l) => l.field === over.id);
    if (oldIndex === -1 || newIndex === -1) return;
    onChange(arrayMove([...levels], oldIndex, newIndex));
  };

  const addLevel = (field: SortField) => {
    onChange([...levels, { field, direction: "asc" }]);
  };

  const changeField = (index: number, field: SortField) => {
    onChange(levels.map((l, i) => (i === index ? { ...l, field } : l)));
  };

  const toggleDirection = (index: number) => {
    onChange(
      levels.map((l, i) =>
        i === index
          ? { ...l, direction: l.direction === "asc" ? "desc" : "asc" }
          : l,
      ),
    );
  };

  const removeLevel = (index: number) => {
    onChange(levels.filter((_, i) => i !== index));
  };

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  return (
    <div className="flex flex-col gap-4 py-4">
      {levels.length > 0 && (
        <DndContext
          collisionDetection={closestCenter}
          onDragEnd={handleDragEnd}
          sensors={sensors}
        >
          <SortableContext
            items={levels.map((l) => l.field)}
            strategy={verticalListSortingStrategy}
          >
            <div className="flex flex-col">
              {levels.map((level, index) => (
                <SortLevelRow
                  index={index}
                  key={level.field}
                  level={level}
                  onChangeField={changeField}
                  onRemove={removeLevel}
                  onToggleDirection={toggleDirection}
                  usedFields={usedFields}
                />
              ))}
            </div>
          </SortableContext>
        </DndContext>
      )}

      {unusedFields.length > 0 && (
        <div>
          <p className="text-xs font-semibold text-muted-foreground mb-2">
            {levels.length === 0 ? "Sort by…" : "Then by…"}
          </p>
          <div className="flex flex-wrap gap-2">
            {unusedFields.map((field) => (
              <Button
                key={field}
                onClick={() => addLevel(field)}
                size="sm"
                variant="outline"
              >
                <Plus className="h-4 w-4" />
                {SORT_FIELD_LABELS[field]}
              </Button>
            ))}
          </div>
        </div>
      )}

      {isDirty && (
        <div className="border border-dashed rounded-md p-3">
          <p className="text-sm text-muted-foreground mb-2">
            Save this as the default for this library?
          </p>
          <Button disabled={isSaving} onClick={onSaveAsDefault} size="sm">
            <Save className="h-4 w-4" />
            {isSaving ? "Saving…" : "Save as default"}
          </Button>
        </div>
      )}
    </div>
  );
};

const SortSheet = ({
  trigger,
  ...props
}: SortSheetProps & { trigger: React.ReactNode }) => {
  const [open, setOpen] = useState(false);
  const isDesktop = useMediaQuery("(min-width: 768px)");

  if (isDesktop) {
    return (
      <Sheet onOpenChange={setOpen} open={open}>
        <SheetTrigger asChild>{trigger}</SheetTrigger>
        <SheetContent className="flex flex-col overflow-hidden">
          <SheetHeader>
            <SheetTitle>Sort</SheetTitle>
          </SheetHeader>
          <div className="flex-1 overflow-y-auto pr-1">
            <SortSheetContent {...props} />
          </div>
        </SheetContent>
      </Sheet>
    );
  }

  return (
    <Drawer onOpenChange={setOpen} open={open}>
      <DrawerTrigger asChild>{trigger}</DrawerTrigger>
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>Sort</DrawerTitle>
        </DrawerHeader>
        <div className="overflow-y-auto px-4 pb-4 max-h-[70vh]">
          <SortSheetContent {...props} />
        </div>
      </DrawerContent>
    </Drawer>
  );
};

export const SortButton = ({
  isDirty,
  onClick,
}: {
  isDirty: boolean;
  onClick?: () => void;
}) => (
  <Button className="relative" onClick={onClick} size="sm" variant="outline">
    <ArrowDownUp className="h-4 w-4" />
    Sort
    {isDirty && (
      <span
        aria-label="Sort differs from default"
        className="absolute top-1 right-1 h-2 w-2 rounded-full bg-primary ring-2 ring-background"
      />
    )}
  </Button>
);

export default SortSheet;
