import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useSyncExternalStore } from "react";

import { useLatestScanJob } from "@/hooks/queries/jobs";
import { QueryKey } from "@/hooks/queries/scanProgress";
import { useCan } from "@/hooks/useCan";
import {
  JobStatusInProgress,
  JobStatusPending,
  type JobScanData,
  type JobScanProgress,
} from "@/types";

// Reads what useSSE wrote to the query cache for the library. It is client
// state with nothing to request, so it subscribes to the cache instead of
// running a query.
const useScanProgress = (
  libraryId: number | undefined,
): JobScanProgress | null => {
  const queryClient = useQueryClient();
  const subscribe = useCallback(
    (onChange: () => void) => queryClient.getQueryCache().subscribe(onChange),
    [queryClient],
  );
  return useSyncExternalStore(
    subscribe,
    () =>
      queryClient.getQueryData<JobScanProgress | null>([
        QueryKey.ScanProgress,
        libraryId,
      ]) ?? null,
  );
};

export interface LibraryScan {
  /** A scan of the library is queued or running. */
  scanning: boolean;
  /** The latest counts, once the scan has reached the library. */
  progress?: JobScanProgress;
  /** The scan is the library-wide Scan for new metadata. */
  scanForNewMetadata: boolean;
  /**
   * The scan covers every library and hasn't reported on this one, which it
   * may not have reached or may already be done with.
   */
  everyLibrary: boolean;
}

const NOT_SCANNING: LibraryScan = {
  scanning: false,
  scanForNewMetadata: false,
  everyLibrary: false,
};

/**
 * Whether a scan of the library is running, and how far it has got. A role
 * with Jobs Read also sees a queued scan, and a scan that hasn't reported
 * yet, from the latest scan job; any other role learns of a scan from its
 * progress events, which every signed-in user receives.
 */
export const useLibraryScan = (libraryId: number | undefined): LibraryScan => {
  const canReadJobs = useCan("jobs:read");
  const latestJobQuery = useLatestScanJob(libraryId);
  const progress = useScanProgress(libraryId);

  if (canReadJobs) {
    const job = latestJobQuery.data?.items[0];
    const active =
      job?.status === JobStatusPending || job?.status === JobStatusInProgress;
    if (!job || !active) return NOT_SCANNING;
    const jobProgress =
      progress && progress.job_id === job.id ? progress : undefined;
    if (jobProgress?.done) return NOT_SCANNING;
    const data = job.data as JobScanData | undefined;
    return {
      scanning: true,
      progress: jobProgress,
      scanForNewMetadata:
        jobProgress?.scan_for_new_metadata ??
        data?.scan_for_new_metadata ??
        false,
      everyLibrary: !jobProgress && job.library_id == null,
    };
  }

  if (!progress || progress.done) return NOT_SCANNING;
  return {
    scanning: true,
    progress,
    scanForNewMetadata: progress.scan_for_new_metadata,
    everyLibrary: false,
  };
};
