import { useEffect, useMemo, useState } from "react";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import LogViewer, { type LogViewerEntry } from "@/components/library/LogViewer";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useLogs } from "@/hooks/queries/logs";
import { usePageTitle } from "@/hooks/usePageTitle";

const INITIAL_LIMIT = 200;

const AdminLogs = () => {
  usePageTitle("Server Logs");

  const [level, setLevel] = useState<string>("");
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");

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

  const entries: LogViewerEntry[] = useMemo(
    () =>
      (data?.entries ?? []).map((e) => ({
        id: e.id,
        level: e.level,
        timestamp: e.timestamp,
        message: e.message,
        data: e.data ?? null,
        error: e.error ?? null,
        stackTrace: null,
      })),
    [data],
  );

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

      {/* Log viewer */}
      <LogViewer
        className="flex-1"
        emptyMessage={
          search || level ? "No logs matching filters." : "No logs yet."
        }
        entries={entries}
        searchTerm={search}
      />
    </div>
  );
};

export default AdminLogs;
