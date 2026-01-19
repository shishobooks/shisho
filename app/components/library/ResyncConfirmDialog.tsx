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

interface ResyncConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: "book" | "file";
  entityName: string;
  onConfirm: () => void;
  isPending: boolean;
}

export function ResyncConfirmDialog({
  open,
  onOpenChange,
  entityType,
  entityName,
  onConfirm,
  isPending,
}: ResyncConfirmDialogProps) {
  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-amber-500 shrink-0" />
            <span>Refresh All Metadata</span>
          </DialogTitle>
          <DialogDescription>
            This will rescan the {entityType} &ldquo;{entityName}&rdquo; and
            overwrite all metadata with values from the source file(s). Any
            manual changes you&apos;ve made will be lost.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            disabled={isPending}
            onClick={() => onOpenChange(false)}
            variant="outline"
          >
            Cancel
          </Button>
          <Button
            disabled={isPending}
            onClick={() => {
              onOpenChange(false);
              onConfirm();
            }}
          >
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Refresh
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
