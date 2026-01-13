import { formatDistanceToNow } from "date-fns";
import { ArrowLeft, ChevronDown, ChevronRight } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { useJobLogs } from "@/hooks/queries/jobs";
import {
  JobLogLevelError,
  JobLogLevelFatal,
  JobLogLevelInfo,
  JobLogLevelWarn,
  JobStatusInProgress,
  JobStatusPending,
  type JobLog,
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
      return "bg-gray-500/20 text-gray-400";
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

interface LogEntryProps {
  log: JobLog;
  searchTerm: string;
}

const LogEntry = ({ log, searchTerm }: LogEntryProps) => {
  const [expanded, setExpanded] = useState(false);
  const hasExpandableContent = log.data || log.stack_trace;

  const date = new Date(log.created_at);
  const timestamp = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")} ${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}:${String(date.getSeconds()).padStart(2, "0")}.${String(date.getMilliseconds()).padStart(3, "0")}`;

  // Parse data if present
  let parsedData: Record<string, unknown> | null = null;
  if (log.data) {
    try {
      parsedData = JSON.parse(log.data);
    } catch {
      parsedData = null;
    }
  }

  // Create data preview
  const dataPreview = parsedData
    ? Object.entries(parsedData)
        .map(([k, v]) => `${k}=${JSON.stringify(v)}`)
        .join(" ")
    : null;

  // Highlight search term in message
  const highlightText = (text: string) => {
    if (!searchTerm) return text;
    const regex = new RegExp(`(${searchTerm})`, "gi");
    const parts = text.split(regex);
    return parts.map((part, i) =>
      regex.test(part) ? (
        <mark className="bg-yellow-300 dark:bg-yellow-700" key={i}>
          {part}
        </mark>
      ) : (
        part
      ),
    );
  };

  return (
    <div className="py-2 border-b border-border last:border-b-0 font-mono text-sm">
      <div
        className={`flex items-center gap-2 overflow-hidden ${hasExpandableContent ? "cursor-pointer" : ""}`}
        onClick={() => hasExpandableContent && setExpanded(!expanded)}
      >
        {hasExpandableContent ? (
          expanded ? (
            <ChevronDown className="h-4 w-4 text-muted-foreground flex-shrink-0" />
          ) : (
            <ChevronRight className="h-4 w-4 text-muted-foreground flex-shrink-0" />
          )
        ) : (
          <div className="w-4 flex-shrink-0" />
        )}
        <span className="text-muted-foreground flex-shrink-0 whitespace-nowrap">
          {timestamp}
        </span>
        <Badge
          className={`${getLevelColor(log.level)} flex-shrink-0`}
          variant="secondary"
        >
          {log.level}
        </Badge>
        <span className="text-foreground flex-shrink-0 whitespace-nowrap">
          {highlightText(log.message)}
        </span>
        {dataPreview && !expanded && (
          <span className="text-muted-foreground truncate min-w-0">
            {highlightText(dataPreview)}
          </span>
        )}
      </div>
      {expanded && (
        <div className="ml-6 mt-2 space-y-2">
          {parsedData && (
            <pre className="bg-muted p-2 rounded text-xs overflow-x-auto whitespace-pre-wrap">
              {highlightText(JSON.stringify(parsedData, null, 2))}
            </pre>
          )}
          {log.stack_trace && (
            <pre className="bg-red-950/20 p-2 rounded text-xs overflow-x-auto text-red-400 whitespace-pre-wrap">
              {highlightText(log.stack_trace)}
            </pre>
          )}
        </div>
      )}
    </div>
  );
};

const JobDetail = () => {
  const { id } = useParams<{ id: string }>();
  const [searchTerm, setSearchTerm] = useState("");
  const [levelFilter, setLevelFilter] = useState<string[]>([]);
  const [autoScroll, setAutoScroll] = useState(true);
  const logContainerRef = useRef<HTMLDivElement>(null);
  const lastLogIdRef = useRef<number | undefined>(undefined);

  const { data, isLoading, error, refetch } = useJobLogs(id, {
    level: levelFilter.length > 0 ? levelFilter : undefined,
  });

  const job = data?.job;
  const logs = data?.logs ?? [];

  // Filter logs by search term (client-side)
  const filteredLogs = logs.filter((log) => {
    if (!searchTerm) return true;
    const searchLower = searchTerm.toLowerCase();
    if (log.message.toLowerCase().includes(searchLower)) return true;
    if (log.data?.toLowerCase().includes(searchLower)) return true;
    return false;
  });

  // Polling for live updates
  useEffect(() => {
    if (
      job?.status === JobStatusPending ||
      job?.status === JobStatusInProgress
    ) {
      const interval = setInterval(refetch, 2000);
      return () => clearInterval(interval);
    }
  }, [job?.status, refetch]);

  // Auto-scroll to bottom when new logs arrive
  useEffect(() => {
    if (autoScroll && logContainerRef.current && filteredLogs.length > 0) {
      const lastLog = filteredLogs[filteredLogs.length - 1];
      if (lastLog.id !== lastLogIdRef.current) {
        lastLogIdRef.current = lastLog.id;
        logContainerRef.current.scrollTop =
          logContainerRef.current.scrollHeight;
      }
    }
  }, [filteredLogs, autoScroll]);

  // Disable auto-scroll when user scrolls up
  const handleScroll = useCallback(() => {
    if (logContainerRef.current) {
      const { scrollTop, scrollHeight, clientHeight } = logContainerRef.current;
      const isAtBottom = scrollHeight - scrollTop - clientHeight < 50;
      if (!isAtBottom && autoScroll) {
        setAutoScroll(false);
      }
    }
  }, [autoScroll]);

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
        <Link
          className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground mb-4"
          to="/settings/jobs"
        >
          <ArrowLeft className="h-4 w-4 mr-1" />
          Back to Jobs
        </Link>
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
      </div>

      {/* Log container */}
      <div
        className="border border-border rounded-md bg-background overflow-y-auto max-h-[500px]"
        onScroll={handleScroll}
        ref={logContainerRef}
      >
        <div className="p-4">
          {filteredLogs.length === 0 ? (
            <p className="text-muted-foreground text-center py-8">
              No logs found.
            </p>
          ) : (
            filteredLogs.map((log) => (
              <LogEntry key={log.id} log={log} searchTerm={searchTerm} />
            ))
          )}
        </div>
      </div>

      {/* Auto-scroll checkbox */}
      <div className="flex items-center gap-2 mt-4">
        <Checkbox
          checked={autoScroll}
          id="autoscroll"
          onCheckedChange={(checked) => {
            setAutoScroll(checked as boolean);
            if (checked && logContainerRef.current) {
              logContainerRef.current.scrollTop =
                logContainerRef.current.scrollHeight;
            }
          }}
        />
        <label className="text-sm text-muted-foreground" htmlFor="autoscroll">
          Auto-scroll to new logs
        </label>
      </div>
    </div>
  );
};

export default JobDetail;
