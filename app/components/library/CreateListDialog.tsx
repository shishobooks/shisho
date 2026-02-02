import { Loader2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormDialog } from "@/components/ui/form-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
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
  const [changesSaved, setChangesSaved] = useState(false);
  // Store initial values when dialog opens - used for hasChanges comparison
  const [initialValues, setInitialValues] = useState<{
    name: string;
    description: string;
    isOrdered: boolean;
  } | null>(null);

  // Track previous open state to detect open transitions.
  // Start with false so that if dialog starts open, we detect it as "just opened".
  const prevOpenRef = useRef(false);

  // Initialize form only when dialog opens (closed->open transition)
  // This preserves user edits when list prop changes while dialog is open
  useEffect(() => {
    const justOpened = open && !prevOpenRef.current;
    prevOpenRef.current = open;

    // Only initialize when dialog just opened, not on every prop change
    if (!justOpened) return;

    if (list) {
      const initialName = list.name;
      const initialDescription = list.description || "";
      const initialIsOrdered = list.is_ordered;
      setName(initialName);
      setDescription(initialDescription);
      setIsOrdered(initialIsOrdered);
      setInitialValues({
        name: initialName,
        description: initialDescription,
        isOrdered: initialIsOrdered,
      });
    } else {
      setName("");
      setDescription("");
      setIsOrdered(false);
      setInitialValues({
        name: "",
        description: "",
        isOrdered: false,
      });
    }
    setChangesSaved(false);
  }, [open, list]);

  const handleSubmit = async () => {
    try {
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
      setChangesSaved(true);
      requestClose();
    } catch {
      // Error handling (e.g., toast) is done by the parent callback
    }
  };

  const hasChanges = useMemo(() => {
    if (changesSaved) return false;
    if (!initialValues) return false;
    // Compare against stored initial values, not live props
    return (
      name !== initialValues.name ||
      description !== initialValues.description ||
      isOrdered !== initialValues.isOrdered
    );
  }, [changesSaved, name, description, isOrdered, initialValues]);

  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  const canSave = name.trim() && hasChanges;

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{isEditMode ? "Edit List" : "Create List"}</DialogTitle>
          <DialogDescription>
            {isEditMode
              ? "Update the list name, description, and settings."
              : "Create a new list to organize your books."}
          </DialogDescription>
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
    </FormDialog>
  );
}
