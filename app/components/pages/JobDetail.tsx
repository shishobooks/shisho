import { formatDistanceToNow } from "date-fns";
import { ScrollText } from "lucide-react";
import { useMemo, useState } from "react";
import { useParams } from "react-router-dom";

import EmptyState, { NoMatchesState } from "@/components/library/EmptyState";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import LogViewer from "@/components/library/LogViewer";
import QueryError from "@/components/library/QueryError";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useJob, useJobLogs } from "@/hooks/queries/jobs";
import { usePageTitle } from "@/hooks/usePageTitle";
import { isLoadFailure } from "@/libraries/api";
import {
  JobStatusInProgress,
  LogLevelError,
  LogLevelFatal,
  LogLevelInfo,
  LogLevelWarn,
} from "@/types";
import { formatElapsed } from "@/utils/format";
import { logLevelColor } from "@/utils/logLevels";

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
      return "bg-muted text-muted-foreground";
  }
};

const JobDetail = () => {
  const { id } = useParams<{ id: string }>();

  usePageTitle(id ? `Job #${id}` : "Job Details");

  const [searchTerm, setSearchTerm] = useState("");
  const [levelFilter, setLevelFilter] = useState<string[]>([]);
  const [pluginFilter, setPluginFilter] = useState<string>("");

  const jobQuery = useJob(id);
  const job = jobQuery.data;

  const logsQuery = useJobLogs(id, {
    level: levelFilter.length > 0 ? levelFilter : undefined,
    plugin: pluginFilter || undefined,
  });
  const logsData = logsQuery.data;

  const isLoading = jobQuery.isLoading || logsQuery.isLoading;
  const logs = useMemo(() => logsData?.items ?? [], [logsData?.items]);

  // A log line's plugin is its own field, not part of its data. The chosen
  // plugin stays listed even when other filters leave none of its lines, so
  // the filter can still be cleared.
  const pluginNames = useMemo(() => {
    const names = new Set<string>(pluginFilter ? [pluginFilter] : []);
    for (const log of logs) {
      if (log.plugin) {
        names.add(log.plugin);
      }
    }
    return Array.from(names).sort();
  }, [logs, pluginFilter]);

  // Filter and map logs to LogViewerEntry format (memoized to avoid
  // re-parsing JSON on every render).
  const viewerEntries = useMemo(() => {
    const searchLower = searchTerm.toLowerCase();
    return logs
      .filter((log) => {
        if (searchTerm) {
          if (
            !log.message.toLowerCase().includes(searchLower) &&
            !log.data?.toLowerCase().includes(searchLower) &&
            !log.plugin?.toLowerCase().includes(searchLower)
          ) {
            return false;
          }
        }
        return true;
      })
      .map((log) => {
        let parsedData: Record<string, unknown> | null = null;
        if (log.data) {
          try {
            parsedData = JSON.parse(log.data);
          } catch {
            parsedData = null;
          }
        }
        // Merged into the data so LogViewer shows the plugin in the row and
        // in the expanded details.
        if (log.plugin) {
          parsedData = { plugin: log.plugin, ...parsedData };
        }
        return {
          id: log.id,
          level: log.level,
          timestamp: log.created_at,
          message: log.message,
          data: parsedData,
          error: null,
          stackTrace: log.stack_trace ?? null,
        };
      });
  }, [logs, searchTerm]);

  const toggleLevelFilter = (level: string) => {
    setLevelFilter((prev) =>
      prev.includes(level) ? prev.filter((l) => l !== level) : [...prev, level],
    );
  };

  if (isLoading) {
    return <LoadingSpinner />;
  }

  if (isLoadFailure(jobQuery)) {
    return <QueryError fallback="Failed to load job" query={jobQuery} />;
  }

  if (!job) {
    return (
      <div className="text-center">
        <h1 className="text-2xl font-semibold mb-4">Job Not Found</h1>
      </div>
    );
  }

  return (
    <div>
      {/* Header */}
      <div className="mb-6">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-semibold">Job #{job.id}</h1>
          <Badge className={getStatusColor(job.status)} variant="secondary">
            {job.status === JobStatusInProgress ? "running" : job.status}
          </Badge>
        </div>
        <div className="flex items-center gap-4 mt-2 text-sm text-muted-foreground">
          <span>Type: {job.type}</span>
          <span>
            Started{" "}
            {formatDistanceToNow(new Date(job.created_at), { addSuffix: true })}
          </span>
          {job.status === JobStatusInProgress && (
            <span>Running for {formatElapsed(job.created_at)}</span>
          )}
          {(job.status === "completed" || job.status === "failed") &&
            job.updated_at && (
              <span>Took {formatElapsed(job.created_at, job.updated_at)}</span>
            )}
          {job.process_id && <span>Process: {job.process_id}</span>}
        </div>
      </div>

      {/* Toolbar */}
      <div className="flex items-center gap-4 mb-4">
        <Input
          aria-label="Search logs"
          className="max-w-xs"
          onChange={(e) => setSearchTerm(e.target.value)}
          placeholder="Search logs..."
          type="text"
          value={searchTerm}
        />
        <div
          aria-label="Log levels"
          className="flex items-center gap-2"
          role="group"
        >
          {[LogLevelInfo, LogLevelWarn, LogLevelError, LogLevelFatal].map(
            (level) => (
              <Button
                aria-pressed={levelFilter.includes(level)}
                className={
                  levelFilter.includes(level) ? logLevelColor(level) : ""
                }
                key={level}
                onClick={() => toggleLevelFilter(level)}
                size="sm"
                variant={levelFilter.includes(level) ? "secondary" : "outline"}
              >
                {level}
              </Button>
            ),
          )}
        </div>
        {pluginNames.length > 0 && (
          <Select
            onValueChange={(value) =>
              setPluginFilter(value === "all" ? "" : value)
            }
            value={pluginFilter || "all"}
          >
            <SelectTrigger aria-label="Plugin" className="w-[180px]">
              <SelectValue placeholder="All Plugins" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Plugins</SelectItem>
              {pluginNames.map((name) => (
                <SelectItem key={name} value={name}>
                  {name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </div>

      {/* Log viewer */}
      {logsQuery.error && !logsData ? (
        <QueryError fallback="Failed to load job logs" query={logsQuery} />
      ) : (
        <LogViewer
          emptyState={
            searchTerm || levelFilter.length > 0 || pluginFilter ? (
              <NoMatchesState bare itemLabel="log messages" />
            ) : (
              <EmptyState
                bare
                description="Messages this job logs show up here as it runs."
                icon={ScrollText}
                title="No log messages yet"
              />
            )
          }
          entries={viewerEntries}
          searchTerm={searchTerm}
        />
      )}
    </div>
  );
};

export default JobDetail;
