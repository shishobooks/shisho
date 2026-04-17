import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import LoadingSpinner from "@/components/library/LoadingSpinner";
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
import { useLogs, type LogEntry } from "@/hooks/queries/logs";
import { usePageTitle } from "@/hooks/usePageTitle";

const INITIAL_LIMIT = 200;

const levelColors: Record<string, string> = {
  debug: "bg-gray-100 text-gray-700 dark:bg-gray-800/50 dark:text-gray-400",
  info: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  warn: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
  error: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
  fatal: "bg-red-200 text-red-900 dark:bg-red-900/50 dark:text-red-300",
};

const formatTimestamp = (ts: string): string => {
  try {
    const d = new Date(ts);
    return d.toLocaleTimeString("en-US", {
      hour12: false,
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  } catch {
    return ts;
  }
};

interface LogRowProps {
  entry: LogEntry;
}

const LogRow = ({ entry }: LogRowProps) => {
  const [expanded, setExpanded] = useState(false);
  const hasExtra =
    (entry.data && Object.keys(entry.data).length > 0) || entry.error;

  return (
    <div
      className={`px-3 py-1.5 font-mono text-xs hover:bg-muted/50 transition-colors ${hasExtra ? "cursor-pointer" : ""}`}
      onClick={hasExtra ? () => setExpanded(!expanded) : undefined}
    >
      <div className="flex items-start gap-2">
        <span className="text-muted-foreground shrink-0 tabular-nums">
          {formatTimestamp(entry.timestamp)}
        </span>
        <Badge
          className={`${levelColors[entry.level] ?? levelColors.info} text-[10px] px-1.5 py-0 shrink-0 font-mono uppercase`}
          variant="secondary"
        >
          {entry.level.slice(0, 3)}
        </Badge>
        <span className="text-foreground break-all">{entry.message}</span>
      </div>
      {expanded && (
        <div className="mt-1 ml-[7.5rem] space-y-1">
          {entry.error && (
            <div className="text-red-500 dark:text-red-400">
              error: {entry.error}
            </div>
          )}
          {entry.data && Object.keys(entry.data).length > 0 && (
            <pre className="text-muted-foreground whitespace-pre-wrap break-all">
              {JSON.stringify(entry.data, null, 2)}
            </pre>
          )}
        </div>
      )}
    </div>
  );
};

const AdminLogs = () => {
  usePageTitle("Server Logs");

  const [level, setLevel] = useState<string>("");
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const scrollRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const prevEntriesRef = useRef<LogEntry[]>([]);

  // Debounce search input
  useEffect(() => {
    const timer = setTimeout(() => setSearch(searchInput), 300);
    return () => clearTimeout(timer);
  }, [searchInput]);

  const { data, isLoading } = useLogs({
    level: level || undefined,
    search: search || undefined,
    limit: INITIAL_LIMIT,
  });

  const entries = useMemo(() => data?.entries ?? [], [data]);

  // Auto-scroll to bottom when new entries arrive (if user is at the bottom)
  useEffect(() => {
    if (autoScroll && scrollRef.current && entries.length > 0) {
      if (entries !== prevEntriesRef.current) {
        scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
        prevEntriesRef.current = entries;
      }
    }
  }, [entries, autoScroll]);

  const handleScroll = useCallback(() => {
    if (!scrollRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = scrollRef.current;
    setAutoScroll(scrollHeight - scrollTop - clientHeight < 50);
  }, []);

  const jumpToLatest = useCallback(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
      setAutoScroll(true);
    }
  }, []);

  if (isLoading) {
    return <LoadingSpinner />;
  }

  return (
    <div className="flex flex-col h-[calc(100vh-12rem)]">
      <div className="mb-4">
        <h1 className="text-xl md:text-2xl font-semibold mb-1 md:mb-2">
          Server Logs
        </h1>
        <p className="text-sm md:text-base text-muted-foreground">
          Real-time server logs from the current session.
        </p>
      </div>

      {/* Filter bar */}
      <div className="flex items-center gap-3 mb-4">
        <Select
          onValueChange={(v) => setLevel(v === "all" ? "" : v)}
          value={level || "all"}
        >
          <SelectTrigger className="w-32">
            <SelectValue placeholder="All levels" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All levels</SelectItem>
            <SelectItem value="debug">Debug</SelectItem>
            <SelectItem value="info">Info</SelectItem>
            <SelectItem value="warn">Warn</SelectItem>
            <SelectItem value="error">Error</SelectItem>
          </SelectContent>
        </Select>
        <Input
          className="max-w-xs"
          onChange={(e) => setSearchInput(e.target.value)}
          placeholder="Search messages..."
          value={searchInput}
        />
      </div>

      {/* Log list */}
      <div className="relative flex-1 min-h-0">
        <div
          className="absolute inset-0 overflow-y-auto border border-border rounded-md bg-muted/20 dark:bg-neutral-950/50"
          onScroll={handleScroll}
          ref={scrollRef}
        >
          {entries.length === 0 ? (
            <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
              {search || level ? "No logs matching filters." : "No logs yet."}
            </div>
          ) : (
            <div className="divide-y divide-border/50">
              {entries.map((entry) => (
                <LogRow entry={entry} key={entry.id} />
              ))}
            </div>
          )}
        </div>

        {/* Jump to latest button */}
        {!autoScroll && entries.length > 0 && (
          <div className="absolute bottom-4 left-1/2 -translate-x-1/2">
            <Button
              className="shadow-lg"
              onClick={jumpToLatest}
              size="sm"
              variant="secondary"
            >
              Jump to latest
            </Button>
          </div>
        )}
      </div>
    </div>
  );
};

export default AdminLogs;
