import type { EntityType } from "./MetadataEditDialog";
import { AlertTriangle, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface MetadataDeleteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: EntityType;
  entityName: string;
  onDelete: () => Promise<void>;
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

export function MetadataDeleteDialog({
  open,
  onOpenChange,
  entityType,
  entityName,
  onDelete,
  isPending,
}: MetadataDeleteDialogProps) {
  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-destructive shrink-0" />
            <span className="truncate">Delete {ENTITY_LABELS[entityType]}</span>
          </DialogTitle>
          <DialogDescription>
            Are you sure you want to delete{" "}
            <span className="font-medium break-all" title={entityName}>
              "{entityName}"
            </span>
            ? This action cannot be undone.
          </DialogDescription>
        </DialogHeader>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button disabled={isPending} onClick={onDelete} variant="destructive">
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
