import { MoreHorizontal, ScanSearch } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { scanForNewMetadataPayload, useCreateJob } from "@/hooks/queries/jobs";
import { useCan } from "@/hooks/useCan";
import { toastRequestError } from "@/libraries/api";

interface LibraryActionsMenuProps {
  libraryId: number;
  libraryName?: string;
}

/**
 * The library page's actions menu. Gate each entry on its own route's
 * permission. Its only entry so far needs Jobs Read and Jobs Write, so the
 * whole menu hides without them.
 */
export const LibraryActionsMenu = ({
  libraryId,
  libraryName,
}: LibraryActionsMenuProps) => {
  // Starting a scan is the resync button's permission: it reads the latest
  // scan (Jobs Read) and creates one (Jobs Write).
  const canScan = useCan(["jobs:read", "jobs:write"]);
  const createJob = useCreateJob();
  const [confirmOpen, setConfirmOpen] = useState(false);

  if (!canScan) return null;

  const startScan = () => {
    createJob.mutate(
      { payload: scanForNewMetadataPayload(libraryId) },
      {
        onError: (error) => toastRequestError(error, "Failed to start scan"),
        onSuccess: () => setConfirmOpen(false),
      },
    );
  };

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button aria-label="Library actions" size="icon-sm" variant="ghost">
            <MoreHorizontal className="h-4 w-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          onCloseAutoFocus={(e) => e.preventDefault()}
        >
          <DropdownMenuItem onSelect={() => setConfirmOpen(true)}>
            <ScanSearch className="mr-2 h-4 w-4" />
            Scan for new metadata
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <ConfirmDialog
        confirmLabel="Start scan"
        description={`Runs your metadata plugins again on every book in ${libraryName ?? "this library"}, including books whose files haven't changed, replacing details that came from the files themselves. It takes a while and calls each plugin's services for every book. Manual edits and values you chose in Identify are kept.`}
        isPending={createJob.isPending}
        onConfirm={startScan}
        onOpenChange={setConfirmOpen}
        open={confirmOpen}
        title="Scan for new metadata?"
        variant="default"
      />
    </>
  );
};
