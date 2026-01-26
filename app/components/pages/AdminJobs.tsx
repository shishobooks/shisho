import { formatDistanceToNow } from "date-fns";
import { RefreshCw } from "lucide-react";
import { useCallback } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import { useCreateJob, useJobs } from "@/hooks/queries/jobs";
import { useAuth } from "@/hooks/useAuth";
import { usePageTitle } from "@/hooks/usePageTitle";
import { JobStatusInProgress, JobTypeScan, type Job } from "@/types";

const JOBS_PER_PAGE = 20;

const getStatusColor = (status: string) => {
  switch (status) {
    case "completed":
      return "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400";
    case "in_progress":
      return "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400";
    case "pending":
      return "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400";
    case "failed":
      return "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400";
    default:
      return "bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400";
  }
};

const formatDuration = (start: string, end?: string | null): string => {
  const startDate = new Date(start);
  const endDate = end ? new Date(end) : new Date();
  const durationMs = endDate.getTime() - startDate.getTime();

  if (durationMs < 1000) {
    return `${durationMs}ms`;
  }
  const seconds = Math.floor(durationMs / 1000);
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  return `${minutes}m ${remainingSeconds}s`;
};

interface JobRowProps {
  job: Job;
}

const JobRow = ({ job }: JobRowProps) => (
  <Link
    className="flex items-center justify-between py-3 md:py-4 px-4 md:px-6 hover:bg-muted/50 transition-colors"
    to={`/settings/jobs/${job.id}`}
  >
    <div className="flex-1 min-w-0">
      <div className="flex items-center gap-2 md:gap-3 flex-wrap">
        <span className="font-medium text-foreground text-sm md:text-base">
          {job.type}
        </span>
        <Badge className={getStatusColor(job.status)} variant="secondary">
          {job.status === JobStatusInProgress ? "running" : job.status}
        </Badge>
      </div>
      <div className="flex flex-col sm:flex-row sm:items-center gap-1 sm:gap-4 mt-1 text-xs md:text-sm text-muted-foreground">
        <span>Started {formatDistanceToNow(new Date(job.created_at))} ago</span>
        {job.status === JobStatusInProgress && job.created_at && (
          <span>Running for {formatDuration(job.created_at)}</span>
        )}
        {(job.status === "completed" || job.status === "failed") &&
          job.created_at &&
          job.updated_at && (
            <span>Took {formatDuration(job.created_at, job.updated_at)}</span>
          )}
      </div>
    </div>
  </Link>
);

const AdminJobs = () => {
  usePageTitle("Background Jobs");

  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);

  const { hasPermission } = useAuth();
  const { data, isLoading, error, refetch } = useJobs({
    limit: JOBS_PER_PAGE,
    offset: (currentPage - 1) * JOBS_PER_PAGE,
  });
  const createJobMutation = useCreateJob();

  const canCreateJobs = hasPermission("jobs", "write");
  const total = data?.total ?? 0;
  const totalPages = Math.ceil(total / JOBS_PER_PAGE);

  const handlePageChange = (page: number) => {
    const newSearchParams = new URLSearchParams(searchParams);
    newSearchParams.set("page", page.toString());
    setSearchParams(newSearchParams);
  };

  const getPageNumbers = () => {
    const pages = [];
    const showPages = 5;

    let start = Math.max(1, currentPage - Math.floor(showPages / 2));
    const end = Math.min(totalPages, start + showPages - 1);

    if (end - start + 1 < showPages) {
      start = Math.max(1, end - showPages + 1);
    }

    for (let i = start; i <= end; i++) {
      pages.push(i);
    }

    return pages;
  };

  const handleTriggerSync = useCallback(async () => {
    try {
      await createJobMutation.mutateAsync({
        payload: { type: JobTypeScan, data: {} },
      });
      toast.success("Library scan started");
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to start scan";
      toast.error(message);
    }
  }, [createJobMutation]);

  if (isLoading) {
    return <LoadingSpinner />;
  }

  if (error) {
    return (
      <div className="text-center">
        <h1 className="text-2xl font-semibold mb-4">Error Loading Jobs</h1>
        <p className="text-muted-foreground">{error.message}</p>
      </div>
    );
  }

  const jobs = data?.jobs ?? [];

  return (
    <div>
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6 md:mb-8">
        <div>
          <h1 className="text-xl md:text-2xl font-semibold mb-1 md:mb-2">
            Background Jobs
          </h1>
          <p className="text-sm md:text-base text-muted-foreground">
            View and manage background processing tasks.
          </p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <Button onClick={() => refetch()} size="sm" variant="outline">
            <RefreshCw className="h-4 w-4 sm:mr-2" />
            <span className="hidden sm:inline">Refresh</span>
          </Button>
          {canCreateJobs && (
            <Button
              disabled={createJobMutation.isPending}
              onClick={handleTriggerSync}
              size="sm"
            >
              <RefreshCw
                className={`h-4 w-4 sm:mr-2 ${createJobMutation.isPending ? "animate-spin" : ""}`}
              />
              <span className="hidden sm:inline">Trigger Scan</span>
            </Button>
          )}
        </div>
      </div>

      {jobs.length === 0 ? (
        <div className="border border-border rounded-md p-8 text-center">
          <p className="text-muted-foreground">No jobs found.</p>
        </div>
      ) : (
        <>
          <div className="mb-4 text-sm text-muted-foreground">
            Showing {(currentPage - 1) * JOBS_PER_PAGE + 1}-
            {Math.min(currentPage * JOBS_PER_PAGE, total)} of {total} jobs
          </div>

          <div className="border border-border rounded-md divide-y divide-border">
            {jobs.map((job) => (
              <JobRow job={job} key={job.id} />
            ))}
          </div>

          {totalPages > 1 && (
            <Pagination className="mt-6">
              <PaginationContent>
                <PaginationItem>
                  <PaginationPrevious
                    className={
                      currentPage <= 1
                        ? "pointer-events-none opacity-50"
                        : "cursor-pointer"
                    }
                    onClick={() => handlePageChange(currentPage - 1)}
                  />
                </PaginationItem>

                {getPageNumbers()[0] > 1 && (
                  <>
                    <PaginationItem>
                      <PaginationLink
                        className="cursor-pointer"
                        onClick={() => handlePageChange(1)}
                      >
                        1
                      </PaginationLink>
                    </PaginationItem>
                    {getPageNumbers()[0] > 2 && (
                      <PaginationItem>
                        <PaginationEllipsis />
                      </PaginationItem>
                    )}
                  </>
                )}

                {getPageNumbers().map((page) => (
                  <PaginationItem key={page}>
                    <PaginationLink
                      className="cursor-pointer"
                      isActive={page === currentPage}
                      onClick={() => handlePageChange(page)}
                    >
                      {page}
                    </PaginationLink>
                  </PaginationItem>
                ))}

                {getPageNumbers()[getPageNumbers().length - 1] < totalPages && (
                  <>
                    {getPageNumbers()[getPageNumbers().length - 1] <
                      totalPages - 1 && (
                      <PaginationItem>
                        <PaginationEllipsis />
                      </PaginationItem>
                    )}
                    <PaginationItem>
                      <PaginationLink
                        className="cursor-pointer"
                        onClick={() => handlePageChange(totalPages)}
                      >
                        {totalPages}
                      </PaginationLink>
                    </PaginationItem>
                  </>
                )}

                <PaginationItem>
                  <PaginationNext
                    className={
                      currentPage >= totalPages
                        ? "pointer-events-none opacity-50"
                        : "cursor-pointer"
                    }
                    onClick={() => handlePageChange(currentPage + 1)}
                  />
                </PaginationItem>
              </PaginationContent>
            </Pagination>
          )}
        </>
      )}
    </div>
  );
};

export default AdminJobs;
