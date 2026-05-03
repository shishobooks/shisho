import { formatDistanceToNow } from "date-fns";
import { useMemo, useState } from "react";
import { useParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import LogViewer from "@/components/library/LogViewer";
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
import { useJobLogs } from "@/hooks/queries/jobs";
import { usePageTitle } from "@/hooks/usePageTitle";
import {
  JobLogLevelError,
  JobLogLevelFatal,
  JobLogLevelInfo,
  JobLogLevelWarn,
  JobStatusInProgress,
} from "@/types";

const getLevelColor = (level: string) => {
  switch (level) {
    case JobLogLevelInfo:
      return "bg-blue-500/20 text-blue-400";
    case JobLogLevelWarn:
      return "bg-yellow-500/20 text-yellow-400";
    case JobLogLevelError:
      return "bg-red-500/20 text-red-400";
    case JobLogLevelFatal:
      return "bg-red-700/30 text-red-300";
    default:
      return "bg-muted text-muted-foreground";
  }
};

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

const JobDetail = () => {
  const { id } = useParams<{ id: string }>();

  usePageTitle(id ? `Job #${id}` : "Job Details");

  const [searchTerm, setSearchTerm] = useState("");
  const [levelFilter, setLevelFilter] = useState<string[]>([]);
  const [pluginFilter, setPluginFilter] = useState<string>("");

  const { data, isLoading, error } = useJobLogs(id, {
    level: levelFilter.length > 0 ? levelFilter : undefined,
    plugin: pluginFilter || undefined,
  });

  const job = data?.job;
  const logs = useMemo(() => data?.logs ?? [], [data?.logs]);

  // Extract unique plugin names from log data
  const pluginNames = useMemo(() => {
    const names = new Set<string>();
    for (const log of logs) {
      if (log.data) {
        try {
          const parsed = JSON.parse(log.data);
          if (parsed.plugin) {
            names.add(parsed.plugin);
          }
        } catch {
          /* ignore parse errors */
        }
      }
    }
    return Array.from(names).sort();
  }, [logs]);

  // Filter and map logs to LogViewerEntry format (memoized to avoid
  // re-parsing JSON on every render).
  const viewerEntries = useMemo(() => {
    const searchLower = searchTerm.toLowerCase();
    return logs
      .filter((log) => {
        if (searchTerm) {
          if (
            !log.message.toLowerCase().includes(searchLower) &&
            !log.data?.toLowerCase().includes(searchLower)
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

  if (error) {
    return (
      <div className="text-center">
        <h1 className="text-2xl font-semibold mb-4">Error Loading Job</h1>
        <p className="text-muted-foreground">{error.message}</p>
      </div>
    );
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
            Started {formatDistanceToNow(new Date(job.created_at))} ago
          </span>
          {job.status === JobStatusInProgress && (
            <span>Running for {formatDuration(job.created_at)}</span>
          )}
          {(job.status === "completed" || job.status === "failed") &&
            job.updated_at && (
              <span>Took {formatDuration(job.created_at, job.updated_at)}</span>
            )}
          {job.process_id && <span>Process: {job.process_id}</span>}
        </div>
      </div>

      {/* Toolbar */}
      <div className="flex items-center gap-4 mb-4">
        <Input
          className="max-w-xs"
          onChange={(e) => setSearchTerm(e.target.value)}
          placeholder="Search logs..."
          type="text"
          value={searchTerm}
        />
        <div className="flex items-center gap-2">
          {[
            JobLogLevelInfo,
            JobLogLevelWarn,
            JobLogLevelError,
            JobLogLevelFatal,
          ].map((level) => (
            <Button
              className={
                levelFilter.includes(level) ? getLevelColor(level) : ""
              }
              key={level}
              onClick={() => toggleLevelFilter(level)}
              size="sm"
              variant={levelFilter.includes(level) ? "secondary" : "outline"}
            >
              {level}
            </Button>
          ))}
        </div>
        {pluginNames.length > 0 && (
          <Select
            onValueChange={(value) =>
              setPluginFilter(value === "all" ? "" : value)
            }
            value={pluginFilter || "all"}
          >
            <SelectTrigger className="w-[180px]">
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
      <LogViewer
        emptyMessage="No logs found."
        entries={viewerEntries}
        searchTerm={searchTerm}
      />
    </div>
  );
};

export default JobDetail;
