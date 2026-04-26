import equal from "fast-deep-equal";
import {
  ArrowLeft,
  ChevronDown,
  ChevronUp,
  ExternalLink,
  Loader2,
  X,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { EntityCombobox } from "@/components/common/EntityCombobox";
import { IdentifierEditor } from "@/components/common/IdentifierEditor";
import { SortableEntityList } from "@/components/common/SortableEntityList";
import { SortNameInput } from "@/components/common/SortNameInput";
import { ExtractSubtitleButton } from "@/components/library/ExtractSubtitleButton";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { MultiSelectCombobox } from "@/components/ui/MultiSelectCombobox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { getLanguageName } from "@/constants/languages";
import { useGenresList } from "@/hooks/queries/genres";
import { useImprintsList } from "@/hooks/queries/imprints";
import { usePeopleList } from "@/hooks/queries/people";
import {
  usePluginApply,
  usePluginIdentifierTypes,
  type PluginSearchResult,
} from "@/hooks/queries/plugins";
import { usePublishersList } from "@/hooks/queries/publishers";
import { useSeriesList } from "@/hooks/queries/series";
import { useTagsList } from "@/hooks/queries/tags";
import { useAutoMatchEntities } from "@/hooks/useAutoMatchEntities";
import { useDebounce } from "@/hooks/useDebounce";
import { cn, isPageBasedFileType } from "@/libraries/utils";
import {
  AuthorRoleColorist,
  AuthorRoleCoverArtist,
  AuthorRoleEditor,
  AuthorRoleInker,
  AuthorRoleLetterer,
  AuthorRolePenciller,
  AuthorRoleTranslator,
  AuthorRoleWriter,
  FileTypeCBZ,
  type Book,
  type File,
} from "@/types";
import { getAuthorRoleLabel } from "@/utils/authorRoles";
import { formatMetadataFieldLabel } from "@/utils/format";
import { getPrimaryFileType } from "@/utils/primaryFile";
import { formatSeriesNumber } from "@/utils/seriesNumber";

import {
  resolveIdentifiers,
  type FieldStatus,
  type IdentifierEntry,
} from "./identify-utils";
import { LanguageCombobox } from "./LanguageCombobox";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface IdentifyReviewFormProps {
  result: PluginSearchResult;
  book: Book;
  fileId?: number;
  onBack: () => void;
  onClose: () => void;
  onHasChangesChange?: (hasChanges: boolean) => void;
}

interface AuthorEntry {
  name: string;
  role?: string;
}

interface NameOption {
  name: string;
}

// Author role options for CBZ files
const AUTHOR_ROLES = [
  { value: AuthorRoleWriter, label: "Writer" },
  { value: AuthorRolePenciller, label: "Penciller" },
  { value: AuthorRoleInker, label: "Inker" },
  { value: AuthorRoleColorist, label: "Colorist" },
  { value: AuthorRoleLetterer, label: "Letterer" },
  { value: AuthorRoleCoverArtist, label: "Cover Artist" },
  { value: AuthorRoleEditor, label: "Editor" },
  { value: AuthorRoleTranslator, label: "Translator" },
] as const;

// Adapter hooks: bridge useXxxList query hooks to EntityCombobox's `hook` prop
// signature. Defined at module scope so they're stable references and the same
// hooks run in the same order on every render. Each adapter maps the API list
// shape to a `{ name }` shape that the combobox consumes.

function usePeopleSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: NameOption[]; isLoading: boolean } {
  const { data, isLoading } = usePeopleList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.people.map((p) => ({ name: p.name }));
  return { data: adapted, isLoading };
}

function useSeriesSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: NameOption[]; isLoading: boolean } {
  const { data, isLoading } = useSeriesList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.series.map((s) => ({ name: s.name }));
  return { data: adapted, isLoading };
}

function usePublisherSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: NameOption[]; isLoading: boolean } {
  const { data, isLoading } = usePublishersList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.publishers.map((p) => ({ name: p.name }));
  return { data: adapted, isLoading };
}

function useImprintSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: NameOption[]; isLoading: boolean } {
  const { data, isLoading } = useImprintsList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.imprints.map((i) => ({ name: i.name }));
  return { data: adapted, isLoading };
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Load natural dimensions of an image URL. */
function useImageDimensions(src: string | undefined) {
  const [dims, setDims] = useState<{ w: number; h: number } | null>(null);

  useEffect(() => {
    if (!src) {
      setDims(null);
      return;
    }
    let cancelled = false;
    const img = new Image();
    img.onload = () => {
      if (!cancelled) setDims({ w: img.naturalWidth, h: img.naturalHeight });
    };
    img.onerror = () => {
      if (!cancelled) setDims(null);
    };
    img.src = src;
    return () => {
      cancelled = true;
    };
  }, [src]);

  return dims;
}

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

/** Determine field status and default value for the abridged bool field. */
function resolveAbridged(
  current: boolean | undefined | null,
  incoming: boolean | undefined | null,
): { value: boolean | null; status: FieldStatus } {
  const cur = current ?? null;
  const inc = incoming ?? null;

  if (cur === null && inc !== null) return { value: inc, status: "new" };
  if (cur !== null && inc === null) return { value: cur, status: "unchanged" };
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
    <Badge className="text-[0.65rem]" variant="outline">
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
    <div className="flex items-start justify-between gap-2 border-l-2 border-muted-foreground/30 bg-muted/50 rounded-r-md px-3 py-1.5 text-sm text-muted-foreground">
      <span className="min-w-0 break-words">{children}</span>
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

// ---------------------------------------------------------------------------
// Main Component
// ---------------------------------------------------------------------------

export function IdentifyReviewForm({
  result,
  book,
  fileId,
  onBack,
  onClose,
  onHasChangesChange,
}: IdentifyReviewFormProps) {
  const file = findFile(book, fileId);
  const primaryFileType = getPrimaryFileType(book);
  const applyMutation = usePluginApply();
  const { data: pluginIdentifierTypes } = usePluginIdentifierTypes();
  const disabledFields = useMemo(
    () => new Set(result.disabled_fields ?? []),
    [result.disabled_fields],
  );
  const isDisabled = (field: string) => disabledFields.has(field);

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
  const currentSeriesNumberUnit =
    book.book_series?.[0]?.series_number_unit ?? "";

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
      seriesNumberUnit: resolveScalar(
        currentSeriesNumberUnit,
        result.series_number_unit ?? undefined,
      ),
      genres: resolveArray(currentGenres, result.genres ?? []),
      tags: resolveArray(currentTags, result.tags ?? []),
      publisher: resolveScalar(file?.publisher?.name, result.publisher),
      imprint: resolveScalar(file?.imprint?.name, result.imprint),
      releaseDate: resolveScalar(
        file?.release_date ? file.release_date.split("T")[0] : undefined,
        result.release_date ? result.release_date.split("T")[0] : undefined,
      ),
      url: resolveScalar(file?.url, result.url),
      language: resolveScalar(file?.language, result.language),
      abridged: resolveAbridged(file?.abridged, result.abridged),
      identifiers: resolveIdentifiers(currentIdentifiers, incomingIdentifiers),
    };
  }, [
    book,
    result,
    currentAuthors,
    currentNarrators,
    currentSeries,
    currentSeriesNumber,
    currentSeriesNumberUnit,
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
  const [seriesNumberUnit, setSeriesNumberUnit] = useState(
    defaults.seriesNumberUnit.value,
  );
  const [genres, setGenres] = useState<string[]>(defaults.genres.value);
  const [tags, setTags] = useState<string[]>(defaults.tags.value);
  const [publisher, setPublisher] = useState(defaults.publisher.value);
  const [imprint, setImprint] = useState(defaults.imprint.value);
  const [releaseDate, setReleaseDate] = useState(defaults.releaseDate.value);
  const [url, setUrl] = useState(defaults.url.value);
  const [language, setLanguage] = useState(defaults.language.value);
  const [abridged, setAbridged] = useState<boolean | null>(
    defaults.abridged.value,
  );
  const [identifiers, setIdentifiers] = useState<IdentifierEntry[]>(
    defaults.identifiers.value,
  );
  // Sort title is intentionally not part of `defaults` — plugin results don't
  // surface a sort title, so there's no diff to render. We wire it through
  // SortNameInput which auto-generates from the title when in auto mode.
  const [sortTitle, setSortTitle] = useState(book.sort_title || "");

  // ---- Genre / tag option pools (server-side search) ----
  const [genreSearch, setGenreSearch] = useState("");
  const debouncedGenreSearch = useDebounce(genreSearch, 200);
  const [tagSearch, setTagSearch] = useState("");
  const debouncedTagSearch = useDebounce(tagSearch, 200);

  const { data: genresData, isLoading: isLoadingGenres } = useGenresList(
    {
      library_id: book.library_id,
      limit: 50,
      search: debouncedGenreSearch || undefined,
    },
    { enabled: !!book.library_id },
  );
  const { data: tagsData, isLoading: isLoadingTags } = useTagsList(
    {
      library_id: book.library_id,
      limit: 50,
      search: debouncedTagSearch || undefined,
    },
    { enabled: !!book.library_id },
  );

  // ---- Auto-match incoming entity names against this library ----
  const autoMatch = useAutoMatchEntities({
    libraryId: book.library_id ?? 0,
    enabled: !!book.library_id,
    authors: (result.authors ?? []).map((a) => a.name),
    narrators: result.narrators ?? [],
    series: result.series ? [result.series] : [],
    publisher: result.publisher,
    imprint: result.imprint,
    genres: result.genres ?? [],
    tags: result.tags ?? [],
  });

  // ---- Identifier types for the editor (built-ins + plugin-defined) ----
  const availableIdentifierTypes = useMemo(
    () => [
      { id: "isbn_10", label: "ISBN-10" },
      { id: "isbn_13", label: "ISBN-13" },
      { id: "asin", label: "ASIN" },
      { id: "uuid", label: "UUID" },
      { id: "goodreads", label: "Goodreads" },
      { id: "google", label: "Google" },
      { id: "other", label: "Other" },
      ...(pluginIdentifierTypes
        ?.filter(
          (pt) =>
            ![
              "isbn_10",
              "isbn_13",
              "asin",
              "uuid",
              "goodreads",
              "google",
              "other",
            ].includes(pt.id),
        )
        .map((pt) => ({ id: pt.id, label: pt.name, pattern: pt.pattern })) ??
        []),
    ],
    [pluginIdentifierTypes],
  );

  const isCbz = file?.file_type === FileTypeCBZ;

  // Cover state — for page-based formats (CBZ, PDF) the cover is a page of
  // the file itself, so plugin cover *image* data (cover_url/cover_data) is
  // ignored, but a plugin-supplied `cover_page` can still change the cover.
  const isFilePageBased = isPageBasedFileType(file?.file_type);
  const isAudiobook = file?.file_type === "m4b";
  const newCoverUrl = !isFilePageBased ? result.cover_url : undefined;
  // Only treat coverPage as usable when it's a non-negative integer within
  // the file's page range. A plugin returning out-of-range values would
  // otherwise render a broken preview and get silently dropped at apply time.
  const newCoverPage =
    isFilePageBased &&
    result.cover_page != null &&
    result.cover_page >= 0 &&
    (file?.page_count == null || result.cover_page < file.page_count)
      ? result.cover_page
      : undefined;
  // The preview URL shown for the "new" option. For page-based files with a
  // plugin-supplied cover_page, render the page via the file's page endpoint.
  const newCoverPreviewUrl =
    newCoverUrl ??
    (file && newCoverPage != null
      ? `/api/books/files/${file.id}/page/${newCoverPage}`
      : undefined);
  const currentCoverUrl = file?.cover_image_filename
    ? `/api/books/files/${file.id}/cover?v=${new Date(file.updated_at).getTime()}`
    : undefined;
  const hasCoverChoice = !!newCoverPreviewUrl;
  const currentCoverPage = file?.cover_page ?? null;
  // For page-based files with a plugin-supplied cover_page, compare by page
  // number instead of pixel resolution — the cover is a page of the file
  // itself, so resolution is a function of the source file, not the choice.
  const isPageBasedCoverChoice = isFilePageBased && newCoverPage != null;
  // Dimensions only matter for the resolution-based comparison on non-page
  // formats. Skip the image preload entirely for page-based choices.
  const currentCoverDims = useImageDimensions(
    isPageBasedCoverChoice ? undefined : currentCoverUrl,
  );
  const newCoverDims = useImageDimensions(
    isPageBasedCoverChoice ? undefined : newCoverPreviewUrl,
  );
  // Same page → unchanged (prefer current); different page → prefer new.
  // Requires `currentCoverUrl` so we don't default to a "Current" thumbnail
  // that isn't rendered (the current button is guarded by `currentCoverUrl`).
  const preferCurrentCover = isPageBasedCoverChoice
    ? !!currentCoverUrl &&
      currentCoverPage !== null &&
      currentCoverPage === newCoverPage
    : !!currentCoverDims &&
      !!newCoverDims &&
      currentCoverDims.w * currentCoverDims.h >=
        newCoverDims.w * newCoverDims.h;
  // The selection we'd land on if the user didn't touch anything. Used by
  // useState init, the auto-select effect, and `hasChanges` — keeping it in
  // one place avoids the three sites drifting.
  const defaultCoverSelection: "current" | "new" =
    hasCoverChoice && !isDisabled("cover") && !preferCurrentCover
      ? "new"
      : "current";
  const [coverSelection, setCoverSelection] = useState<"current" | "new">(
    defaultCoverSelection,
  );
  const [coverUserTouched, setCoverUserTouched] = useState(false);
  useEffect(() => {
    if (!coverUserTouched) {
      setCoverSelection(defaultCoverSelection);
    }
  }, [defaultCoverSelection, coverUserTouched]);

  // ---- Unsaved changes tracking ----
  const hasChanges = useMemo(() => {
    return (
      title !== defaults.title.value ||
      subtitle !== defaults.subtitle.value ||
      description !== defaults.description.value ||
      !equal(authors, defaults.authors.value) ||
      !equal(narrators, defaults.narrators.value) ||
      series !== defaults.series.value ||
      seriesNumber !== defaults.seriesNumber.value ||
      seriesNumberUnit !== defaults.seriesNumberUnit.value ||
      !equal(genres, defaults.genres.value) ||
      !equal(tags, defaults.tags.value) ||
      publisher !== defaults.publisher.value ||
      imprint !== defaults.imprint.value ||
      releaseDate !== defaults.releaseDate.value ||
      url !== defaults.url.value ||
      language !== defaults.language.value ||
      abridged !== defaults.abridged.value ||
      !equal(identifiers, defaults.identifiers.value) ||
      sortTitle !== (book.sort_title || "") ||
      coverSelection !== defaultCoverSelection
    );
  }, [
    title,
    subtitle,
    description,
    authors,
    narrators,
    series,
    seriesNumber,
    seriesNumberUnit,
    genres,
    tags,
    publisher,
    imprint,
    releaseDate,
    url,
    language,
    abridged,
    identifiers,
    sortTitle,
    book.sort_title,
    coverSelection,
    defaults,
    defaultCoverSelection,
  ]);

  useEffect(() => {
    onHasChangesChange?.(hasChanges);
  }, [hasChanges, onHasChangesChange]);

  // ---- Submit ----
  const handleSubmit = async () => {
    const fields: Record<string, unknown> = {
      title,
      // Always include sort_title so clearing it (toggling SortNameInput back
      // to auto-generate) is persisted. Trimmed, but empty string is sent as
      // a meaningful "clear" signal — same convention BookEditDialog uses.
      sort_title: sortTitle.trim(),
      subtitle,
      description,
      authors: authors.map((a) => ({ name: a.name, role: a.role })),
      narrators,
      series,
      series_number: seriesNumber !== "" ? parseFloat(seriesNumber) : undefined,
      series_number_unit:
        seriesNumberUnit !== "" ? seriesNumberUnit : undefined,
      genres,
      tags,
      publisher,
      imprint,
      release_date: releaseDate,
      url,
      language,
      // Only include abridged when explicitly set (non-null) so we don't
      // clobber a prior explicit value with an unknown state.
      ...(abridged !== null && { abridged }),
      identifiers: identifiers.map((id) => ({
        type: id.type,
        value: id.value,
      })),
    };

    if (coverSelection === "new") {
      if (newCoverUrl) {
        fields.cover_url = newCoverUrl;
      } else if (newCoverPage != null) {
        fields.cover_page = newCoverPage;
      }
    }

    try {
      await applyMutation.mutateAsync({
        book_id: book.id,
        file_id: fileId,
        fields,
        plugin_scope: result.plugin_scope,
        plugin_id: result.plugin_id,
      });
      toast.success("Metadata applied successfully.");
      onClose();
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "Failed to apply metadata.";
      toast.error(message);
    }
  };

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
                isDisabled("cover") || preferCurrentCover
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
                onClick={() => {
                  setCoverSelection("current");
                  setCoverUserTouched(true);
                }}
                type="button"
              >
                <img
                  alt="Current cover"
                  className={cn(
                    "w-24 object-cover bg-muted",
                    isAudiobook ? "h-24" : "h-36",
                  )}
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
              onClick={() => {
                setCoverSelection("new");
                setCoverUserTouched(true);
              }}
              type="button"
            >
              <img
                alt="New cover"
                className={cn(
                  "w-24 object-cover bg-muted",
                  isAudiobook ? "h-24" : "h-36",
                )}
                src={newCoverPreviewUrl}
              />
              <span className="absolute bottom-0 inset-x-0 bg-black/60 text-white text-[0.6rem] text-center py-0.5">
                Use new
              </span>
            </button>
          </div>
          <div className="flex gap-4 text-xs text-muted-foreground">
            {currentCoverUrl && (
              <span className="w-[calc(6rem+4px)] text-center">
                {isPageBasedCoverChoice
                  ? currentCoverPage !== null
                    ? `Page ${currentCoverPage + 1}`
                    : "\u00A0"
                  : currentCoverDims
                    ? `${currentCoverDims.w} × ${currentCoverDims.h}`
                    : "\u00A0"}
              </span>
            )}
            <span className="w-[calc(6rem+4px)] text-center">
              {isPageBasedCoverChoice
                ? `Page ${newCoverPage + 1}`
                : newCoverDims
                  ? `${newCoverDims.w} × ${newCoverDims.h}`
                  : "\u00A0"}
            </span>
          </div>
        </div>
      )}

      {/* Title */}
      <div className="space-y-1.5">
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

        {!isDisabled("title") && !isDisabled("subtitle") && (
          <ExtractSubtitleButton
            onExtract={(t, s) => {
              setTitle(t);
              setSubtitle(s);
            }}
            title={title}
          />
        )}
      </div>

      {/* Sort Title */}
      <div className="space-y-1.5">
        <Label>Sort Title</Label>
        <SortNameInput
          nameValue={title}
          onChange={setSortTitle}
          sortValue={book.sort_title || ""}
          source={book.sort_title_source}
          type="title"
        />
      </div>

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
            ? currentAuthors
                .map((a) => {
                  const label = getAuthorRoleLabel(a.role);
                  return label ? `${a.name} (${label})` : a.name;
                })
                .join(", ")
            : undefined
        }
        disabled={isDisabled("authors")}
        field="authors"
        onUseCurrent={() => setAuthors(currentAuthors)}
        status={defaults.authors.status}
      >
        <SortableEntityList<AuthorEntry>
          comboboxProps={{
            getOptionKey: (p) => p.name,
            getOptionLabel: (p) => p.name,
            hook: function useAuthorOptions(q) {
              return usePeopleSearch(book.library_id, true, q);
            },
            label: "Author",
          }}
          items={authors}
          onAppend={(next) => {
            const name = "__create" in next ? next.__create : next.name;
            if (!name.trim()) return;
            if (authors.some((a) => a.name === name)) return;
            const role = isCbz ? AuthorRoleWriter : undefined;
            setAuthors([...authors, { name, role }]);
          }}
          onRemove={(idx) => setAuthors(authors.filter((_, i) => i !== idx))}
          onReorder={setAuthors}
          pendingCreate={(a) => {
            const m = autoMatch.matches.authors.find((x) => x.name === a.name);
            return !!m && m.existing == null;
          }}
          renderExtras={
            isCbz
              ? (author, idx) => (
                  <div className="w-36">
                    <Select
                      onValueChange={(value) => {
                        const next = [...authors];
                        next[idx] = {
                          ...next[idx],
                          role: value === "none" ? undefined : value,
                        };
                        setAuthors(next);
                      }}
                      value={author.role || "none"}
                    >
                      <SelectTrigger className="cursor-pointer">
                        <SelectValue placeholder="Role" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem className="cursor-pointer" value="none">
                          No role
                        </SelectItem>
                        {AUTHOR_ROLES.map((role) => (
                          <SelectItem
                            className="cursor-pointer"
                            key={role.value}
                            value={role.value}
                          >
                            {role.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                )
              : undefined
          }
        />
      </FieldWrapper>

      {/* Narrators (audiobooks only) */}
      {isAudiobook && (
        <FieldWrapper
          currentValue={
            currentNarrators.length > 0
              ? currentNarrators.join(", ")
              : undefined
          }
          disabled={isDisabled("narrators")}
          field="narrators"
          onUseCurrent={() => setNarrators(currentNarrators)}
          status={defaults.narrators.status}
        >
          <SortableEntityList<NameOption>
            comboboxProps={{
              getOptionKey: (p) => p.name,
              getOptionLabel: (p) => p.name,
              hook: function useNarratorOptions(q) {
                return usePeopleSearch(book.library_id, true, q);
              },
              label: "Narrator",
            }}
            items={narrators.map((name) => ({ name }))}
            onAppend={(next) => {
              const name = "__create" in next ? next.__create : next.name;
              if (!name.trim()) return;
              if (narrators.includes(name)) return;
              setNarrators([...narrators, name]);
            }}
            onRemove={(idx) =>
              setNarrators(narrators.filter((_, i) => i !== idx))
            }
            onReorder={(next) => setNarrators(next.map((n) => n.name))}
            pendingCreate={(n) => {
              const m = autoMatch.matches.narrators.find(
                (x) => x.name === n.name,
              );
              return !!m && m.existing == null;
            }}
          />
        </FieldWrapper>
      )}

      {/* Series */}
      <FieldWrapper
        currentValue={
          currentSeries
            ? `${currentSeries}${
                currentSeriesNumber
                  ? ` ${formatSeriesNumber(parseFloat(currentSeriesNumber), currentSeriesNumberUnit || null, primaryFileType)}`
                  : ""
              }`
            : undefined
        }
        disabled={isDisabled("series")}
        field="series"
        onUseCurrent={() => {
          setSeries(currentSeries);
          setSeriesNumber(currentSeriesNumber);
          setSeriesNumberUnit(currentSeriesNumberUnit);
        }}
        status={
          defaults.series.status === "changed" ||
          defaults.seriesNumber.status === "changed" ||
          defaults.seriesNumberUnit.status === "changed"
            ? "changed"
            : defaults.series.status === "new" ||
                defaults.seriesNumber.status === "new" ||
                defaults.seriesNumberUnit.status === "new"
              ? "new"
              : "unchanged"
        }
      >
        <div className="flex gap-2 items-center">
          <div className="flex-1">
            <EntityCombobox<NameOption>
              getOptionKey={(s) => s.name}
              getOptionLabel={(s) => s.name}
              hook={function useSeriesOptions(q) {
                return useSeriesSearch(book.library_id, true, q);
              }}
              label="Series"
              onChange={(next) =>
                setSeries("__create" in next ? next.__create : next.name)
              }
              pendingCreate={(() => {
                if (!series) return false;
                const m = autoMatch.matches.series.find(
                  (x) => x.name === series,
                );
                return !!m && m.existing == null;
              })()}
              value={series ? { name: series } : null}
            />
          </div>
          <Input
            className="w-24"
            disabled={isDisabled("series")}
            onChange={(e) => setSeriesNumber(e.target.value)}
            placeholder="#"
            type="number"
            value={seriesNumber}
          />
          {series && !isDisabled("series") && (
            <Button
              aria-label="Clear series"
              className="cursor-pointer shrink-0"
              onClick={() => {
                setSeries("");
                setSeriesNumber("");
              }}
              size="icon"
              type="button"
              variant="ghost"
            >
              <X className="h-4 w-4" />
            </Button>
          )}
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
        <MultiSelectCombobox
          isLoading={isLoadingGenres}
          label="Genre"
          onChange={setGenres}
          onSearch={setGenreSearch}
          options={genresData?.genres.map((g) => g.name) ?? []}
          placeholder="Add genres..."
          removed={currentGenres.filter((g) => !genres.includes(g))}
          searchValue={genreSearch}
          status={(v) => (currentGenres.includes(v) ? "unchanged" : "new")}
          values={genres}
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
        <MultiSelectCombobox
          isLoading={isLoadingTags}
          label="Tag"
          onChange={setTags}
          onSearch={setTagSearch}
          options={tagsData?.tags.map((t) => t.name) ?? []}
          placeholder="Add tags..."
          removed={currentTags.filter((t) => !tags.includes(t))}
          searchValue={tagSearch}
          status={(v) => (currentTags.includes(v) ? "unchanged" : "new")}
          values={tags}
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
        <div className="flex gap-2 items-center">
          <div className="flex-1">
            <EntityCombobox<NameOption>
              getOptionKey={(p) => p.name}
              getOptionLabel={(p) => p.name}
              hook={function usePublisherOptions(q) {
                return usePublisherSearch(book.library_id, true, q);
              }}
              label="Publisher"
              onChange={(next) =>
                setPublisher("__create" in next ? next.__create : next.name)
              }
              pendingCreate={(() => {
                if (!publisher) return false;
                const m = autoMatch.matches.publisher;
                return !!m && m.name === publisher && m.existing == null;
              })()}
              value={publisher ? { name: publisher } : null}
            />
          </div>
          {publisher && !isDisabled("publisher") && (
            <Button
              aria-label="Clear publisher"
              className="cursor-pointer shrink-0"
              onClick={() => setPublisher("")}
              size="icon"
              type="button"
              variant="ghost"
            >
              <X className="h-4 w-4" />
            </Button>
          )}
        </div>
      </FieldWrapper>

      {/* Imprint */}
      <FieldWrapper
        currentValue={file?.imprint?.name || undefined}
        disabled={isDisabled("imprint")}
        field="imprint"
        onUseCurrent={() => setImprint(file?.imprint?.name ?? "")}
        status={defaults.imprint.status}
      >
        <div className="flex gap-2 items-center">
          <div className="flex-1">
            <EntityCombobox<NameOption>
              getOptionKey={(p) => p.name}
              getOptionLabel={(p) => p.name}
              hook={function useImprintOptions(q) {
                return useImprintSearch(book.library_id, true, q);
              }}
              label="Imprint"
              onChange={(next) =>
                setImprint("__create" in next ? next.__create : next.name)
              }
              pendingCreate={(() => {
                if (!imprint) return false;
                const m = autoMatch.matches.imprint;
                return !!m && m.name === imprint && m.existing == null;
              })()}
              value={imprint ? { name: imprint } : null}
            />
          </div>
          {imprint && !isDisabled("imprint") && (
            <Button
              aria-label="Clear imprint"
              className="cursor-pointer shrink-0"
              onClick={() => setImprint("")}
              size="icon"
              type="button"
              variant="ghost"
            >
              <X className="h-4 w-4" />
            </Button>
          )}
        </div>
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
        <DatePicker
          onChange={setReleaseDate}
          placeholder="Pick a date"
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
        <div className="flex gap-2">
          <Input
            className="flex-1"
            disabled={isDisabled("url")}
            onChange={(e) => setUrl(e.target.value)}
            value={url}
          />
          <Button
            asChild={!!url.trim()}
            disabled={!url.trim()}
            size="icon"
            type="button"
            variant="outline"
          >
            {url.trim() ? (
              <a href={url.trim()} rel="noopener noreferrer" target="_blank">
                <ExternalLink className="h-4 w-4" />
              </a>
            ) : (
              <span>
                <ExternalLink className="h-4 w-4" />
              </span>
            )}
          </Button>
        </div>
      </FieldWrapper>

      {/* Language */}
      <FieldWrapper
        currentValue={
          file?.language
            ? getLanguageName(file.language)
              ? `${getLanguageName(file.language)} (${file.language})`
              : file.language
            : undefined
        }
        disabled={isDisabled("language")}
        field="language"
        onUseCurrent={() => setLanguage(file?.language ?? "")}
        status={defaults.language.status}
      >
        <LanguageCombobox
          disabled={isDisabled("language")}
          libraryId={book.library_id}
          onChange={setLanguage}
          value={language}
        />
      </FieldWrapper>

      {/* Abridged */}
      <FieldWrapper
        currentValue={
          file?.abridged != null
            ? file.abridged
              ? "Abridged"
              : "Unabridged"
            : undefined
        }
        disabled={isDisabled("abridged")}
        field="abridged"
        onUseCurrent={() => setAbridged(file?.abridged ?? null)}
        status={defaults.abridged.status}
      >
        <div className="flex items-center gap-2">
          <Checkbox
            checked={abridged === true}
            disabled={isDisabled("abridged")}
            id="identify-abridged"
            onCheckedChange={(checked) => setAbridged(checked ? true : null)}
          />
          <Label
            className="cursor-pointer font-normal text-muted-foreground"
            htmlFor="identify-abridged"
          >
            This is an abridged edition
          </Label>
        </div>
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
                .map((id) => {
                  const label =
                    availableIdentifierTypes.find((t) => t.id === id.type)
                      ?.label ?? id.type;
                  return `${label}: ${id.value}`;
                })
                .join(", ")}
            </CurrentBar>
          )}
        <IdentifierEditor
          identifierTypes={availableIdentifierTypes}
          onChange={setIdentifiers}
          status={(row) =>
            currentIdentifiers.some(
              (c) => c.type === row.type && c.value === row.value,
            )
              ? "unchanged"
              : "new"
          }
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
