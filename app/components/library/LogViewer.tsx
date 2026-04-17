import { ChevronDown, ChevronRight } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

/**
 * Normalized log entry that both JobDetail and AdminLogs map into.
 */
export interface LogViewerEntry {
  id: number;
  level: string;
  timestamp: string;
  message: string;
  data?: Record<string, unknown> | null;
  error?: string | null;
  stackTrace?: string | null;
}

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
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")} ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}:${String(d.getSeconds()).padStart(2, "0")}.${String(d.getMilliseconds()).padStart(3, "0")}`;
  } catch {
    return ts;
  }
};

function highlightText(text: string, searchTerm?: string): React.ReactNode {
  if (!searchTerm) return text;
  try {
    const regex = new RegExp(
      `(${searchTerm.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")})`,
      "gi",
    );
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
  } catch {
    return text;
  }
}

interface LogRowProps {
  entry: LogViewerEntry;
  searchTerm?: string;
}

const LogRow = ({ entry, searchTerm }: LogRowProps) => {
  const [expanded, setExpanded] = useState(false);
  const hasExpandableContent =
    (entry.data && Object.keys(entry.data).length > 0) ||
    entry.error ||
    entry.stackTrace;

  // Create data preview for collapsed state
  const dataPreview =
    entry.data && Object.keys(entry.data).length > 0
      ? Object.entries(entry.data)
          .map(([k, v]) => `${k}=${JSON.stringify(v)}`)
          .join(" ")
      : null;

  return (
    <div className="py-1.5 px-3 border-b border-border/50 last:border-b-0 font-mono text-xs hover:bg-muted/50 transition-colors">
      <div
        className={`flex items-center gap-2 overflow-hidden ${hasExpandableContent ? "cursor-pointer" : ""}`}
        onClick={() => hasExpandableContent && setExpanded(!expanded)}
      >
        {hasExpandableContent ? (
          expanded ? (
            <ChevronDown className="h-3.5 w-3.5 text-muted-foreground flex-shrink-0" />
          ) : (
            <ChevronRight className="h-3.5 w-3.5 text-muted-foreground flex-shrink-0" />
          )
        ) : (
          <div className="w-3.5 flex-shrink-0" />
        )}
        <span className="text-muted-foreground flex-shrink-0 whitespace-nowrap tabular-nums">
          {formatTimestamp(entry.timestamp)}
        </span>
        <Badge
          className={`${levelColors[entry.level] ?? levelColors.info} text-[10px] px-1.5 py-0 flex-shrink-0 font-mono uppercase`}
          variant="secondary"
        >
          {entry.level.length > 4 ? entry.level.slice(0, 3) : entry.level}
        </Badge>
        <span className="text-foreground flex-shrink-0 whitespace-nowrap">
          {highlightText(entry.message, searchTerm)}
        </span>
        {dataPreview && !expanded && (
          <span className="text-muted-foreground truncate min-w-0">
            {highlightText(dataPreview, searchTerm)}
          </span>
        )}
      </div>
      {expanded && (
        <div className="ml-6 mt-2 space-y-2">
          {entry.data && Object.keys(entry.data).length > 0 && (
            <pre className="bg-muted p-2 rounded text-xs overflow-x-auto whitespace-pre-wrap">
              {highlightText(JSON.stringify(entry.data, null, 2), searchTerm)}
            </pre>
          )}
          {entry.error && (
            <div className="text-red-500 dark:text-red-400 text-xs">
              error: {highlightText(entry.error, searchTerm)}
            </div>
          )}
          {entry.stackTrace && (
            <pre className="bg-red-950/20 p-2 rounded text-xs overflow-x-auto text-red-400 whitespace-pre-wrap">
              {highlightText(entry.stackTrace, searchTerm)}
            </pre>
          )}
        </div>
      )}
    </div>
  );
};

interface LogViewerProps {
  entries: LogViewerEntry[];
  searchTerm?: string;
  emptyMessage?: string;
  className?: string;
}

const LogViewer = ({
  entries,
  searchTerm,
  emptyMessage = "No logs found.",
  className = "max-h-[500px]",
}: LogViewerProps) => {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const prevEntriesRef = useRef<LogViewerEntry[]>([]);

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

  return (
    <div className="relative">
      <div
        className={`overflow-y-auto border border-border rounded-md bg-muted/20 dark:bg-neutral-950/50 ${className}`}
        onScroll={handleScroll}
        ref={scrollRef}
      >
        {entries.length === 0 ? (
          <div className="flex items-center justify-center h-full min-h-[200px] text-muted-foreground text-sm">
            {emptyMessage}
          </div>
        ) : (
          entries.map((entry) => (
            <LogRow entry={entry} key={entry.id} searchTerm={searchTerm} />
          ))
        )}
      </div>

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
  );
};

export default LogViewer;
