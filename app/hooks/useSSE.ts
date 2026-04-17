import { useQueryClient } from "@tanstack/react-query";
import { useContext, useEffect, useLayoutEffect, useRef } from "react";

import {
  BulkDownloadContext,
  type BulkDownloadContextValue,
} from "@/contexts/BulkDownload";
import { QueryKey as BooksQueryKey } from "@/hooks/queries/books";
import { QueryKey as JobsQueryKey } from "@/hooks/queries/jobs";
import { QueryKey as LibrariesQueryKey } from "@/hooks/queries/libraries";
import { QueryKey as LogsQueryKey } from "@/hooks/queries/logs";
import { useAuth } from "@/hooks/useAuth";

export function useSSE() {
  const queryClient = useQueryClient();
  const { isAuthenticated } = useAuth();
  const bulkDownload = useContext(BulkDownloadContext);

  // Use ref to avoid the EventSource reconnecting on every progress update.
  // Synced via useLayoutEffect so it's always up-to-date before any async
  // callbacks fire.
  const bulkDownloadRef = useRef<BulkDownloadContextValue | null>(null);
  useLayoutEffect(() => {
    bulkDownloadRef.current = bulkDownload;
  });

  useEffect(() => {
    if (!isAuthenticated) {
      return;
    }

    const es = new EventSource("/api/events");

    es.onerror = () => {
      // EventSource auto-reconnects on error. Log for debugging visibility
      // but don't take action — reconnection is handled by the browser.
      console.debug("[SSE] Connection error, will auto-reconnect");
    };

    const handleJobCreated = () => {
      queryClient.invalidateQueries({ queryKey: [JobsQueryKey.ListJobs] });
      queryClient.invalidateQueries({
        queryKey: [JobsQueryKey.LatestScanJob],
      });
    };

    const handleJobStatusChanged = (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data);
        queryClient.invalidateQueries({ queryKey: [JobsQueryKey.ListJobs] });
        queryClient.invalidateQueries({
          queryKey: [JobsQueryKey.LatestScanJob],
        });
        queryClient.invalidateQueries({
          queryKey: [JobsQueryKey.RetrieveJob, String(data.job_id)],
        });
        queryClient.invalidateQueries({
          queryKey: [JobsQueryKey.ListJobLogs, String(data.job_id)],
        });

        // When a scan completes, invalidate book queries so lists refresh
        if (data.status === "completed" && data.type === "scan") {
          queryClient.invalidateQueries({
            queryKey: [BooksQueryKey.ListBooks],
          });
          // Also invalidate library languages in case scanned files introduced new languages
          queryClient.invalidateQueries({
            queryKey: [LibrariesQueryKey.LibraryLanguages],
          });
        }

        // Handle bulk download completion/failure
        const bd = bulkDownloadRef.current;
        if (data.type === "bulk_download" && bd) {
          if (data.status === "completed") {
            bd.completeDownload(data.job_id);
          } else if (data.status === "failed") {
            bd.failDownload(data.job_id);
          }
        }
      } catch {
        // Ignore malformed events
      }
    };

    const handleBulkDownloadProgress = (event: MessageEvent) => {
      const bd = bulkDownloadRef.current;
      if (!bd) return;
      try {
        const data = JSON.parse(event.data);
        bd.updateProgress(
          data.job_id,
          data.status,
          data.current,
          data.total,
          data.estimated_size_bytes,
        );
      } catch {
        // Ignore malformed events
      }
    };

    let logInvalidateTimeout: ReturnType<typeof setTimeout> | undefined;
    const handleLogEntry = () => {
      clearTimeout(logInvalidateTimeout);
      logInvalidateTimeout = setTimeout(() => {
        queryClient.invalidateQueries({ queryKey: [LogsQueryKey.ListLogs] });
      }, 500);
    };

    es.addEventListener("job.created", handleJobCreated);
    es.addEventListener("job.status_changed", handleJobStatusChanged);
    es.addEventListener("bulk_download.progress", handleBulkDownloadProgress);
    es.addEventListener("log.entry", handleLogEntry);

    return () => {
      clearTimeout(logInvalidateTimeout);
      es.removeEventListener("job.created", handleJobCreated);
      es.removeEventListener("job.status_changed", handleJobStatusChanged);
      es.removeEventListener(
        "bulk_download.progress",
        handleBulkDownloadProgress,
      );
      es.removeEventListener("log.entry", handleLogEntry);
      es.close();
    };
  }, [isAuthenticated, queryClient]);
}
