import { AlertTriangle, ExternalLink, Loader2, Search, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormDialog } from "@/components/ui/form-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { getLanguageName } from "@/constants/languages";
import {
  usePluginIdentifierTypes,
  usePluginOrder,
  usePluginSearch,
  type PluginSearchResult,
} from "@/hooks/queries/plugins";
import { cn } from "@/libraries/utils";
import { PluginHookMetadataEnricher, type Book } from "@/types";
import { getAuthorRoleLabel } from "@/utils/authorRoles";
import {
  formatDate,
  formatDuration,
  formatFileSize,
  formatIdentifierType,
  getFilename,
} from "@/utils/format";
import { getIdentifierUrl } from "@/utils/identifiers";
import { formatSeriesNumber } from "@/utils/seriesNumber";

import { computeIdentifyEmptyState } from "./identify-utils";
import { IdentifyReviewForm } from "./IdentifyReviewForm";

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
  const [step, setStep] = useState<"search" | "review">("search");
  const [query, setQuery] = useState("");
  const [author, setAuthor] = useState("");
  const [identifiers, setIdentifiers] = useState<
    Array<{ type: string; value: string }>
  >([]);
  const [selectedResult, setSelectedResult] =
    useState<PluginSearchResult | null>(null);
  const [selectedFileId, setSelectedFileId] = useState<number | undefined>(
    undefined,
  );
  const [reviewHasChanges, setReviewHasChanges] = useState(false);
  const searchMutation = usePluginSearch();
  const { data: pluginIdentifierTypes } = usePluginIdentifierTypes();
  const { data: enricherPlugins } = usePluginOrder(PluginHookMetadataEnricher);
  const hasEnricherPlugins = (enricherPlugins?.length ?? 0) > 0;
  const inputRef = useRef<HTMLInputElement>(null);
  const hasSearchedRef = useRef(false);
  const queryUserTouched = useRef(false);

  const mainFiles = useMemo(
    () => book.files?.filter((f) => f.file_role === "main") ?? [],
    [book.files],
  );
  const hasMultipleFiles = mainFiles.length > 1;

  const selectedFile = selectedFileId
    ? mainFiles.find((f) => f.id === selectedFileId)
    : mainFiles[0];
  const isAudiobook = selectedFile?.file_type === "m4b";

  // Pre-fill form and auto-search when dialog opens
  useEffect(() => {
    if (open) {
      setStep("search");
      setSelectedResult(null);
      hasSearchedRef.current = false;
      queryUserTouched.current = false;

      const initialQuery = book.title;
      const initialAuthor = book.authors?.[0]?.person?.name ?? "";
      const initialFileId = mainFiles.length > 1 ? mainFiles[0].id : undefined;
      const initialFile = mainFiles[0];
      const initialIds = (initialFile?.identifiers ?? []).map((id) => ({
        type: id.type,
        value: id.value,
      }));

      setQuery(initialQuery);
      setAuthor(initialAuthor);
      setIdentifiers(initialIds);
      setSelectedFileId(initialFileId);

      if (initialQuery) {
        hasSearchedRef.current = true;
        searchMutation.mutate({
          query: initialQuery,
          bookId: book.id,
          fileId: initialFileId,
          author: initialAuthor || undefined,
          identifiers: initialIds.length > 0 ? initialIds : undefined,
        });
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, book.title, book.authors, book.files, mainFiles]);

  const handleSearch = () => {
    if (!query.trim()) return;
    setSelectedResult(null);
    searchMutation.mutate({
      query: query.trim(),
      bookId: book.id,
      fileId: selectedFileId,
      author: author.trim() || undefined,
      identifiers: identifiers.length > 0 ? identifiers : undefined,
    });
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !searchMutation.isPending) {
      handleSearch();
    }
  };

  const results = searchMutation.data?.results ?? [];
  const pluginErrors = searchMutation.data?.errors ?? [];
  const skippedPlugins = searchMutation.data?.skipped_plugins ?? [];
  const totalPlugins = searchMutation.data?.total_plugins ?? 0;
  const selectedFileType = selectedFile?.file_type;

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

  const resolveAuthors = (result: PluginSearchResult): string | undefined => {
    if (!result.authors || result.authors.length === 0) return undefined;
    return result.authors
      .map((a) => {
        const label = getAuthorRoleLabel(a.role);
        return label ? `${a.name} (${label})` : a.name;
      })
      .join(", ");
  };

  const handleSelectResult = (result: PluginSearchResult) => {
    setSelectedResult(result);
    setStep("review");
  };

  return (
    <FormDialog
      hasChanges={step === "review" && reviewHasChanges}
      onOpenChange={onOpenChange}
      open={open}
    >
      <DialogContent className="max-w-2xl overflow-x-hidden [&>*]:min-w-0">
        <DialogHeader className="pr-8">
          <DialogTitle>Identify Book</DialogTitle>
          <DialogDescription>
            Search for this book across metadata providers and apply the correct
            match.
          </DialogDescription>
        </DialogHeader>

        {/* File selector — visible in both search and review steps */}
        {hasMultipleFiles && (
          <div className="space-y-2">
            <div>
              <Label>Apply to file</Label>
              <p className="mt-1 text-xs text-muted-foreground">
                File-specific metadata (identifiers, cover, narrators,
                publisher, etc.) will be applied to the selected file.
              </p>
            </div>
            <div className="space-y-1.5">
              {mainFiles.map((file) => (
                <button
                  className={cn(
                    "w-full text-left rounded-md border p-2.5 cursor-pointer transition-colors",
                    "hover:bg-muted/50",
                    selectedFileId === file.id
                      ? "border-primary bg-primary/5"
                      : "border-border",
                  )}
                  key={file.id}
                  onClick={() => {
                    setSelectedFileId(file.id);
                    // Update identifiers to the selected file's identifiers
                    const fileIds = (file.identifiers ?? []).map((id) => ({
                      type: id.type,
                      value: id.value,
                    }));
                    setIdentifiers(fileIds);
                    if (!queryUserTouched.current) {
                      const fileTitle = file.name || getFilename(file.filepath);
                      setQuery(fileTitle);
                      setSelectedResult(null);
                      searchMutation.reset();
                      searchMutation.mutate({
                        query: fileTitle,
                        bookId: book.id,
                        fileId: file.id,
                        author: author.trim() || undefined,
                        identifiers: fileIds.length > 0 ? fileIds : undefined,
                      });
                    }
                  }}
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

        {/* Step 1: Search */}
        {step === "search" && (
          <>
            {/* Search bar */}
            <div className="flex gap-2">
              <Input
                className="flex-1"
                onChange={(e) => {
                  setQuery(e.target.value);
                  queryUserTouched.current = true;
                }}
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

            {/* Author and identifier filters */}
            <div className="space-y-3">
              <div className="space-y-1">
                <Label className="text-xs text-muted-foreground">Author</Label>
                <Input
                  className="h-8 text-sm"
                  onChange={(e) => setAuthor(e.target.value)}
                  onKeyDown={handleKeyDown}
                  placeholder="Author name (optional)"
                  value={author}
                />
              </div>
              {identifiers.length > 0 && (
                <div className="space-y-1">
                  <Label className="text-xs text-muted-foreground">
                    Identifiers
                  </Label>
                  <div className="flex flex-wrap gap-1.5">
                    {identifiers.map((id, i) => (
                      <Badge
                        className="max-w-full gap-1 pr-1"
                        key={`${id.type}-${id.value}-${i}`}
                        variant="secondary"
                      >
                        <span
                          className="truncate"
                          title={`${id.type}:${id.value}`}
                        >
                          {formatIdentifierType(id.type, pluginIdentifierTypes)}
                          : {id.value}
                        </span>
                        <button
                          className="shrink-0 rounded-sm hover:bg-muted-foreground/20 p-0.5 cursor-pointer"
                          onClick={() =>
                            setIdentifiers(
                              identifiers.filter((_, j) => j !== i),
                            )
                          }
                          type="button"
                        >
                          <X className="h-3 w-3" />
                        </button>
                      </Badge>
                    ))}
                  </div>
                </div>
              )}
            </div>

            {/* Results */}
            <div className="min-h-[200px] max-h-[60vh] overflow-y-auto">
              {searchMutation.isPending && (
                <div className="flex items-center justify-center py-12">
                  <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                </div>
              )}

              {searchMutation.isSuccess && pluginErrors.length > 0 && (
                <div className="mb-3 space-y-2">
                  {pluginErrors.map((err) => (
                    <div
                      className="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/5 p-2.5 text-xs"
                      key={`${err.plugin_scope}-${err.plugin_id}`}
                    >
                      <AlertTriangle className="h-4 w-4 shrink-0 text-destructive mt-0.5" />
                      <div className="min-w-0 flex-1">
                        <p className="font-medium text-destructive">
                          {err.plugin_name || err.plugin_id} failed
                        </p>
                        <p
                          className="text-muted-foreground break-words"
                          title={err.message}
                        >
                          {err.message}
                        </p>
                      </div>
                    </div>
                  ))}
                </div>
              )}

              {searchMutation.isSuccess &&
                results.length === 0 &&
                pluginErrors.length === 0 &&
                (() => {
                  const message = computeIdentifyEmptyState({
                    hasEnricherPlugins,
                    totalPlugins,
                    skippedPlugins,
                    fileType: selectedFileType,
                  });
                  return (
                    <div className="text-center py-12 text-muted-foreground space-y-2">
                      <p>No results found.</p>
                      <p className="text-xs">{message.primary}</p>
                      {message.secondary && (
                        <p className="text-xs">{message.secondary}</p>
                      )}
                    </div>
                  );
                })()}

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
                      onClick={() => handleSelectResult(result)}
                      type="button"
                    >
                      <div className="flex gap-3">
                        {/* Cover thumbnail */}
                        <ResultCoverThumbnail
                          coverPage={result.cover_page}
                          coverUrl={result.cover_url}
                          isAudiobook={isAudiobook}
                          previewFileId={selectedFileId ?? selectedFile?.id}
                          previewFileType={selectedFileType}
                          previewPageCount={selectedFile?.page_count}
                        />

                        {/* Details */}
                        <div className="flex-1 min-w-0">
                          {/* Zone 1: Identity */}
                          <div>
                            {/* Title + subtitle + badges */}
                            <div className="flex items-start justify-between gap-4">
                              <div className="min-w-0 flex-1">
                                <p className="font-medium leading-tight">
                                  {result.title}
                                </p>
                                {result.subtitle && (
                                  <p className="text-sm text-muted-foreground/80 leading-tight mt-0.5">
                                    {result.subtitle}
                                  </p>
                                )}
                                {result.series && (
                                  <p className="text-xs text-muted-foreground font-medium mt-0.5">
                                    {result.series}
                                    {result.series_number != null &&
                                      ` ${formatSeriesNumber(result.series_number, result.series_number_unit, selectedFileType)}`}
                                  </p>
                                )}
                              </div>
                              <div className="flex items-center gap-1 shrink-0">
                                {result.confidence != null && (
                                  <Badge
                                    className={cn(
                                      "text-xs",
                                      result.confidence >= 0.9
                                        ? "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400"
                                        : result.confidence >= 0.7
                                          ? "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400"
                                          : "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400",
                                    )}
                                    variant="secondary"
                                  >
                                    {Math.round(result.confidence * 100)}%
                                  </Badge>
                                )}
                                <Badge className="text-xs" variant="outline">
                                  {pluginLabel(result)}
                                </Badge>
                              </div>
                            </div>

                            {/* People */}
                            {(() => {
                              const authors = resolveAuthors(result);
                              const narrators = result.narrators;
                              const hasAuthors = authors && authors.length > 0;
                              const hasNarrators =
                                narrators && narrators.length > 0;
                              return hasAuthors || hasNarrators ? (
                                <div className="mt-2 space-y-0.5">
                                  {hasAuthors && (
                                    <p className="text-sm text-muted-foreground">
                                      {authors}
                                    </p>
                                  )}
                                  {hasNarrators && (
                                    <p className="text-xs text-muted-foreground">
                                      Narrated by {narrators.join(", ")}
                                    </p>
                                  )}
                                </div>
                              ) : null;
                            })()}

                            {/* Date + publisher + language + abridged */}
                            {(() => {
                              const metaItems: string[] = [];
                              if (result.release_date) {
                                metaItems.push(formatDate(result.release_date));
                              }
                              if (result.publisher) {
                                metaItems.push(result.publisher);
                              }
                              if (result.language) {
                                metaItems.push(
                                  getLanguageName(result.language) ||
                                    result.language,
                                );
                              }
                              if (result.abridged != null) {
                                metaItems.push(
                                  result.abridged ? "Abridged" : "Unabridged",
                                );
                              }
                              return metaItems.length > 0 ? (
                                <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground/80 mt-2">
                                  {metaItems.map((item, i) => (
                                    <span
                                      className="flex items-center gap-x-2"
                                      key={`${i}-${item}`}
                                    >
                                      {i > 0 && (
                                        <span className="text-muted-foreground/50">
                                          ·
                                        </span>
                                      )}
                                      <span>{item}</span>
                                    </span>
                                  ))}
                                </div>
                              ) : null;
                            })()}
                          </div>

                          {/* Zone 2: Identifiers */}
                          {result.identifiers &&
                            result.identifiers.filter(
                              (id) => id.type && id.value,
                            ).length > 0 && (
                              <div className="flex flex-wrap gap-1 mt-2.5">
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

                          {/* Zone 3: Taxonomy */}
                          {(() => {
                            const genres = result.genres ?? [];
                            const tags = result.tags ?? [];
                            return genres.length > 0 || tags.length > 0 ? (
                              <div className="mt-2.5 space-y-1">
                                {genres.length > 0 && (
                                  <div className="flex flex-wrap items-center gap-1">
                                    <span className="text-[0.65rem] uppercase tracking-wide font-medium text-muted-foreground/50 mr-1">
                                      Genres
                                    </span>
                                    {genres.map((g) => (
                                      <Badge
                                        className="text-xs"
                                        key={`genre-${g}`}
                                        variant="outline"
                                      >
                                        {g}
                                      </Badge>
                                    ))}
                                  </div>
                                )}
                                {tags.length > 0 && (
                                  <div className="flex flex-wrap items-center gap-1">
                                    <span className="text-[0.65rem] uppercase tracking-wide font-medium text-muted-foreground/50 mr-1">
                                      Tags
                                    </span>
                                    {tags.map((tag) => (
                                      <Badge
                                        className="text-xs"
                                        key={`tag-${tag}`}
                                        variant="outline"
                                      >
                                        {tag}
                                      </Badge>
                                    ))}
                                  </div>
                                )}
                              </div>
                            ) : null;
                          })()}

                          {/* Zone 4: Description */}
                          {result.description && (
                            <p className="text-xs text-muted-foreground line-clamp-3 whitespace-pre-line mt-2.5">
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
            </DialogFooter>
          </>
        )}

        {/* Step 2: Review */}
        {step === "review" && selectedResult && (
          <IdentifyReviewForm
            book={book}
            fileId={selectedFileId}
            onBack={() => {
              setReviewHasChanges(false);
              setStep("search");
            }}
            onClose={() => onOpenChange(false)}
            onHasChangesChange={setReviewHasChanges}
            result={selectedResult}
          />
        )}
      </DialogContent>
    </FormDialog>
  );
}

/** Cover thumbnail for a single search result. Falls back to a "No cover"
 * placeholder when the plugin's cover_url 404s/errors instead of rendering a
 * broken-image icon. */
function ResultCoverThumbnail({
  coverUrl,
  coverPage,
  isAudiobook,
  previewFileId,
  previewFileType,
  previewPageCount,
}: {
  coverUrl?: string;
  coverPage?: number | null;
  isAudiobook: boolean;
  previewFileId?: number;
  previewFileType?: string;
  previewPageCount?: number | null;
}) {
  const [imgError, setImgError] = useState(false);
  // Reset on URL change so a previous broken result doesn't latch the
  // placeholder when results swap in.
  useEffect(() => {
    setImgError(false);
  }, [coverUrl, coverPage, previewFileId]);

  const thumbClass = cn(
    "w-16 object-cover rounded shrink-0 bg-muted",
    isAudiobook ? "h-16" : "h-24",
  );
  const placeholder = (
    <div
      className={cn(
        "w-16 rounded shrink-0 bg-muted flex items-center justify-center text-muted-foreground text-xs",
        isAudiobook ? "h-16" : "h-24",
      )}
    >
      No cover
    </div>
  );

  if (coverUrl && !imgError) {
    return (
      <img
        alt=""
        className={thumbClass}
        onError={() => setImgError(true)}
        src={coverUrl}
      />
    );
  }
  const coverPageInRange =
    coverPage != null &&
    coverPage >= 0 &&
    (previewPageCount == null || coverPage < previewPageCount);
  if (
    !coverUrl &&
    !imgError &&
    coverPageInRange &&
    previewFileId &&
    (previewFileType === "cbz" || previewFileType === "pdf")
  ) {
    return (
      <img
        alt=""
        className={thumbClass}
        onError={() => setImgError(true)}
        src={`/api/books/files/${previewFileId}/page/${coverPage}`}
      />
    );
  }
  return placeholder;
}
