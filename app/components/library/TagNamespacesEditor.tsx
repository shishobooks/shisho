import { ArrowDown, ArrowUp, GripVertical, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  SortableList,
  type DragHandleProps,
} from "@/components/ui/SortableList";
import { draftNamespace, type DraftNamespace } from "@/utils/tagNamespaces";

interface TagNamespacesEditorProps {
  namespaces: DraftNamespace[];
  onChange: (namespaces: DraftNamespace[]) => void;
}

// TagNamespacesEditor edits a library's ordered list of tag namespaces:
// rename, reorder (the order of the book page's rows), remove, or add one.
// Saving is the caller's.
const TagNamespacesEditor = ({
  namespaces,
  onChange,
}: TagNamespacesEditorProps) => {
  const move = (index: number, offset: number) => {
    const next = [...namespaces];
    const [entry] = next.splice(index, 1);
    next.splice(index + offset, 0, entry);
    onChange(next);
  };

  return (
    <div className="space-y-2">
      <SortableList<DraftNamespace>
        getItemId={(entry) => entry.key}
        items={namespaces}
        onReorder={onChange}
        renderItem={(entry, index, drag: DragHandleProps) => {
          const label = entry.name.trim() || `namespace ${index + 1}`;
          return (
            <div className="flex items-center gap-2 py-1">
              <Button
                aria-label={`Drag ${label}`}
                className="cursor-grab touch-none text-muted-foreground hover:text-foreground"
                variant="unstyled"
                {...drag.attributes}
                {...drag.listeners}
              >
                <GripVertical className="h-4 w-4" />
              </Button>
              <Input
                aria-label={`Tag namespace ${index + 1}`}
                className="flex-1"
                maxLength={50}
                onChange={(e) =>
                  onChange(
                    namespaces.map((n) =>
                      n.key === entry.key ? { ...n, name: e.target.value } : n,
                    ),
                  )
                }
                placeholder="Source"
                value={entry.name}
              />
              <div className="flex items-center">
                <Button
                  aria-label={`Move ${label} up`}
                  disabled={index === 0}
                  onClick={() => move(index, -1)}
                  size="icon-sm"
                  variant="ghost"
                >
                  <ArrowUp className="h-4 w-4" />
                </Button>
                <Button
                  aria-label={`Move ${label} down`}
                  disabled={index === namespaces.length - 1}
                  onClick={() => move(index, 1)}
                  size="icon-sm"
                  variant="ghost"
                >
                  <ArrowDown className="h-4 w-4" />
                </Button>
                <Button
                  aria-label={`Remove ${label}`}
                  onClick={() =>
                    onChange(namespaces.filter((n) => n.key !== entry.key))
                  }
                  size="icon-sm"
                  variant="ghost"
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>
            </div>
          );
        }}
      />
      <Button
        onClick={() => onChange([...namespaces, draftNamespace()])}
        variant="outline"
      >
        Add Namespace
      </Button>
    </div>
  );
};

export default TagNamespacesEditor;
