import * as React from "react";

import { Dialog } from "@/components/ui/dialog";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";

type DialogProps = React.ComponentProps<typeof Dialog>;

/**
 * Context to allow DialogContent to close a FormDialog directly,
 * bypassing the unsaved changes warning. Used for explicit close actions
 * like the X button, which shouldn't trigger the warning.
 */
export const FormDialogContext = React.createContext<{
  closeDirectly: () => void;
} | null>(null);

interface FormDialogProps extends DialogProps {
  /**
   * Whether the form has unsaved changes. When true, closing the dialog
   * will show a confirmation dialog.
   */
  hasChanges: boolean;
}

/**
 * A wrapper around Dialog that warns users when they try to close a form
 * with unsaved changes. Intercepts close attempts (X button, ESC key,
 * overlay click, Cancel button) and shows a confirmation dialog.
 *
 * @example
 * <FormDialog open={open} onOpenChange={setOpen} hasChanges={hasChanges}>
 *   <DialogContent>
 *     <DialogHeader>
 *       <DialogTitle>Edit Item</DialogTitle>
 *     </DialogHeader>
 *     <form>...</form>
 *   </DialogContent>
 * </FormDialog>
 */
export function FormDialog({
  hasChanges,
  open,
  onOpenChange,
  children,
  ...props
}: FormDialogProps) {
  const [showConfirmDialog, setShowConfirmDialog] = React.useState(false);

  // Reset confirmation dialog when main dialog closes externally
  React.useEffect(() => {
    if (!open) {
      setShowConfirmDialog(false);
    }
  }, [open]);

  // Warn on browser refresh/close when dialog is open with unsaved changes
  React.useEffect(() => {
    if (!open || !hasChanges) return;

    const handleBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };

    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => window.removeEventListener("beforeunload", handleBeforeUnload);
  }, [open, hasChanges]);

  const handleOpenChange = (newOpen: boolean) => {
    // If trying to close and there are unsaved changes, show confirmation
    // Guard against showing multiple dialogs on rapid clicks
    if (!newOpen && hasChanges && !showConfirmDialog) {
      setShowConfirmDialog(true);
      return;
    }

    onOpenChange?.(newOpen);
  };

  const handleStay = () => {
    setShowConfirmDialog(false);
  };

  const handleDiscard = () => {
    setShowConfirmDialog(false);
    onOpenChange?.(false);
  };

  // Close directly without showing the warning - used by explicit close actions
  // like the X button, which shouldn't trigger the unsaved changes warning
  const closeDirectly = React.useCallback(() => {
    onOpenChange?.(false);
  }, [onOpenChange]);

  const contextValue = React.useMemo(
    () => ({ closeDirectly }),
    [closeDirectly],
  );

  return (
    <>
      <Dialog open={open} onOpenChange={handleOpenChange} {...props}>
        <FormDialogContext.Provider value={contextValue}>
          {children}
        </FormDialogContext.Provider>
      </Dialog>
      <UnsavedChangesDialog
        open={showConfirmDialog}
        onStay={handleStay}
        onDiscard={handleDiscard}
      />
    </>
  );
}
