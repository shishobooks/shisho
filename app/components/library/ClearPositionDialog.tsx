import { RotateCcw } from "lucide-react";
import { toast } from "sonner";

import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { useClearFilePosition } from "@/hooks/queries/reading";
import { useAuth } from "@/hooks/useAuth";
import { DEMO_MODE_MESSAGE, toastRequestError } from "@/libraries/api";
import type { File } from "@/types";
import { fileLabel } from "@/utils/format";

/**
 * The file menu's entry that opens ClearPositionDialog. The demo can't
 * change reading data, so there it is disabled and says why. iconClassName
 * matches the menu's other icons, which space themselves differently from
 * page to page.
 */
export const ClearPositionMenuItem = ({
  onSelect,
  iconClassName = "h-4 w-4",
}: {
  onSelect: () => void;
  iconClassName?: string;
}) => {
  const { demoMode } = useAuth();
  return (
    <DropdownMenuItem disabled={demoMode} onSelect={onSelect}>
      <RotateCcw aria-hidden="true" className={iconClassName} />
      <span className="flex flex-col">
        Clear saved position
        {demoMode && (
          <span className="text-xs text-muted-foreground">
            {DEMO_MODE_MESSAGE}
          </span>
        )}
      </span>
    </DropdownMenuItem>
  );
};

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
