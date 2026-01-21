import { Loader2 } from "lucide-react";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { CreateListPayload, List, UpdateListPayload } from "@/types";

interface CreateListDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // For edit mode
  list?: List;
  // Callbacks
  onCreate?: (payload: CreateListPayload) => Promise<void>;
  onUpdate?: (payload: UpdateListPayload) => Promise<void>;
  isPending: boolean;
}

export function CreateListDialog({
  open,
  onOpenChange,
  list,
  onCreate,
  onUpdate,
  isPending,
}: CreateListDialogProps) {
  const isEditMode = Boolean(list);

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [isOrdered, setIsOrdered] = useState(false);

  // Reset form when dialog opens
  useEffect(() => {
    if (open) {
      if (list) {
        setName(list.name);
        setDescription(list.description || "");
        setIsOrdered(list.is_ordered);
      } else {
        setName("");
        setDescription("");
        setIsOrdered(false);
      }
    }
  }, [open, list]);

  const handleSubmit = async () => {
    if (isEditMode && onUpdate) {
      const payload: UpdateListPayload = {};
      if (name !== list?.name) {
        payload.name = name;
      }
      if (description !== (list?.description || "")) {
        payload.description = description || undefined;
      }
      if (isOrdered !== list?.is_ordered) {
        payload.is_ordered = isOrdered;
      }
      await onUpdate(payload);
    } else if (onCreate) {
      await onCreate({
        name,
        description: description || undefined,
        is_ordered: isOrdered,
      });
    }
  };

  const hasChanges = isEditMode
    ? name !== list?.name ||
      description !== (list?.description || "") ||
      isOrdered !== list?.is_ordered
    : true;

  const canSave = name.trim() && hasChanges;

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{isEditMode ? "Edit List" : "Create List"}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="list-name">Name</Label>
            <Input
              id="list-name"
              onChange={(e) => setName(e.target.value)}
              placeholder="Enter list name"
              value={name}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="list-description">Description</Label>
            <Textarea
              id="list-description"
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Optional description"
              rows={3}
              value={description}
            />
          </div>

          <div className="flex items-start space-x-3">
            <Checkbox
              checked={isOrdered}
              className="mt-1"
              id="list-ordered"
              onCheckedChange={(checked) => setIsOrdered(checked === true)}
            />
            <div className="space-y-1">
              <Label className="cursor-pointer" htmlFor="list-ordered">
                Ordered list
              </Label>
              <p className="text-sm text-muted-foreground">
                Enable manual ordering of books. When disabled, books are sorted
                by the selected sort option.
              </p>
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button disabled={isPending || !canSave} onClick={handleSubmit}>
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {isEditMode ? "Save" : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
