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

interface UnsavedChangesDialogProps {
  open: boolean;
  onStay: () => void;
  onDiscard: () => void;
}

/**
 * Dialog shown when the user attempts to navigate away with unsaved changes.
 * Used for both route navigation (via useUnsavedChanges) and dialog close attempts
 * (via FormDialog).
 */
export function UnsavedChangesDialog({
  open,
  onStay,
  onDiscard,
}: UnsavedChangesDialogProps) {
  return (
    <Dialog open={open} onOpenChange={(isOpen) => !isOpen && onStay()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Unsaved Changes</DialogTitle>
        </DialogHeader>
        <DialogBody>
          <DialogDescription className="text-sm">
            You have unsaved changes. Are you sure you want to leave? Your
            changes will be lost.
          </DialogDescription>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={onStay} size="sm">
            Stay
          </Button>
          <Button variant="destructive" onClick={onDiscard} size="sm">
            Discard Changes
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
