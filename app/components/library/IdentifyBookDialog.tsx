import { Loader2, Search } from "lucide-react";
import { useEffect, useRef, useState } from "react";
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
import {
  usePluginEnrich,
  usePluginSearch,
  type PluginSearchResult,
} from "@/hooks/queries/plugins";
import { cn } from "@/libraries/utils";
import type { Book } from "@/types";

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
  const searchMutation = usePluginSearch();
  const enrichMutation = usePluginEnrich();
  const inputRef = useRef<HTMLInputElement>(null);
  const hasSearchedRef = useRef(false);

  // Pre-fill query and auto-search when dialog opens
  useEffect(() => {
    if (open) {
      setQuery(book.title);
      setSelectedResult(null);
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
    if (e.key === "Enter") {
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
            <div className="text-center py-12 text-muted-foreground">
              No results found. Try a different search query.
            </div>
          )}

          {searchMutation.isSuccess && results.length > 0 && (
            <div className="space-y-2">
              {results.map((result, index) => (
                <button
                  className={cn(
                    "w-full text-left rounded-lg border p-3 cursor-pointer transition-colors",
                    "hover:bg-muted/50",
                    selectedResult === result
                      ? "ring-2 ring-primary border-primary"
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
                          {result.plugin_id}
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

                      {result.identifiers && result.identifiers.length > 0 && (
                        <div className="flex flex-wrap gap-1 mt-1">
                          {result.identifiers.map((id) => (
                            <Badge
                              className="text-xs"
                              key={`${id.type}-${id.value}`}
                              variant="secondary"
                            >
                              {id.type}: {id.value}
                            </Badge>
                          ))}
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
