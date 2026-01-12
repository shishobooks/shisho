import { Loader2 } from "lucide-react";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export type EntityType =
  | "person"
  | "series"
  | "genre"
  | "tag"
  | "publisher"
  | "imprint";

interface MetadataEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: EntityType;
  entityName: string;
  sortName?: string;
  onSave: (data: { name: string; sort_name?: string }) => Promise<void>;
  isPending: boolean;
}

const ENTITY_LABELS: Record<EntityType, string> = {
  person: "Person",
  series: "Series",
  genre: "Genre",
  tag: "Tag",
  publisher: "Publisher",
  imprint: "Imprint",
};

export function MetadataEditDialog({
  open,
  onOpenChange,
  entityType,
  entityName,
  sortName,
  onSave,
  isPending,
}: MetadataEditDialogProps) {
  const [name, setName] = useState(entityName);
  const [editSortName, setEditSortName] = useState(sortName || "");

  const hasSortName = entityType === "person" || entityType === "series";

  useEffect(() => {
    if (open) {
      setName(entityName);
      setEditSortName(sortName || "");
    }
  }, [open, entityName, sortName]);

  const handleSubmit = async () => {
    const data: { name: string; sort_name?: string } = { name };
    if (hasSortName) {
      data.sort_name = editSortName || undefined;
    }
    await onSave(data);
  };

  const hasChanges =
    name !== entityName || (hasSortName && editSortName !== (sortName || ""));

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Edit {ENTITY_LABELS[entityType]}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              onChange={(e) => setName(e.target.value)}
              value={name}
            />
          </div>

          {hasSortName && (
            <div className="space-y-2">
              <Label htmlFor="sort_name">Sort Name</Label>
              <Input
                id="sort_name"
                onChange={(e) => setEditSortName(e.target.value)}
                placeholder="Leave empty to auto-generate"
                value={editSortName}
              />
              <p className="text-xs text-muted-foreground">
                Used for sorting. Clear to regenerate automatically.
              </p>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={isPending || !hasChanges || !name.trim()}
            onClick={handleSubmit}
          >
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
