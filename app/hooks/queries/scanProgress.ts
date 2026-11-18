import type { QueryClient } from "@tanstack/react-query";

import type { JobScanProgress } from "@/types";

export enum QueryKey {
  ScanProgress = "ScanProgress",
}

// Scan progress never comes from a request: useSSE writes each scan.progress
// event into the query cache under its library, and useLibraryScan reads it.

/** Records a scan.progress event for its library's page. */
export const applyScanProgress = (
  queryClient: QueryClient,
  progress: JobScanProgress,
) => {
  // No query observes these entries, so they would otherwise be collected
  // five minutes in, in the middle of a long scan.
  queryClient.setQueryDefaults([QueryKey.ScanProgress], { gcTime: Infinity });
  queryClient.setQueryData<JobScanProgress | null>(
    [QueryKey.ScanProgress, progress.library_id],
    progress,
  );
};

/**
 * Marks every library's progress from a scan job as done once the job has
 * ended, including a library the job never sent its last event for. Progress
 * from other scans, which may still be running, is left alone.
 */
export const markScanEnded = (queryClient: QueryClient, jobId: number) => {
  for (const [
    queryKey,
    progress,
  ] of queryClient.getQueriesData<JobScanProgress | null>({
    queryKey: [QueryKey.ScanProgress],
  })) {
    if (progress && progress.job_id === jobId && !progress.done) {
      const ended: JobScanProgress = { ...progress, done: true };
      queryClient.setQueryData<JobScanProgress | null>(queryKey, ended);
    }
  }
};
