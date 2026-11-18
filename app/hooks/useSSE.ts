import { useQueryClient } from "@tanstack/react-query";
import { useContext, useEffect, useLayoutEffect, useRef } from "react";

import {
  BulkDownloadContext,
  type BulkDownloadContextValue,
} from "@/contexts/BulkDownload";
import { QueryKey as BooksQueryKey } from "@/hooks/queries/books";
import { invalidateCoverPreviewLists } from "@/hooks/queries/coverPreviews";
import { QueryKey as JobsQueryKey } from "@/hooks/queries/jobs";
import { QueryKey as LibrariesQueryKey } from "@/hooks/queries/libraries";
import { QueryKey as LogsQueryKey } from "@/hooks/queries/logs";
import { invalidateReadingRecords } from "@/hooks/queries/reading";
import { applyScanProgress, markScanEnded } from "@/hooks/queries/scanProgress";
import { useAuth } from "@/hooks/useAuth";
import { checkServerBuild } from "@/libraries/build";
import type { JobScanProgress, ListBooksQuery } from "@/types";

// The least time between two gallery refetches for a library while a scan
// imports into it. Progress events come about once a second; each refetch
// reloads the page of Books on screen.
const SCAN_REFETCH_INTERVAL_MS = 3000;

export function useSSE() {
  const queryClient = useQueryClient();
  const { demoMode, isAuthenticated } = useAuth();
  const bulkDownload = useContext(BulkDownloadContext);

  // Use ref to avoid the EventSource reconnecting on every progress update.
  // Synced via useLayoutEffect so it's always up-to-date before any async
  // callbacks fire.
  const bulkDownloadRef = useRef<BulkDownloadContextValue | null>(null);
  useLayoutEffect(() => {
    bulkDownloadRef.current = bulkDownload;
  });

  useEffect(() => {
    // A Share Link page is for recipients, so it never opens the stream,
    // even for a signed-in user previewing their own link. The page is only
    // reached by a full page load, so the pathname at mount is enough.
    const { pathname } = window.location;
    const onSharePage = pathname === "/share" || pathname.startsWith("/share/");
    if (!isAuthenticated || demoMode || onSharePage) {
      return;
    }

    const es = new EventSource("/api/events");

    // The stream drops when the server restarts, as it does for an upgrade,
    // and the browser reopens it once the server is back.
    es.onopen = () => {
      void checkServerBuild();
    };

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

        // A scan that has ended leaves none of its libraries scanning.
        if (
          data.type === "scan" &&
          (data.status === "completed" || data.status === "failed")
        ) {
          markScanEnded(queryClient, data.job_id);
        }

        // When a scan completes, invalidate book queries so lists refresh
        if (data.status === "completed" && data.type === "scan") {
          queryClient.invalidateQueries({
            queryKey: [BooksQueryKey.ListBooks],
          });
          // A scan can delete books, whose Read-throughs stay on Reading
          // History without them.
          invalidateReadingRecords(queryClient);
          // A scan can add books and change covers on list entries too.
          invalidateCoverPreviewLists(queryClient);
          // Also invalidate library languages in case scanned files introduced new languages
          queryClient.invalidateQueries({
            queryKey: [LibrariesQueryKey.LibraryLanguages],
          });
        }

        // When a recompute_review job completes, refresh book lists and
        // detail views so the new reviewed states show up.
        if (data.status === "completed" && data.type === "recompute_review") {
          queryClient.invalidateQueries({
            queryKey: [BooksQueryKey.ListBooks],
          });
          queryClient.invalidateQueries({
            queryKey: [BooksQueryKey.RetrieveBook],
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

    // Books appear in a library's gallery as a scan imports them: a progress
    // event that imported Books (or, in a Scan for new metadata, scanned any)
    // refetches the library's Books, at most once per
    // SCAN_REFETCH_INTERVAL_MS, with a trailing refetch so the last Books of
    // a burst show too. The last event for the library always refetches.
    const lastBooksRefetch = new Map<number, number>();
    const lastProgressCount = new Map<string, number>();
    const pendingBooksRefetch = new Map<
      number,
      ReturnType<typeof setTimeout>
    >();
    const refetchLibraryBooks = (libraryId: number) => {
      lastBooksRefetch.set(libraryId, Date.now());
      void queryClient.invalidateQueries({
        predicate: (query) =>
          query.queryKey[0] === BooksQueryKey.ListBooks &&
          (query.queryKey[1] as ListBooksQuery | undefined)?.library_id ===
            libraryId,
      });
    };
    const handleScanProgress = (event: MessageEvent) => {
      let progress: JobScanProgress;
      try {
        progress = JSON.parse(event.data);
      } catch {
        return;
      }
      applyScanProgress(queryClient, progress);

      const libraryId = progress.library_id;
      const countKey = `${progress.job_id}:${libraryId}`;
      const count = progress.scan_for_new_metadata
        ? progress.files_scanned
        : progress.books_imported;
      const changed = count > (lastProgressCount.get(countKey) ?? 0);
      lastProgressCount.set(countKey, count);
      if (!changed && !progress.done) return;

      const sinceLast = Date.now() - (lastBooksRefetch.get(libraryId) ?? 0);
      if (progress.done || sinceLast >= SCAN_REFETCH_INTERVAL_MS) {
        clearTimeout(pendingBooksRefetch.get(libraryId));
        pendingBooksRefetch.delete(libraryId);
        refetchLibraryBooks(libraryId);
      } else if (!pendingBooksRefetch.has(libraryId)) {
        pendingBooksRefetch.set(
          libraryId,
          setTimeout(() => {
            pendingBooksRefetch.delete(libraryId);
            refetchLibraryBooks(libraryId);
          }, SCAN_REFETCH_INTERVAL_MS - sinceLast),
        );
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
    es.addEventListener("scan.progress", handleScanProgress);

    return () => {
      clearTimeout(logInvalidateTimeout);
      pendingBooksRefetch.forEach((timeout) => clearTimeout(timeout));
      es.removeEventListener("job.created", handleJobCreated);
      es.removeEventListener("job.status_changed", handleJobStatusChanged);
      es.removeEventListener(
        "bulk_download.progress",
        handleBulkDownloadProgress,
      );
      es.removeEventListener("log.entry", handleLogEntry);
      es.removeEventListener("scan.progress", handleScanProgress);
      es.close();
    };
  }, [demoMode, isAuthenticated, queryClient]);
}
