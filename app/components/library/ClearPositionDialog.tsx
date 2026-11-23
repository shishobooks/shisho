import { toast } from "sonner";

import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { useClearFilePosition } from "@/hooks/queries/reading";
import { toastRequestError } from "@/libraries/api";
import type { File } from "@/types";
import { fileLabel } from "@/utils/format";

interface ClearPositionDialogProps {
  file: File;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Asks before clearing the user's saved position in a File, then clears it.
 * The book page's file rows and the file's own page both offer it. A failed
 * clear keeps the dialog open.
 */
const ClearPositionDialog = ({
  file,
  open,
  onOpenChange,
}: ClearPositionDialogProps) => {
  const clearPosition = useClearFilePosition();

  const handleConfirm = async () => {
    try {
      await clearPosition.mutateAsync(file.id);
      onOpenChange(false);
      toast.success("Saved position cleared");
    } catch (error) {
      toastRequestError(error, "Failed to clear saved position");
    }
  };

  return (
    <ConfirmDialog
      confirmLabel="Clear"
      description={`${fileLabel(file)} will open at the start. Your reading history and status stay as they are.`}
      isPending={clearPosition.isPending}
      onConfirm={handleConfirm}
      onOpenChange={onOpenChange}
      open={open}
      title="Clear saved position?"
    />
  );
};

export default ClearPositionDialog;
