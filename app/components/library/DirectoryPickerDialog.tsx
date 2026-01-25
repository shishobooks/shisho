import {
  ChevronRight,
  File,
  Folder,
  FolderOpen,
  Loader2,
  Search,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { useFilesystemBrowse } from "@/hooks/queries/filesystem";
import type { Entry } from "@/types";

interface DirectoryPickerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (paths: string[]) => void;
  initialPath?: string;
}

const DEBOUNCE_MS = 300;

const DirectoryPickerDialog = ({
  open,
  onOpenChange,
  onSelect,
  initialPath = "/",
}: DirectoryPickerDialogProps) => {
  const [currentPath, setCurrentPath] = useState(initialPath);
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [showHidden, setShowHidden] = useState(false);
  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(new Set());
  const [offset, setOffset] = useState(0);
  const [accumulatedEntries, setAccumulatedEntries] = useState<Entry[]>([]);
  const searchInputRef = useRef<HTMLInputElement>(null);

  // Focus search input when navigating to a new directory.
  useEffect(() => {
    // Small delay to ensure the input is rendered and ready.
    const timer = setTimeout(() => {
      searchInputRef.current?.focus();
    }, 50);
    return () => clearTimeout(timer);
  }, [currentPath]);

  // Debounce search input.
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(search);
    }, DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [search]);

  // Reset when path changes or search/hidden toggles.
  useEffect(() => {
    setOffset(0);
    setAccumulatedEntries([]);
  }, [currentPath, debouncedSearch, showHidden]);

  // Reset when dialog opens.
  useEffect(() => {
    if (open) {
      setCurrentPath(initialPath);
      setSearch("");
      setDebouncedSearch("");
      setSelectedPaths(new Set());
      setOffset(0);
      setAccumulatedEntries([]);
    }
  }, [open, initialPath]);

  const browseQuery = useFilesystemBrowse(
    {
      path: currentPath,
      search: debouncedSearch,
      show_hidden: showHidden,
      limit: 50,
      offset,
    },
    {
      enabled: open,
    },
  );

  // Accumulate entries for "load more" functionality.
  // Use dataUpdatedAt to detect when new data actually arrives.
  useEffect(() => {
    if (browseQuery.data?.entries) {
      if (offset === 0) {
        setAccumulatedEntries(browseQuery.data.entries);
      } else {
        setAccumulatedEntries((prev) => [...prev, ...browseQuery.data.entries]);
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [browseQuery.dataUpdatedAt]);

  // Repopulate from cache if entries were cleared but cached data exists.
  // This handles the case when opening the dialog with a previously-browsed path.
  useEffect(() => {
    if (
      accumulatedEntries.length === 0 &&
      browseQuery.data?.entries &&
      browseQuery.data.entries.length > 0 &&
      offset === 0 &&
      !browseQuery.isFetching
    ) {
      setAccumulatedEntries(browseQuery.data.entries);
    }
  }, [
    accumulatedEntries.length,
    browseQuery.data?.entries,
    browseQuery.isFetching,
    offset,
  ]);

  const pathSegments = useMemo(() => {
    const path = browseQuery.data?.current_path || currentPath;
    if (path === "/") return [{ name: "/", path: "/" }];

    const segments = path.split("/").filter(Boolean);
    const result = [{ name: "/", path: "/" }];
    let currentSegmentPath = "";

    for (const segment of segments) {
      currentSegmentPath += "/" + segment;
      result.push({ name: segment, path: currentSegmentPath });
    }

    return result;
  }, [browseQuery.data?.current_path, currentPath]);

  const handleNavigate = useCallback((path: string) => {
    setCurrentPath(path);
    setSearch("");
    setDebouncedSearch("");
  }, []);

  const handleEntryClick = useCallback((entry: Entry) => {
    if (entry.is_dir) {
      setCurrentPath(entry.path);
      setSearch("");
      setDebouncedSearch("");
    }
  }, []);

  const handleToggleSelect = useCallback((path: string) => {
    setSelectedPaths((prev) => {
      const next = new Set(prev);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  }, []);

  const handleSelectCurrentDirectory = useCallback(() => {
    const path = browseQuery.data?.current_path || currentPath;
    onSelect([path]);
    onOpenChange(false);
  }, [browseQuery.data?.current_path, currentPath, onSelect, onOpenChange]);

  const handleConfirmSelection = useCallback(() => {
    onSelect(Array.from(selectedPaths));
    onOpenChange(false);
  }, [selectedPaths, onSelect, onOpenChange]);

  const handleLoadMore = useCallback(() => {
    setOffset((prev) => prev + 50);
  }, []);

  const hasMore = browseQuery.data?.has_more ?? false;
  const total = browseQuery.data?.total ?? 0;
  const remaining = total - accumulatedEntries.length;

  const directories = accumulatedEntries.filter((e) => e.is_dir);
  const files = accumulatedEntries.filter((e) => !e.is_dir);

  const handleSearchKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === "Enter" && directories.length === 1) {
        e.preventDefault();
        handleEntryClick(directories[0]);
      }
    },
    [directories, handleEntryClick],
  );

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-2xl max-h-[80vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>Select Directories</DialogTitle>
        </DialogHeader>

        {/* Breadcrumb navigation */}
        <div className="flex items-center gap-1 text-sm flex-wrap">
          {pathSegments.map((segment, index) => (
            <div className="flex items-center" key={segment.path}>
              {index > 0 && (
                <ChevronRight className="h-4 w-4 text-muted-foreground mx-1" />
              )}
              <button
                className="hover:text-primary hover:underline transition-colors px-1 py-0.5 rounded cursor-pointer"
                onClick={() => handleNavigate(segment.path)}
                type="button"
              >
                {segment.name}
              </button>
            </div>
          ))}
        </div>

        {/* Search and show hidden toggle */}
        <div className="flex items-center gap-4">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              className="pl-9"
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={handleSearchKeyDown}
              placeholder="Search..."
              ref={searchInputRef}
              value={search}
            />
          </div>
          <label className="flex items-center gap-2 text-sm whitespace-nowrap cursor-pointer">
            <Checkbox
              checked={showHidden}
              onCheckedChange={(checked) => setShowHidden(checked as boolean)}
            />
            Show hidden files
          </label>
        </div>

        {/* Directory listing */}
        <div className="h-[400px] border rounded-md overflow-y-auto">
          {browseQuery.isLoading && offset === 0 ? (
            <div className="flex items-center justify-center h-full py-12">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : browseQuery.isError ? (
            <div className="flex items-center justify-center h-full py-12 text-destructive">
              {browseQuery.error?.message || "Failed to load directory"}
            </div>
          ) : accumulatedEntries.length === 0 ? (
            <div className="flex items-center justify-center h-full py-12 text-muted-foreground">
              No entries found
            </div>
          ) : (
            <div className="p-2">
              {/* Directories */}
              {directories.map((entry) => (
                <div
                  className="flex items-center gap-3 px-3 py-2 rounded-md hover:bg-muted cursor-pointer group"
                  key={entry.path}
                  onClick={() => handleEntryClick(entry)}
                >
                  <Checkbox
                    checked={selectedPaths.has(entry.path)}
                    onCheckedChange={() => handleToggleSelect(entry.path)}
                    onClick={(e) => e.stopPropagation()}
                  />
                  {selectedPaths.has(entry.path) ? (
                    <FolderOpen className="h-5 w-5 text-primary" />
                  ) : (
                    <Folder className="h-5 w-5 text-muted-foreground group-hover:text-primary" />
                  )}
                  <span className="flex-1 truncate">{entry.name}</span>
                  <ChevronRight className="h-4 w-4 text-muted-foreground opacity-0 group-hover:opacity-100" />
                </div>
              ))}

              {/* Files (visually muted, not selectable) */}
              {files.map((entry) => (
                <div
                  className="flex items-center gap-3 px-3 py-2 rounded-md opacity-50"
                  key={entry.path}
                >
                  <div className="w-4" /> {/* Spacer for checkbox alignment */}
                  <File className="h-5 w-5 text-muted-foreground" />
                  <span className="flex-1 truncate">{entry.name}</span>
                </div>
              ))}

              {/* Load more */}
              {hasMore && (
                <div className="mt-2 pt-2 border-t">
                  <Button
                    className="w-full"
                    disabled={browseQuery.isFetching}
                    onClick={handleLoadMore}
                    variant="ghost"
                  >
                    {browseQuery.isFetching && offset > 0 ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Loading...
                      </>
                    ) : (
                      `Load more (${remaining} remaining)`
                    )}
                  </Button>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Selected paths summary */}
        {selectedPaths.size > 0 && (
          <div className="text-sm text-muted-foreground truncate">
            Selected: {Array.from(selectedPaths).join(", ")}
          </div>
        )}

        <DialogFooter className="gap-2 sm:gap-0">
          <Button onClick={handleSelectCurrentDirectory} variant="outline">
            Select current directory
          </Button>
          <div className="flex-1" />
          <Button onClick={() => onOpenChange(false)} variant="ghost">
            Cancel
          </Button>
          <Button
            disabled={selectedPaths.size === 0}
            onClick={handleConfirmSelection}
          >
            Select{" "}
            {selectedPaths.size > 0
              ? `${selectedPaths.size} folder${selectedPaths.size > 1 ? "s" : ""}`
              : "folders"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default DirectoryPickerDialog;
