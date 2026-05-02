import { AlertTriangle, Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";

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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useDeleteLibrary } from "@/hooks/queries/libraries";

interface DeleteLibraryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  library: { id: number; name: string };
}

export function DeleteLibraryDialog({
  open,
  onOpenChange,
  library,
}: DeleteLibraryDialogProps) {
  const [typedName, setTypedName] = useState("");
  const deleteMutation = useDeleteLibrary();
  const navigate = useNavigate();

  // Reset the typed value whenever the dialog reopens.
  useEffect(() => {
    if (open) setTypedName("");
  }, [open]);

  const canDelete = typedName === library.name && !deleteMutation.isPending;

  const handleConfirm = async () => {
    try {
      await deleteMutation.mutateAsync({ id: library.id });
      toast.success(`Library "${library.name}" deleted.`);
      onOpenChange(false);
      navigate("/settings/libraries");
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Something went wrong.";
      toast.error(msg);
    }
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md overflow-x-hidden">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-destructive shrink-0" />
            Delete library
          </DialogTitle>
          <DialogDescription className="sr-only">
            Permanently delete this library from the database.
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-4 min-w-0">
          <div className="bg-destructive/10 border border-destructive/20 rounded-md p-3 text-sm text-destructive break-words space-y-2">
            <p>This action is irreversible.</p>
            <p>Files on disk will not be deleted.</p>
            <p>
              Sidecar and metadata files will not be cleaned up. You&apos;ll
              need to remove them manually if desired.
            </p>
          </div>

          <p className="text-sm">
            Are you sure you want to delete{" "}
            <span className="font-medium">&ldquo;{library.name}&rdquo;</span>?
          </p>

          <div className="space-y-2">
            <Label htmlFor="delete-library-confirm">
              Type the library name to confirm
            </Label>
            <Input
              autoComplete="off"
              id="delete-library-confirm"
              onChange={(e) => setTypedName(e.target.value)}
              placeholder={library.name}
              value={typedName}
            />
          </div>
        </DialogBody>

        <DialogFooter>
          <Button
            disabled={deleteMutation.isPending}
            onClick={() => onOpenChange(false)}
            size="sm"
            variant="outline"
          >
            Cancel
          </Button>
          <Button
            disabled={!canDelete}
            onClick={handleConfirm}
            size="sm"
            variant="destructive"
          >
            {deleteMutation.isPending && (
              <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
            )}
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
