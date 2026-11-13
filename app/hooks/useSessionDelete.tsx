import { useState } from "react";
import { toast } from "sonner";

import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { useDeleteReadingSession } from "@/hooks/queries/reading";
import { isDemoModeError, toastRequestError } from "@/libraries/api";

/**
 * Deleting a Reading Session, behind a confirmation. Call `requestDelete`
 * with the session's id and a line naming it ("Dune, 7:00 AM to 8:00 AM"),
 * and render `dialog`. Demo Mode refuses the delete; its toast says so, and
 * the dialog closes, as retrying won't help.
 */
export const useSessionDelete = () => {
  const mutation = useDeleteReadingSession();
  const [pending, setPending] = useState<{
    id: number;
    description: string;
  } | null>(null);

  const handleDelete = async () => {
    if (!pending) return;
    try {
      await mutation.mutateAsync(pending.id);
      toast.success("Reading session deleted");
      setPending(null);
    } catch (error) {
      if (isDemoModeError(error)) setPending(null);
      toastRequestError(error, "Failed to delete reading session");
    }
  };

  return {
    requestDelete: (id: number, description: string) =>
      setPending({ id, description }),
    dialog: (
      <ConfirmDialog
        confirmLabel="Delete"
        description={
          pending
            ? `${pending.description}. Its time stops counting toward your Reading Stats, and it can't be brought back.`
            : ""
        }
        isPending={mutation.isPending}
        onConfirm={handleDelete}
        onOpenChange={(open) => !open && setPending(null)}
        open={!!pending}
        title="Delete this reading session?"
        variant="destructive"
      />
    ),
  };
};
