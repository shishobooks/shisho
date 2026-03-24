import { ArrowLeft, ChevronDown, ChevronUp, Loader2, X } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  usePluginApply,
  type PluginSearchResult,
} from "@/hooks/queries/plugins";
import { cn } from "@/libraries/utils";
import type { Book, File } from "@/types";
import { formatMetadataFieldLabel } from "@/utils/format";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface IdentifyReviewFormProps {
  result: PluginSearchResult;
  book: Book;
  fileId?: number;
  onBack: () => void;
  onClose: () => void;
}

type FieldStatus = "unchanged" | "changed" | "new";

interface AuthorEntry {
  name: string;
  role?: string;
}

interface IdentifierEntry {
  type: string;
  value: string;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Determine field status and default value for a scalar field. */
function resolveScalar(
  current: string | undefined | null,
  incoming: string | undefined | null,
): { value: string; status: FieldStatus } {
  const cur = current?.trim() ?? "";
  const inc = incoming?.trim() ?? "";

  if (!cur && inc) return { value: inc, status: "new" };
  if (cur && !inc) return { value: cur, status: "unchanged" };
  if (cur === inc) return { value: cur, status: "unchanged" };
  // Both populated, values differ => use plugin value
  return { value: inc, status: "changed" };
}

/** Determine field status and default value for an array field. */
function resolveArray(
  current: string[],
  incoming: string[],
): { value: string[]; status: FieldStatus } {
  if (current.length === 0 && incoming.length > 0)
    return { value: incoming, status: "new" };
  if (current.length > 0 && incoming.length === 0)
    return { value: current, status: "unchanged" };
  const curSorted = [...current].sort();
  const incSorted = [...incoming].sort();
  if (
    curSorted.length === incSorted.length &&
    curSorted.every((v, i) => v === incSorted[i])
  ) {
    return { value: current, status: "unchanged" };
  }
  return { value: incoming, status: "changed" };
}

function resolveAuthors(
  current: AuthorEntry[],
  incoming: AuthorEntry[],
): { value: AuthorEntry[]; status: FieldStatus } {
  if (current.length === 0 && incoming.length > 0)
    return { value: incoming, status: "new" };
  if (current.length > 0 && incoming.length === 0)
    return { value: current, status: "unchanged" };
  const key = (a: AuthorEntry) => `${a.name}|${a.role ?? ""}`;
  const curKeys = current.map(key).sort();
  const incKeys = incoming.map(key).sort();
  if (
    curKeys.length === incKeys.length &&
    curKeys.every((v, i) => v === incKeys[i])
  ) {
    return { value: current, status: "unchanged" };
  }
  return { value: incoming, status: "changed" };
}

function resolveIdentifiers(
  current: IdentifierEntry[],
  incoming: IdentifierEntry[],
): { value: IdentifierEntry[]; status: FieldStatus } {
  if (current.length === 0 && incoming.length > 0)
    return { value: incoming, status: "new" };
  if (current.length > 0 && incoming.length === 0)
    return { value: current, status: "unchanged" };
  const key = (id: IdentifierEntry) => `${id.type}:${id.value}`;
  const curKeys = current.map(key).sort();
  const incKeys = incoming.map(key).sort();
  if (
    curKeys.length === incKeys.length &&
    curKeys.every((v, i) => v === incKeys[i])
  ) {
    return { value: current, status: "unchanged" };
  }
  return { value: incoming, status: "changed" };
}

/** Extract current file from book. */
function findFile(book: Book, fileId?: number): File | undefined {
  if (!fileId) return book.files?.[0];
  return book.files?.find((f) => f.id === fileId);
}

// ---------------------------------------------------------------------------
// Sub-components (inline, single-use)
// ---------------------------------------------------------------------------

function StatusBadge({ status }: { status: FieldStatus }) {
  if (status === "new") {
    return (
      <Badge
        className="text-[0.65rem] bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-400 border-transparent"
        variant="outline"
      >
        New
      </Badge>
    );
  }
  if (status === "changed") {
    return (
      <Badge
        className="text-[0.65rem] bg-primary/10 text-primary dark:bg-primary/20 border-transparent"
        variant="outline"
      >
        Changed
      </Badge>
    );
  }
  return (
    <Badge className="text-[0.65rem]" variant="subtle">
      Unchanged
    </Badge>
  );
}

function CurrentBar({
  children,
  onUseCurrent,
}: {
  children: React.ReactNode;
  onUseCurrent?: () => void;
}) {
  return (
    <div className="flex items-center justify-between gap-2 border-l-2 border-muted-foreground/30 bg-muted/50 rounded-r-md px-3 py-1.5 text-sm text-muted-foreground">
      <span className="min-w-0 truncate">{children}</span>
      {onUseCurrent && (
        <Button
          className="shrink-0 text-xs h-6 px-2"
          onClick={onUseCurrent}
          size="sm"
          type="button"
          variant="ghost"
        >
          Use current
        </Button>
      )}
    </div>
  );
}

function CollapsibleCurrentBar({
  text,
  onUseCurrent,
}: {
  text: string;
  onUseCurrent?: () => void;
}) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="border-l-2 border-muted-foreground/30 bg-muted/50 rounded-r-md px-3 py-1.5 text-sm text-muted-foreground">
      <div className="flex items-start justify-between gap-2">
        <p
          className={cn(
            "whitespace-pre-line break-words min-w-0",
            !expanded && "line-clamp-3",
          )}
        >
          {text}
        </p>
        <div className="flex items-center gap-1 shrink-0">
          {onUseCurrent && (
            <Button
              className="text-xs h-6 px-2"
              onClick={onUseCurrent}
              size="sm"
              type="button"
              variant="ghost"
            >
              Use current
            </Button>
          )}
          <Button
            className="text-xs h-6 w-6 p-0"
            onClick={() => setExpanded(!expanded)}
            size="sm"
            type="button"
            variant="ghost"
          >
            {expanded ? (
              <ChevronUp className="h-3.5 w-3.5" />
            ) : (
              <ChevronDown className="h-3.5 w-3.5" />
            )}
          </Button>
        </div>
      </div>
    </div>
  );
}

function FieldWrapper({
  field,
  status,
  children,
  currentValue,
  onUseCurrent,
  disabled,
}: {
  field: string;
  status: FieldStatus;
  children: React.ReactNode;
  currentValue?: React.ReactNode;
  onUseCurrent?: () => void;
  disabled?: boolean;
}) {
  const effectiveStatus = disabled ? "unchanged" : status;
  const showUseCurrent =
    !disabled && effectiveStatus === "changed" && onUseCurrent;

  const content = (
    <div className={cn("space-y-1.5", disabled && "opacity-60")}>
      <div className="flex items-center justify-between">
        <Label>{formatMetadataFieldLabel(field)}</Label>
        <StatusBadge status={effectiveStatus} />
      </div>
      {currentValue != null && effectiveStatus !== "unchanged" && (
        <CurrentBar onUseCurrent={showUseCurrent ? onUseCurrent : undefined}>
          {currentValue}
        </CurrentBar>
      )}
      {children}
    </div>
  );

  if (disabled) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <div>{content}</div>
        </TooltipTrigger>
        <TooltipContent>Field disabled for this plugin</TooltipContent>
      </Tooltip>
    );
  }

  return content;
}

function TagInput({
  tags,
  onChange,
  disabled,
  placeholder,
}: {
  tags: string[];
  onChange: (tags: string[]) => void;
  disabled?: boolean;
  placeholder?: string;
}) {
  const [input, setInput] = useState("");

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" && input.trim()) {
      e.preventDefault();
      if (!tags.includes(input.trim())) {
        onChange([...tags, input.trim()]);
      }
      setInput("");
    }
    if (e.key === "Backspace" && !input && tags.length > 0) {
      onChange(tags.slice(0, -1));
    }
  };

  return (
    <div
      className={cn(
        "flex flex-wrap gap-1.5 rounded-md border border-input bg-transparent p-2 min-h-[36px]",
        disabled && "opacity-50 cursor-not-allowed",
      )}
    >
      {tags.map((tag, i) => (
        <Badge
          className="max-w-full gap-1 pr-1"
          key={`${tag}-${i}`}
          variant="secondary"
        >
          <span className="truncate" title={tag}>
            {tag}
          </span>
          {!disabled && (
            <button
              className="shrink-0 rounded-sm hover:bg-muted-foreground/20 p-0.5 cursor-pointer"
              onClick={() => onChange(tags.filter((_, j) => j !== i))}
              type="button"
            >
              <X className="h-3 w-3" />
            </button>
          )}
        </Badge>
      ))}
      {!disabled && (
        <input
          className="flex-1 min-w-[80px] bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={
            tags.length === 0 ? (placeholder ?? "Type and press Enter") : ""
          }
          type="text"
          value={input}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main Component
// ---------------------------------------------------------------------------

export function IdentifyReviewForm({
  result,
  book,
  fileId,
  onBack,
  onClose,
}: IdentifyReviewFormProps) {
  const file = findFile(book, fileId);
  const applyMutation = usePluginApply();
  const disabledFields = useMemo(
    () => new Set(result.disabled_fields ?? []),
    [result.disabled_fields],
  );

  // ---- Extract current values ----
  const currentAuthors: AuthorEntry[] = useMemo(
    () =>
      (book.authors ?? []).map((a) => ({
        name: a.person?.name ?? "",
        role: a.role,
      })),
    [book.authors],
  );

  const currentNarrators: string[] = useMemo(
    () => (file?.narrators ?? []).map((n) => n.person?.name ?? ""),
    [file?.narrators],
  );

  const currentSeries = book.book_series?.[0]?.series?.name ?? "";
  const currentSeriesNumber =
    book.book_series?.[0]?.series_number?.toString() ?? "";

  const currentGenres: string[] = useMemo(
    () => (book.book_genres ?? []).map((bg) => bg.genre?.name ?? ""),
    [book.book_genres],
  );

  const currentTags: string[] = useMemo(
    () => (book.book_tags ?? []).map((bt) => bt.tag?.name ?? ""),
    [book.book_tags],
  );

  const currentIdentifiers: IdentifierEntry[] = useMemo(
    () =>
      (file?.identifiers ?? []).map((id) => ({
        type: id.type,
        value: id.value,
      })),
    [file?.identifiers],
  );

  // ---- Compute smart merge defaults ----
  const defaults = useMemo(() => {
    const incomingAuthors: AuthorEntry[] = (result.authors ?? []).map((a) => ({
      name: a.name,
      role: a.role,
    }));
    const incomingIdentifiers: IdentifierEntry[] = (
      result.identifiers ?? []
    ).map((id) => ({ type: id.type, value: id.value }));

    return {
      title: resolveScalar(book.title, result.title),
      subtitle: resolveScalar(book.subtitle, result.subtitle),
      description: resolveScalar(book.description, result.description),
      authors: resolveAuthors(currentAuthors, incomingAuthors),
      narrators: resolveArray(currentNarrators, result.narrators ?? []),
      series: resolveScalar(currentSeries, result.series),
      seriesNumber: resolveScalar(
        currentSeriesNumber,
        result.series_number?.toString(),
      ),
      genres: resolveArray(currentGenres, result.genres ?? []),
      tags: resolveArray(currentTags, result.tags ?? []),
      publisher: resolveScalar(file?.publisher?.name, result.publisher),
      imprint: resolveScalar(file?.imprint?.name, result.imprint),
      releaseDate: resolveScalar(
        file?.release_date ? file.release_date.split("T")[0] : undefined,
        result.release_date,
      ),
      url: resolveScalar(file?.url, result.url),
      identifiers: resolveIdentifiers(currentIdentifiers, incomingIdentifiers),
    };
  }, [
    book,
    result,
    currentAuthors,
    currentNarrators,
    currentSeries,
    currentSeriesNumber,
    currentGenres,
    currentTags,
    currentIdentifiers,
    file,
  ]);

  // ---- Form state ----
  const [title, setTitle] = useState(defaults.title.value);
  const [subtitle, setSubtitle] = useState(defaults.subtitle.value);
  const [description, setDescription] = useState(defaults.description.value);
  const [authors, setAuthors] = useState<AuthorEntry[]>(defaults.authors.value);
  const [narrators, setNarrators] = useState<string[]>(
    defaults.narrators.value,
  );
  const [series, setSeries] = useState(defaults.series.value);
  const [seriesNumber, setSeriesNumber] = useState(defaults.seriesNumber.value);
  const [genres, setGenres] = useState<string[]>(defaults.genres.value);
  const [tags, setTags] = useState<string[]>(defaults.tags.value);
  const [publisher, setPublisher] = useState(defaults.publisher.value);
  const [imprint, setImprint] = useState(defaults.imprint.value);
  const [releaseDate, setReleaseDate] = useState(defaults.releaseDate.value);
  const [url, setUrl] = useState(defaults.url.value);
  const [identifiers, setIdentifiers] = useState<IdentifierEntry[]>(
    defaults.identifiers.value,
  );

  // Cover state
  const newCoverUrl = result.image_url || result.cover_url;
  const currentCoverUrl = file
    ? `/api/books/files/${file.id}/cover?t=${new Date(file.updated_at).getTime()}`
    : undefined;
  const hasCoverChoice = !!newCoverUrl;
  const [coverSelection, setCoverSelection] = useState<"current" | "new">(
    newCoverUrl ? "new" : "current",
  );

  // ---- Submit ----
  const handleSubmit = () => {
    const fields: Record<string, unknown> = {
      title,
      subtitle,
      description,
      authors: authors.map((a) => ({ name: a.name, role: a.role })),
      narrators,
      series,
      series_number: seriesNumber ? parseFloat(seriesNumber) : undefined,
      genres,
      tags,
      publisher,
      imprint,
      release_date: releaseDate,
      url,
      identifiers: identifiers.map((id) => ({
        type: id.type,
        value: id.value,
      })),
    };

    if (coverSelection === "new" && newCoverUrl) {
      fields.cover_url = newCoverUrl;
    }

    applyMutation.mutate(
      {
        book_id: book.id,
        file_id: fileId,
        fields,
        plugin_scope: result.plugin_scope,
        plugin_id: result.plugin_id,
      },
      {
        onSuccess: () => {
          toast.success("Metadata applied successfully.");
          onClose();
        },
        onError: (err) => {
          toast.error(err.message || "Failed to apply metadata.");
        },
      },
    );
  };

  // ---- Helper to check if a field is disabled ----
  const isDisabled = (field: string) => disabledFields.has(field);

  // ---- Render ----
  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-2">
        <Button
          className="shrink-0"
          onClick={onBack}
          size="sm"
          type="button"
          variant="ghost"
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h3 className="text-sm font-semibold">Review Changes</h3>
          <p className="text-xs text-muted-foreground">
            Review and edit the metadata before applying.
          </p>
        </div>
      </div>

      {/* Cover */}
      {hasCoverChoice && (
        <div className="space-y-1.5">
          <div className="flex items-center justify-between">
            <Label>{formatMetadataFieldLabel("cover")}</Label>
            <StatusBadge
              status={
                isDisabled("cover")
                  ? "unchanged"
                  : currentCoverUrl
                    ? "changed"
                    : "new"
              }
            />
          </div>
          <div className="flex gap-4">
            {/* Current cover */}
            {currentCoverUrl && (
              <button
                className={cn(
                  "relative rounded-md overflow-hidden border-2 transition-colors cursor-pointer",
                  coverSelection === "current"
                    ? "border-primary"
                    : "border-border hover:border-muted-foreground/50",
                  isDisabled("cover") && "opacity-60 cursor-not-allowed",
                )}
                disabled={isDisabled("cover")}
                onClick={() => setCoverSelection("current")}
                type="button"
              >
                <img
                  alt="Current cover"
                  className="w-24 h-36 object-cover bg-muted"
                  src={currentCoverUrl}
                />
                <span className="absolute bottom-0 inset-x-0 bg-black/60 text-white text-[0.6rem] text-center py-0.5">
                  Keep current
                </span>
              </button>
            )}
            {/* New cover */}
            <button
              className={cn(
                "relative rounded-md overflow-hidden border-2 transition-colors cursor-pointer",
                coverSelection === "new"
                  ? "border-primary"
                  : "border-border hover:border-muted-foreground/50",
                isDisabled("cover") && "opacity-60 cursor-not-allowed",
              )}
              disabled={isDisabled("cover")}
              onClick={() => setCoverSelection("new")}
              type="button"
            >
              <img
                alt="New cover"
                className="w-24 h-36 object-cover bg-muted"
                src={newCoverUrl}
              />
              <span className="absolute bottom-0 inset-x-0 bg-black/60 text-white text-[0.6rem] text-center py-0.5">
                Use new
              </span>
            </button>
          </div>
        </div>
      )}

      {/* Title */}
      <FieldWrapper
        currentValue={book.title || undefined}
        disabled={isDisabled("title")}
        field="title"
        onUseCurrent={() => setTitle(book.title)}
        status={defaults.title.status}
      >
        <Input
          disabled={isDisabled("title")}
          onChange={(e) => setTitle(e.target.value)}
          value={title}
        />
      </FieldWrapper>

      {/* Subtitle */}
      <FieldWrapper
        currentValue={book.subtitle || undefined}
        disabled={isDisabled("subtitle")}
        field="subtitle"
        onUseCurrent={() => setSubtitle(book.subtitle ?? "")}
        status={defaults.subtitle.status}
      >
        <Input
          disabled={isDisabled("subtitle")}
          onChange={(e) => setSubtitle(e.target.value)}
          value={subtitle}
        />
      </FieldWrapper>

      {/* Authors */}
      <FieldWrapper
        currentValue={
          currentAuthors.length > 0
            ? currentAuthors.map((a) => a.name).join(", ")
            : undefined
        }
        disabled={isDisabled("authors")}
        field="authors"
        onUseCurrent={() => setAuthors(currentAuthors)}
        status={defaults.authors.status}
      >
        <TagInput
          disabled={isDisabled("authors")}
          onChange={(names) =>
            setAuthors(names.map((name) => ({ name, role: undefined })))
          }
          placeholder="Add author..."
          tags={authors.map((a) => a.name)}
        />
      </FieldWrapper>

      {/* Narrators */}
      <FieldWrapper
        currentValue={
          currentNarrators.length > 0 ? currentNarrators.join(", ") : undefined
        }
        disabled={isDisabled("narrators")}
        field="narrators"
        onUseCurrent={() => setNarrators(currentNarrators)}
        status={defaults.narrators.status}
      >
        <TagInput
          disabled={isDisabled("narrators")}
          onChange={setNarrators}
          placeholder="Add narrator..."
          tags={narrators}
        />
      </FieldWrapper>

      {/* Series */}
      <FieldWrapper
        currentValue={
          currentSeries
            ? `${currentSeries}${currentSeriesNumber ? ` #${currentSeriesNumber}` : ""}`
            : undefined
        }
        disabled={isDisabled("series")}
        field="series"
        onUseCurrent={() => {
          setSeries(currentSeries);
          setSeriesNumber(currentSeriesNumber);
        }}
        status={
          defaults.series.status === "changed" ||
          defaults.seriesNumber.status === "changed"
            ? "changed"
            : defaults.series.status === "new" ||
                defaults.seriesNumber.status === "new"
              ? "new"
              : "unchanged"
        }
      >
        <div className="flex gap-2">
          <Input
            className="flex-1"
            disabled={isDisabled("series")}
            onChange={(e) => setSeries(e.target.value)}
            placeholder="Series name"
            value={series}
          />
          <Input
            className="w-24"
            disabled={isDisabled("series")}
            onChange={(e) => setSeriesNumber(e.target.value)}
            placeholder="#"
            type="number"
            value={seriesNumber}
          />
        </div>
      </FieldWrapper>

      {/* Genres */}
      <FieldWrapper
        currentValue={
          currentGenres.length > 0 ? currentGenres.join(", ") : undefined
        }
        disabled={isDisabled("genres")}
        field="genres"
        onUseCurrent={() => setGenres(currentGenres)}
        status={defaults.genres.status}
      >
        <TagInput
          disabled={isDisabled("genres")}
          onChange={setGenres}
          placeholder="Add genre..."
          tags={genres}
        />
      </FieldWrapper>

      {/* Tags */}
      <FieldWrapper
        currentValue={
          currentTags.length > 0 ? currentTags.join(", ") : undefined
        }
        disabled={isDisabled("tags")}
        field="tags"
        onUseCurrent={() => setTags(currentTags)}
        status={defaults.tags.status}
      >
        <TagInput
          disabled={isDisabled("tags")}
          onChange={setTags}
          placeholder="Add tag..."
          tags={tags}
        />
      </FieldWrapper>

      {/* Description */}
      <div
        className={cn("space-y-1.5", isDisabled("description") && "opacity-60")}
      >
        <div className="flex items-center justify-between">
          <Label>{formatMetadataFieldLabel("description")}</Label>
          <StatusBadge
            status={
              isDisabled("description")
                ? "unchanged"
                : defaults.description.status
            }
          />
        </div>
        {(book.description ?? "").trim() &&
          defaults.description.status !== "unchanged" && (
            <CollapsibleCurrentBar
              onUseCurrent={
                !isDisabled("description") &&
                defaults.description.status === "changed"
                  ? () => setDescription(book.description ?? "")
                  : undefined
              }
              text={book.description ?? ""}
            />
          )}
        <Textarea
          className="min-h-[100px]"
          disabled={isDisabled("description")}
          onChange={(e) => setDescription(e.target.value)}
          value={description}
        />
      </div>

      {/* Publisher */}
      <FieldWrapper
        currentValue={file?.publisher?.name || undefined}
        disabled={isDisabled("publisher")}
        field="publisher"
        onUseCurrent={() => setPublisher(file?.publisher?.name ?? "")}
        status={defaults.publisher.status}
      >
        <Input
          disabled={isDisabled("publisher")}
          onChange={(e) => setPublisher(e.target.value)}
          value={publisher}
        />
      </FieldWrapper>

      {/* Imprint */}
      <FieldWrapper
        currentValue={file?.imprint?.name || undefined}
        disabled={isDisabled("imprint")}
        field="imprint"
        onUseCurrent={() => setImprint(file?.imprint?.name ?? "")}
        status={defaults.imprint.status}
      >
        <Input
          disabled={isDisabled("imprint")}
          onChange={(e) => setImprint(e.target.value)}
          value={imprint}
        />
      </FieldWrapper>

      {/* Release Date */}
      <FieldWrapper
        currentValue={
          file?.release_date ? file.release_date.split("T")[0] : undefined
        }
        disabled={isDisabled("releaseDate")}
        field="releaseDate"
        onUseCurrent={() =>
          setReleaseDate(
            file?.release_date ? file.release_date.split("T")[0] : "",
          )
        }
        status={defaults.releaseDate.status}
      >
        <Input
          disabled={isDisabled("releaseDate")}
          onChange={(e) => setReleaseDate(e.target.value)}
          placeholder="YYYY-MM-DD"
          value={releaseDate}
        />
      </FieldWrapper>

      {/* URL */}
      <FieldWrapper
        currentValue={file?.url || undefined}
        disabled={isDisabled("url")}
        field="url"
        onUseCurrent={() => setUrl(file?.url ?? "")}
        status={defaults.url.status}
      >
        <Input
          disabled={isDisabled("url")}
          onChange={(e) => setUrl(e.target.value)}
          value={url}
        />
      </FieldWrapper>

      {/* Identifiers */}
      <div
        className={cn("space-y-1.5", isDisabled("identifiers") && "opacity-60")}
      >
        <div className="flex items-center justify-between">
          <Label>{formatMetadataFieldLabel("identifiers")}</Label>
          <StatusBadge
            status={
              isDisabled("identifiers")
                ? "unchanged"
                : defaults.identifiers.status
            }
          />
        </div>
        {currentIdentifiers.length > 0 &&
          defaults.identifiers.status !== "unchanged" && (
            <CurrentBar
              onUseCurrent={
                !isDisabled("identifiers") &&
                defaults.identifiers.status === "changed"
                  ? () => setIdentifiers(currentIdentifiers)
                  : undefined
              }
            >
              {currentIdentifiers
                .map((id) => `${id.type}:${id.value}`)
                .join(", ")}
            </CurrentBar>
          )}
        <IdentifierTagInput
          disabled={isDisabled("identifiers")}
          onChange={setIdentifiers}
          value={identifiers}
        />
      </div>

      {/* Footer */}
      <div className="flex justify-between border-t p-4">
        <Button
          disabled={applyMutation.isPending}
          onClick={onBack}
          variant="ghost"
        >
          Back to results
        </Button>
        <div className="flex gap-2">
          <Button
            disabled={applyMutation.isPending}
            onClick={onClose}
            variant="outline"
          >
            Cancel
          </Button>
          <Button disabled={applyMutation.isPending} onClick={handleSubmit}>
            {applyMutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
                Applying...
              </>
            ) : (
              "Apply Changes"
            )}
          </Button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Identifier tag input (key:value pairs)
// ---------------------------------------------------------------------------

function IdentifierTagInput({
  value,
  onChange,
  disabled,
}: {
  value: IdentifierEntry[];
  onChange: (ids: IdentifierEntry[]) => void;
  disabled?: boolean;
}) {
  const [input, setInput] = useState("");

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" && input.trim()) {
      e.preventDefault();
      const colonIndex = input.indexOf(":");
      if (colonIndex > 0) {
        const type = input.slice(0, colonIndex).trim();
        const val = input.slice(colonIndex + 1).trim();
        if (type && val) {
          onChange([...value, { type, value: val }]);
          setInput("");
        }
      }
    }
    if (e.key === "Backspace" && !input && value.length > 0) {
      onChange(value.slice(0, -1));
    }
  };

  return (
    <div
      className={cn(
        "flex flex-wrap gap-1.5 rounded-md border border-input bg-transparent p-2 min-h-[36px]",
        disabled && "opacity-50 cursor-not-allowed",
      )}
    >
      {value.map((id, i) => (
        <Badge
          className="max-w-full gap-1 pr-1"
          key={`${id.type}-${id.value}-${i}`}
          variant="secondary"
        >
          <span className="truncate" title={`${id.type}:${id.value}`}>
            {id.type}:{id.value}
          </span>
          {!disabled && (
            <button
              className="shrink-0 rounded-sm hover:bg-muted-foreground/20 p-0.5 cursor-pointer"
              onClick={() => onChange(value.filter((_, j) => j !== i))}
              type="button"
            >
              <X className="h-3 w-3" />
            </button>
          )}
        </Badge>
      ))}
      {!disabled && (
        <input
          className="flex-1 min-w-[120px] bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={value.length === 0 ? "type:value (press Enter)" : ""}
          type="text"
          value={input}
        />
      )}
    </div>
  );
}
