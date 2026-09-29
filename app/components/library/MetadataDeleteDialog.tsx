import { AlertTriangle, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { toastRequestError } from "@/libraries/api";

import type { EntityType } from "./MetadataEditDialog";

interface MetadataDeleteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: EntityType;
  entityName: string;
  /** A rejection is toasted here and keeps the dialog open. */
  onDelete: () => Promise<unknown>;
  isPending: boolean;
}

const ENTITY_LABELS: Record<EntityType, string> = {
  person: "Person",
  series: "Series",
  genre: "Genre",
  tag: "Tag",
  publisher: "Publisher",
};

export function MetadataDeleteDialog({
  open,
  onOpenChange,
  entityType,
  entityName,
  onDelete,
  isPending,
}: MetadataDeleteDialogProps) {
  const handleDelete = async () => {
    try {
      await onDelete();
    } catch (error) {
      toastRequestError(error, `Failed to delete ${entityType}`);
    }
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-destructive shrink-0" />
            <span className="truncate">Delete {ENTITY_LABELS[entityType]}</span>
          </DialogTitle>
        </DialogHeader>
        <DialogBody>
          <DialogDescription>
            Are you sure you want to delete{" "}
            <span className="font-medium break-all" title={entityName}>
              "{entityName}"
            </span>
            ? This action cannot be undone.
          </DialogDescription>
        </DialogBody>

        <DialogFooter>
          <Button
            onClick={() => onOpenChange(false)}
            size="sm"
            variant="outline"
          >
            Cancel
          </Button>
          <Button
            disabled={isPending}
            onClick={handleDelete}
            size="sm"
            variant="destructive"
          >
            {isPending && <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />}
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
