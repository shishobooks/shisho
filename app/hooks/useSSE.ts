import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";

import { QueryKey as BooksQueryKey } from "@/hooks/queries/books";
import { QueryKey as JobsQueryKey } from "@/hooks/queries/jobs";
import { useAuth } from "@/hooks/useAuth";

export function useSSE() {
  const queryClient = useQueryClient();
  const { isAuthenticated } = useAuth();

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
        }
      } catch {
        // Ignore malformed events
      }
    };

    es.addEventListener("job.created", handleJobCreated);
    es.addEventListener("job.status_changed", handleJobStatusChanged);

    return () => {
      es.removeEventListener("job.created", handleJobCreated);
      es.removeEventListener("job.status_changed", handleJobStatusChanged);
      es.close();
    };
  }, [isAuthenticated, queryClient]);
}
