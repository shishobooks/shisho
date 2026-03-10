import { ExternalLink, Info, Loader2, Search } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  usePluginEnrich,
  usePluginIdentifierTypes,
  usePluginSearch,
  type PluginSearchResult,
} from "@/hooks/queries/plugins";
import { cn } from "@/libraries/utils";
import type { Book } from "@/types";
import {
  formatDuration,
  formatFileSize,
  formatIdentifierType,
  formatMetadataFieldLabel,
  getFilename,
} from "@/utils/format";
import { getIdentifierUrl } from "@/utils/identifiers";

interface IdentifyBookDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  book: Book;
}

export function IdentifyBookDialog({
  open,
  onOpenChange,
  book,
}: IdentifyBookDialogProps) {
  const [query, setQuery] = useState("");
  const [selectedResult, setSelectedResult] =
    useState<PluginSearchResult | null>(null);
  const [selectedFileId, setSelectedFileId] = useState<number | undefined>(
    undefined,
  );
  const searchMutation = usePluginSearch();
  const enrichMutation = usePluginEnrich();
  const { data: pluginIdentifierTypes } = usePluginIdentifierTypes();
  const inputRef = useRef<HTMLInputElement>(null);
  const hasSearchedRef = useRef(false);

  const mainFiles = useMemo(
    () => book.files?.filter((f) => f.file_role === "main") ?? [],
    [book.files],
  );
  const hasMultipleFiles = mainFiles.length > 1;

  // Pre-fill query and auto-search when dialog opens
  useEffect(() => {
    if (open) {
      setQuery(book.title);
      setSelectedResult(null);
      setSelectedFileId(undefined);
      hasSearchedRef.current = false;
    }
  }, [open, book.title]);

  // Auto-search after query is set from dialog open
  useEffect(() => {
    if (open && query && !hasSearchedRef.current) {
      hasSearchedRef.current = true;
      searchMutation.mutate({ query, bookId: book.id });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, query]);

  const handleSearch = () => {
    if (!query.trim()) return;
    setSelectedResult(null);
    searchMutation.mutate({ query: query.trim(), bookId: book.id });
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !searchMutation.isPending) {
      handleSearch();
    }
  };

  const handleApply = () => {
    if (!selectedResult) return;
    enrichMutation.mutate(
      {
        pluginScope: selectedResult.plugin_scope,
        pluginId: selectedResult.plugin_id,
        bookId: book.id,
        fileId: selectedFileId,
        providerData: selectedResult.provider_data,
      },
      {
        onSuccess: () => {
          toast.success("Book metadata updated successfully");
          onOpenChange(false);
        },
        onError: (error) => {
          toast.error(error.message || "Failed to apply metadata");
        },
      },
    );
  };

  const results = searchMutation.data?.results ?? [];

  // Detect plugin IDs that appear under multiple scopes
  const ambiguousIds = useMemo(() => {
    const items = searchMutation.data?.results ?? [];
    const scopesByPluginId = new Map<string, Set<string>>();
    for (const r of items) {
      const scopes = scopesByPluginId.get(r.plugin_id) ?? new Set();
      scopes.add(r.plugin_scope);
      scopesByPluginId.set(r.plugin_id, scopes);
    }
    const ids = new Set<string>();
    for (const [id, scopes] of scopesByPluginId) {
      if (scopes.size > 1) ids.add(id);
    }
    return ids;
  }, [searchMutation.data?.results]);

  const pluginLabel = (result: PluginSearchResult) =>
    ambiguousIds.has(result.plugin_id)
      ? `${result.plugin_scope}/${result.plugin_id}`
      : result.plugin_id;

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-3xl max-h-[90vh] overflow-y-auto overflow-x-hidden">
        <DialogHeader className="pr-8">
          <DialogTitle>Identify Book</DialogTitle>
          <DialogDescription>
            Search for this book across metadata providers and apply the correct
            match.
          </DialogDescription>
        </DialogHeader>

        {/* Search bar */}
        <div className="flex gap-2">
          <Input
            className="flex-1"
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Search by title, author, ISBN..."
            ref={inputRef}
            value={query}
          />
          <Button
            disabled={searchMutation.isPending || !query.trim()}
            onClick={handleSearch}
            variant="outline"
          >
            {searchMutation.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Search className="h-4 w-4" />
            )}
          </Button>
        </div>

        {/* Results */}
        <div className="min-h-[200px] max-h-[60vh] overflow-y-auto">
          {searchMutation.isPending && (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          )}

          {searchMutation.isSuccess && results.length === 0 && (
            <div className="text-center py-12 text-muted-foreground space-y-2">
              <p>No results found.</p>
              <p className="text-xs">
                Make sure you have a metadata enricher plugin installed, or try
                a different search query.
              </p>
            </div>
          )}

          {searchMutation.isSuccess && results.length > 0 && (
            <div className="space-y-2">
              {results.map((result, index) => (
                <button
                  className={cn(
                    "w-full text-left rounded-lg border-2 p-3 cursor-pointer transition-colors",
                    "hover:bg-muted/50",
                    selectedResult === result
                      ? "border-primary bg-primary/5"
                      : "border-border",
                  )}
                  key={`${result.plugin_scope}-${result.plugin_id}-${index}`}
                  onClick={() => setSelectedResult(result)}
                  type="button"
                >
                  <div className="flex gap-3">
                    {/* Cover thumbnail */}
                    {result.image_url ? (
                      <img
                        alt=""
                        className="w-16 h-24 object-cover rounded shrink-0 bg-muted"
                        src={result.image_url}
                      />
                    ) : (
                      <div className="w-16 h-24 rounded shrink-0 bg-muted flex items-center justify-center text-muted-foreground text-xs">
                        No cover
                      </div>
                    )}

                    {/* Details */}
                    <div className="flex-1 min-w-0 space-y-1">
                      <div className="flex items-start justify-between gap-2">
                        <p className="font-medium leading-tight">
                          {result.title}
                        </p>
                        <Badge className="shrink-0 text-xs" variant="outline">
                          {pluginLabel(result)}
                        </Badge>
                      </div>

                      {result.authors && result.authors.length > 0 && (
                        <p className="text-sm text-muted-foreground">
                          {result.authors.join(", ")}
                        </p>
                      )}

                      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                        {result.release_date && (
                          <span>{result.release_date}</span>
                        )}
                        {result.publisher && (
                          <>
                            {result.release_date && (
                              <span className="text-muted-foreground/50">
                                ·
                              </span>
                            )}
                            <span>{result.publisher}</span>
                          </>
                        )}
                      </div>

                      {result.identifiers &&
                        result.identifiers.filter((id) => id.type && id.value)
                          .length > 0 && (
                          <div className="flex flex-wrap gap-1 mt-1">
                            {result.identifiers
                              .filter((id) => id.type && id.value)
                              .map((id) => {
                                const url = getIdentifierUrl(
                                  id.type,
                                  id.value,
                                  pluginIdentifierTypes,
                                );
                                return url ? (
                                  <a
                                    className="inline-flex"
                                    href={url}
                                    key={`${id.type}-${id.value}`}
                                    onClick={(e) => e.stopPropagation()}
                                    rel="noopener noreferrer"
                                    target="_blank"
                                  >
                                    <Badge
                                      className="text-xs hover:bg-primary/20 transition-colors"
                                      variant="secondary"
                                    >
                                      {formatIdentifierType(
                                        id.type,
                                        pluginIdentifierTypes,
                                      )}
                                      : {id.value}
                                      <ExternalLink className="h-3 w-3 ml-1 shrink-0" />
                                    </Badge>
                                  </a>
                                ) : (
                                  <Badge
                                    className="text-xs"
                                    key={`${id.type}-${id.value}`}
                                    variant="secondary"
                                  >
                                    {formatIdentifierType(
                                      id.type,
                                      pluginIdentifierTypes,
                                    )}
                                    : {id.value}
                                  </Badge>
                                );
                              })}
                          </div>
                        )}

                      {result.description && (
                        <p className="text-xs text-muted-foreground line-clamp-2 mt-1">
                          {result.description}
                        </p>
                      )}
                    </div>
                  </div>
                </button>
              ))}
            </div>
          )}

          {searchMutation.isError && (
            <div className="text-center py-12 text-destructive">
              Search failed. Please try again.
            </div>
          )}
        </div>

        {selectedResult?.disabled_fields &&
          selectedResult.disabled_fields.length > 0 && (
            <div className="flex items-start gap-2 rounded-md border border-border bg-muted/50 p-3 text-sm text-muted-foreground">
              <Info className="h-4 w-4 mt-0.5 shrink-0" />
              <span>
                The following fields are disabled for this plugin and won&apos;t
                be updated:{" "}
                {selectedResult.disabled_fields
                  .map((f) => formatMetadataFieldLabel(f))
                  .sort()
                  .join(", ")}
                . You can change this in the plugin settings.
              </span>
            </div>
          )}

        {hasMultipleFiles && (
          <div className="space-y-2">
            <div>
              <Label>Apply to file</Label>
              <p className="mt-1 text-xs text-muted-foreground">
                Identifiers and cover image will be applied to the selected
                file.
              </p>
            </div>
            <div className="space-y-1.5">
              {mainFiles.map((file) => (
                <button
                  className={cn(
                    "w-full text-left rounded-md border p-2.5 cursor-pointer transition-colors",
                    "hover:bg-muted/50",
                    selectedFileId === file.id ||
                      (selectedFileId === undefined &&
                        file.id === mainFiles[0].id)
                      ? "border-primary bg-primary/5"
                      : "border-border",
                  )}
                  key={file.id}
                  onClick={() => setSelectedFileId(file.id)}
                  type="button"
                >
                  <div className="flex items-center gap-2">
                    <Badge className="shrink-0 text-xs" variant="outline">
                      {file.file_type.toUpperCase()}
                    </Badge>
                    <span className="text-sm truncate min-w-0">
                      {file.name || getFilename(file.filepath)}
                    </span>
                  </div>
                  <div className="flex items-center gap-x-2 mt-1 text-xs text-muted-foreground">
                    <span>{formatFileSize(file.filesize_bytes)}</span>
                    {file.audiobook_duration_seconds != null && (
                      <>
                        <span className="text-muted-foreground/50">·</span>
                        <span>
                          {formatDuration(file.audiobook_duration_seconds)}
                        </span>
                      </>
                    )}
                    {file.page_count != null && (
                      <>
                        <span className="text-muted-foreground/50">·</span>
                        <span>
                          {file.page_count} page
                          {file.page_count !== 1 ? "s" : ""}
                        </span>
                      </>
                    )}
                  </div>
                </button>
              ))}
            </div>
          </div>
        )}

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={!selectedResult || enrichMutation.isPending}
            onClick={handleApply}
          >
            {enrichMutation.isPending && (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            )}
            Apply
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
