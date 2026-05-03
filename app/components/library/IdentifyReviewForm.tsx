import equal from "fast-deep-equal";
import {
  ArrowLeft,
  ChevronDown,
  ChevronUp,
  Copy,
  ExternalLink,
  Loader2,
  RefreshCcw,
  X,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import { EntityCombobox } from "@/components/common/EntityCombobox";
import { IdentifierEditor } from "@/components/common/IdentifierEditor";
import { SortableEntityList } from "@/components/common/SortableEntityList";
import { StatusBadge } from "@/components/common/StatusBadge";
import { ExtractSubtitleButton } from "@/components/library/ExtractSubtitleButton";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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
import {
  useGenreSearch,
  useImprintSearch,
  usePeopleSearch,
  usePublisherSearch,
  useSeriesSearch,
  useTagSearch,
  type NameOption,
  type NameWithBookCount,
  type NameWithFileCount,
} from "@/hooks/queries/entity-search";
import {
  usePluginApply,
  usePluginIdentifierTypes,
  usePluginsInstalled,
  type PluginApplyPayload,
  type PluginSearchResult,
} from "@/hooks/queries/plugins";
import { cn, isPageBasedFileType } from "@/libraries/utils";
import { AuthorRoleWriter, FileTypeCBZ, type Book, type File } from "@/types";
import { AUTHOR_ROLES, getAuthorRoleLabel } from "@/utils/authorRoles";
import { formatDuration, formatMetadataFieldLabel } from "@/utils/format";
import { getPrimaryFileType } from "@/utils/primaryFile";
import { formatSeriesNumber } from "@/utils/seriesNumber";

import {
  aggregateDecisions,
  defaultDecision,
  type FieldScope,
} from "./identify-decisions";
import {
  resolveIdentifiers,
  type FieldStatus,
  type IdentifierEntry,
} from "./identify-utils";
import { IdentifySectionBanner } from "./IdentifySectionBanner";
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

interface SeriesEntry {
  name: string;
  number: string;
  unit: "" | "volume" | "chapter";
}

type BookFieldKey =
  | "title"
  | "subtitle"
  | "authors"
  | "series"
  | "genres"
  | "tags"
  | "description";
type FileFieldKey =
  | "cover"
  | "name"
  | "narrators"
  | "publisher"
  | "imprint"
  | "language"
  | "release_date"
  | "url"
  | "identifiers"
  | "abridged";
type FieldKey = BookFieldKey | FileFieldKey;

const BOOK_FIELDS: BookFieldKey[] = [
  "title",
  "subtitle",
  "authors",
  "series",
  "genres",
  "tags",
  "description",
];
const FILE_FIELDS: FileFieldKey[] = [
  "cover",
  "name",
  "narrators",
  "publisher",
  "imprint",
  "language",
  "release_date",
  "url",
  "identifiers",
  "abridged",
];

// Some plugin disabled-field keys are camelCase (releaseDate); accept both.
const PLUGIN_FIELD_ALIASES: Record<FieldKey, string[]> = {
  release_date: ["release_date", "releaseDate"],
  // No other aliases needed today; keep the lookup explicit for future fields.
  title: ["title"],
  subtitle: ["subtitle"],
  authors: ["authors"],
  series: ["series"],
  genres: ["genres"],
  tags: ["tags"],
  description: ["description"],
  cover: ["cover"],
  name: ["name"],
  narrators: ["narrators"],
  publisher: ["publisher"],
  imprint: ["imprint"],
  language: ["language"],
  url: ["url"],
  identifiers: ["identifiers"],
  abridged: ["abridged"],
};

function fieldScope(k: FieldKey): FieldScope {
  return (BOOK_FIELDS as string[]).includes(k) ? "book" : "file";
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Load natural dimensions of an image URL. See identify-cover gating notes
 * in the previous implementation: `dims` is non-null only on successful
 * load, `settled` flips true once the load attempt finishes (success OR
 * error). Callers gate the cover picker on `settled` so the default
 * selection doesn't flip after the picker mounts. */
function useImageDimensions(src: string | undefined) {
  const [state, setState] = useState<{
    dims: { w: number; h: number } | null;
    settled: boolean;
  }>(() => ({ dims: null, settled: !src }));

  useEffect(() => {
    if (!src) {
      setState({ dims: null, settled: true });
      return;
    }
    setState({ dims: null, settled: false });
    let cancelled = false;
    const img = new Image();
    img.onload = () => {
      if (!cancelled) {
        setState({
          dims: { w: img.naturalWidth, h: img.naturalHeight },
          settled: true,
        });
      }
    };
    img.onerror = () => {
      if (!cancelled) setState({ dims: null, settled: true });
    };
    img.src = src;
    return () => {
      cancelled = true;
    };
  }, [src]);

  return state;
}

function resolveScalar(
  current: string | undefined | null,
  incoming: string | undefined | null,
): { value: string; status: FieldStatus } {
  const cur = current?.trim() ?? "";
  const inc = incoming?.trim() ?? "";

  if (!cur && inc) return { value: inc, status: "new" };
  if (cur && !inc) return { value: cur, status: "unchanged" };
  if (cur === inc) return { value: cur, status: "unchanged" };
  return { value: inc, status: "changed" };
}

function resolveAbridged(
  current: boolean | undefined | null,
  incoming: boolean | undefined | null,
): { value: boolean | null; status: FieldStatus } {
  const cur = current ?? null;
  const inc = incoming ?? null;

  if (cur === null && inc !== null) return { value: inc, status: "new" };
  if (cur !== null && inc === null) return { value: cur, status: "unchanged" };
  if (cur === inc) return { value: cur, status: "unchanged" };
  return { value: inc, status: "changed" };
}

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

function resolveSeries(
  current: SeriesEntry[],
  incoming: SeriesEntry[],
): { value: SeriesEntry[]; status: FieldStatus } {
  if (current.length === 0 && incoming.length > 0)
    return { value: incoming, status: "new" };
  if (current.length > 0 && incoming.length === 0)
    return { value: current, status: "unchanged" };
  const key = (s: SeriesEntry) => `${s.name}|${s.number}|${s.unit}`;
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

function findFile(book: Book, fileId?: number): File | undefined {
  if (fileId) return book.files?.find((f) => f.id === fileId);
  // Fall back to the first MAIN file rather than the first file overall, so
  // a book whose first file is a supplement doesn't surface supplement
  // metadata (Name, narrators, identifiers) into the identify dialog.
  return book.files?.find((f) => f.file_role === "main") ?? book.files?.[0];
}

// ---------------------------------------------------------------------------
// Sub-components (inline, single-use)
// ---------------------------------------------------------------------------

function FieldRow({
  label,
  status,
  decision,
  onDecisionChange,
  disabled,
  hidden,
  currentValue,
  inlineAction,
  children,
}: {
  label: string;
  status: FieldStatus;
  decision: boolean;
  onDecisionChange: (v: boolean) => void;
  disabled?: boolean;
  /** When true the row renders nothing (used by the Changed/All filter
   * to suppress unchanged rows in "Changed" mode). */
  hidden?: boolean;
  currentValue?: React.ReactNode;
  inlineAction?: React.ReactNode;
  children: React.ReactNode;
}) {
  if (hidden) return null;
  const effectiveStatus: FieldStatus = disabled ? "unchanged" : status;
  return (
    <div
      className={cn(
        "grid grid-cols-[24px_minmax(0,1fr)] gap-3.5 border-b px-5 py-4 last:border-b-0",
        disabled && "opacity-50",
        !decision && !disabled && "opacity-60",
      )}
    >
      <div className="pt-0.5">
        <Checkbox
          aria-label={`Apply ${label}`}
          checked={decision && !disabled}
          disabled={disabled}
          onCheckedChange={(v) => onDecisionChange(v === true)}
        />
      </div>
      <div className="min-w-0 space-y-2">
        <div className="flex h-6 items-center gap-2">
          <Label className="text-sm font-semibold">{label}</Label>
          <StatusBadge status={effectiveStatus} />
          <div className="ml-auto shrink-0">{inlineAction}</div>
        </div>
        <div
          aria-disabled={!decision || disabled}
          className={cn(
            "space-y-2",
            (!decision || disabled) && "pointer-events-none",
            disabled && "opacity-50",
          )}
        >
          {children}
        </div>
        {currentValue != null && effectiveStatus !== "unchanged" && (
          <p className="text-xs text-muted-foreground/70">
            <span className="font-medium">Currently:</span>{" "}
            <span className="text-foreground/60">{currentValue}</span>
          </p>
        )}
      </div>
    </div>
  );
}

function CollapsibleCurrentText({ text }: { text: string }) {
  const [expanded, setExpanded] = useState(false);
  return (
    <div className="text-xs text-muted-foreground/70 space-y-1">
      <p className="font-medium">Currently:</p>
      <span
        className={cn(
          "block text-foreground/60 whitespace-pre-line",
          !expanded && "line-clamp-2",
        )}
      >
        {text}
      </span>
      <button
        className="inline-flex items-center gap-1 text-primary hover:underline cursor-pointer"
        onClick={() => setExpanded(!expanded)}
        type="button"
      >
        {expanded ? (
          <>
            Show less <ChevronUp className="h-3 w-3" />
          </>
        ) : (
          <>
            Show full <ChevronDown className="h-3 w-3" />
          </>
        )}
      </button>
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
  onHasChangesChange,
}: IdentifyReviewFormProps) {
  const file = findFile(book, fileId);
  const primaryFileType = getPrimaryFileType(book);
  const applyMutation = usePluginApply();
  const { data: pluginIdentifierTypes } = usePluginIdentifierTypes();

  const disabledFieldsRaw = useMemo(
    () => new Set(result.disabled_fields ?? []),
    [result.disabled_fields],
  );
  const isDisabled = (field: FieldKey) => {
    const aliases = PLUGIN_FIELD_ALIASES[field];
    return aliases.some((alias) => disabledFieldsRaw.has(alias));
  };

  // Plugin display name for the source pill in the header. Falls back to
  // the plugin id when the installed list hasn't loaded yet or doesn't
  // match (e.g. the plugin was uninstalled mid-flow).
  const { data: installedPlugins } = usePluginsInstalled();
  const pluginDisplayName = useMemo(() => {
    const match = installedPlugins?.find(
      (p) => p.scope === result.plugin_scope && p.id === result.plugin_id,
    );
    return match?.name ?? result.plugin_id;
  }, [installedPlugins, result.plugin_scope, result.plugin_id]);

  const [filterMode, setFilterMode] = useState<"changed" | "all">("changed");

  // The "primary file" gate for book-level changed-field defaults. A book
  // with no explicit primary_file_id and a single MAIN file is treated as
  // primary; this avoids surprising "nothing applies" defaults on freshly
  // scanned single-file books. Supplements never count toward this — a
  // book with one main + one supplement is still effectively single-main.
  const isPrimaryFile = useMemo(() => {
    const mainCount = (book.files ?? []).filter(
      (f) => f.file_role === "main",
    ).length;
    if (book.primary_file_id == null) {
      return mainCount <= 1;
    }
    return file?.id === book.primary_file_id;
  }, [book.primary_file_id, book.files, file?.id]);

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

  const currentSeriesEntries: SeriesEntry[] = useMemo(
    () =>
      (book.book_series ?? []).map((bs) => ({
        name: bs.series?.name ?? "",
        number: bs.series_number?.toString() ?? "",
        unit: (bs.series_number_unit ?? "") as SeriesEntry["unit"],
      })),
    [book.book_series],
  );

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

    const incomingSeries: SeriesEntry[] = result.series
      ? [
          {
            name: result.series,
            number: result.series_number?.toString() ?? "",
            unit: (result.series_number_unit ?? "") as SeriesEntry["unit"],
          },
        ]
      : [];

    return {
      title: resolveScalar(book.title, result.title),
      subtitle: resolveScalar(book.subtitle, result.subtitle),
      description: resolveScalar(book.description, result.description),
      authors: resolveAuthors(currentAuthors, incomingAuthors),
      narrators: resolveArray(currentNarrators, result.narrators ?? []),
      series: resolveSeries(currentSeriesEntries, incomingSeries),
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
    currentSeriesEntries,
    currentGenres,
    currentTags,
    currentIdentifiers,
    file,
  ]);

  // ---- Form state (values) ----
  const [title, setTitle] = useState(defaults.title.value);
  const [subtitle, setSubtitle] = useState(defaults.subtitle.value);
  const [description, setDescription] = useState(defaults.description.value);
  const [authors, setAuthors] = useState<AuthorEntry[]>(defaults.authors.value);
  const [narrators, setNarrators] = useState<string[]>(
    defaults.narrators.value,
  );
  const narratorItems = useMemo(
    () => narrators.map((name) => ({ name })),
    [narrators],
  );
  const [seriesEntries, setSeriesEntries] = useState<SeriesEntry[]>(
    defaults.series.value,
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

  // ---- Name field (file.Name) ----
  // Surfaced as a real per-field decision. The default proposed value is
  // the plugin's title (plugins don't model file.Name separately). Source
  // attribution: `"plugin"` when the saved value matches the proposal,
  // `"user"` otherwise.
  const initialName = result.title?.trim() ?? "";
  const [name, setName] = useState(initialName);
  const initialNameStatus: FieldStatus = useMemo(() => {
    const cur = (file?.name ?? "").trim();
    if (!cur && initialName) return "new";
    if (cur && !initialName) return "unchanged";
    if (cur === initialName) return "unchanged";
    return "changed";
  }, [file?.name, initialName]);

  // ---- Identifier types ----
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
  const isFilePageBased = isPageBasedFileType(file?.file_type);
  const isAudiobook = file?.file_type === "m4b";

  // ---- Cover state ----
  const newCoverUrl = !isFilePageBased ? result.cover_url : undefined;
  const newCoverPage =
    isFilePageBased &&
    result.cover_page != null &&
    result.cover_page >= 0 &&
    (file?.page_count == null || result.cover_page < file.page_count)
      ? result.cover_page
      : undefined;
  const newCoverPreviewUrl =
    newCoverUrl ??
    (file && newCoverPage != null
      ? `/api/books/files/${file.id}/page/${newCoverPage}`
      : undefined);
  const currentCoverUrl = file?.cover_image_filename
    ? `/api/books/files/${file.id}/cover?v=${new Date(file.updated_at).getTime()}`
    : undefined;
  const currentCoverPage = file?.cover_page ?? null;
  const isPageBasedCoverChoice = isFilePageBased && newCoverPage != null;
  const currentCover = useImageDimensions(
    isPageBasedCoverChoice ? undefined : currentCoverUrl,
  );
  const newCover = useImageDimensions(
    isPageBasedCoverChoice ? undefined : newCoverPreviewUrl,
  );
  const currentCoverDims = currentCover.dims;
  const newCoverDims = newCover.dims;
  const hasCoverChoice = isPageBasedCoverChoice
    ? !!newCoverPreviewUrl
    : !!newCoverDims && currentCover.settled;
  const preferCurrentCover = isPageBasedCoverChoice
    ? !!currentCoverUrl &&
      currentCoverPage !== null &&
      currentCoverPage === newCoverPage
    : !!currentCoverDims &&
      !!newCoverDims &&
      currentCoverDims.w * currentCoverDims.h >=
        newCoverDims.w * newCoverDims.h;
  const defaultCoverSelection: "current" | "new" =
    hasCoverChoice && !isDisabled("cover") && !preferCurrentCover
      ? "new"
      : "current";
  const [userCoverSelection, setUserCoverSelection] = useState<
    "current" | "new" | null
  >(null);
  const coverSelection: "current" | "new" =
    userCoverSelection ?? defaultCoverSelection;

  // ---- Initial field statuses (plugin vs saved — for default decisions) ----
  const initialFieldStatus: Record<FieldKey, FieldStatus> = useMemo(() => {
    const seriesStatus: FieldStatus = defaults.series.status;

    const coverStatus: FieldStatus =
      hasCoverChoice && !preferCurrentCover
        ? currentCoverUrl
          ? "changed"
          : "new"
        : "unchanged";

    return {
      title: defaults.title.status,
      subtitle: defaults.subtitle.status,
      authors: defaults.authors.status,
      series: seriesStatus,
      genres: defaults.genres.status,
      tags: defaults.tags.status,
      description: defaults.description.status,
      cover: coverStatus,
      name: initialNameStatus,
      narrators: defaults.narrators.status,
      publisher: defaults.publisher.status,
      imprint: defaults.imprint.status,
      language: defaults.language.status,
      release_date: defaults.releaseDate.status,
      url: defaults.url.status,
      identifiers: defaults.identifiers.status,
      abridged: defaults.abridged.status,
    };
  }, [
    defaults,
    hasCoverChoice,
    preferCurrentCover,
    currentCoverUrl,
    initialNameStatus,
  ]);

  // ---- Live field statuses (form state vs saved — updates as user edits) ----
  const fieldStatus: Record<FieldKey, FieldStatus> = useMemo(() => {
    const scalarStatus = (
      saved: string | undefined | null,
      current: string,
    ): FieldStatus => {
      const s = saved?.trim() ?? "";
      const c = current.trim();
      if (!s && c) return "new";
      if (s && !c) return "unchanged";
      if (s === c) return "unchanged";
      return "changed";
    };

    const seriesKey = (s: SeriesEntry) =>
      `${s.name.trim()}|${s.number.trim()}|${s.unit}`;
    const seriesSavedKeys = currentSeriesEntries.map(seriesKey).sort();
    const seriesCurrentKeys = seriesEntries.map(seriesKey).sort();
    const seriesMatch =
      seriesSavedKeys.length === seriesCurrentKeys.length &&
      seriesSavedKeys.every((v, i) => v === seriesCurrentKeys[i]);
    const seriesStatus: FieldStatus = seriesMatch
      ? "unchanged"
      : currentSeriesEntries.length === 0 && seriesEntries.length > 0
        ? "new"
        : "changed";

    const arrayStatus = (saved: string[], current: string[]): FieldStatus => {
      if (saved.length === 0 && current.length > 0) return "new";
      if (saved.length > 0 && current.length === 0) return "unchanged";
      const s = [...saved].sort();
      const c = [...current].sort();
      if (s.length === c.length && s.every((v, i) => v === c[i]))
        return "unchanged";
      return "changed";
    };

    const authorsStatus = (): FieldStatus => {
      if (currentAuthors.length === 0 && authors.length > 0) return "new";
      if (currentAuthors.length > 0 && authors.length === 0) return "unchanged";
      const key = (a: AuthorEntry) => `${a.name}|${a.role ?? ""}`;
      const s = currentAuthors.map(key).sort();
      const c = authors.map(key).sort();
      if (s.length === c.length && s.every((v, i) => v === c[i]))
        return "unchanged";
      return "changed";
    };

    const abridgedStatus = (): FieldStatus => {
      const saved = file?.abridged ?? null;
      if (saved === null && abridged !== null) return "new";
      if (saved !== null && abridged === null) return "unchanged";
      if (saved === abridged) return "unchanged";
      return "changed";
    };

    const identifiersStatus = (): FieldStatus => {
      if (currentIdentifiers.length === 0 && identifiers.length > 0)
        return "new";
      if (currentIdentifiers.length > 0 && identifiers.length === 0)
        return "unchanged";
      const key = (id: IdentifierEntry) => `${id.type}|${id.value}`;
      const s = currentIdentifiers.map(key).sort();
      const c = identifiers.map(key).sort();
      if (s.length === c.length && s.every((v, i) => v === c[i]))
        return "unchanged";
      return "changed";
    };

    const coverStatus: FieldStatus =
      hasCoverChoice && coverSelection === "new"
        ? currentCoverUrl
          ? "changed"
          : "new"
        : "unchanged";

    return {
      title: scalarStatus(book.title, title),
      subtitle: scalarStatus(book.subtitle, subtitle),
      authors: authorsStatus(),
      series: seriesStatus,
      genres: arrayStatus(currentGenres, genres),
      tags: arrayStatus(currentTags, tags),
      description: scalarStatus(book.description, description),
      cover: coverStatus,
      name: scalarStatus(file?.name, name),
      narrators: arrayStatus(currentNarrators, narrators),
      publisher: scalarStatus(file?.publisher?.name, publisher),
      imprint: scalarStatus(file?.imprint?.name, imprint),
      language: scalarStatus(file?.language, language),
      release_date: scalarStatus(
        file?.release_date ? file.release_date.split("T")[0] : undefined,
        releaseDate,
      ),
      url: scalarStatus(file?.url, url),
      identifiers: identifiersStatus(),
      abridged: abridgedStatus(),
    };
  }, [
    book,
    file,
    title,
    subtitle,
    description,
    authors,
    narrators,
    seriesEntries,
    genres,
    tags,
    publisher,
    imprint,
    releaseDate,
    url,
    language,
    abridged,
    identifiers,
    name,
    currentAuthors,
    currentNarrators,
    currentSeriesEntries,
    currentGenres,
    currentTags,
    currentIdentifiers,
    hasCoverChoice,
    coverSelection,
    currentCoverUrl,
  ]);

  // ---- Decision state ----
  const initialDecisions: Record<FieldKey, boolean> = useMemo(() => {
    const out = {} as Record<FieldKey, boolean>;
    for (const k of [...BOOK_FIELDS, ...FILE_FIELDS] as FieldKey[]) {
      if (isDisabled(k)) {
        out[k] = false;
        continue;
      }
      out[k] = defaultDecision({
        scope: fieldScope(k),
        status: initialFieldStatus[k],
        isPrimaryFile,
      });
    }
    return out;
    // isDisabled is stable for the component's lifetime (closes over
    // disabledFieldsRaw which is derived from result.disabled_fields).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialFieldStatus, isPrimaryFile]);

  const [decisions, setDecisions] =
    useState<Record<FieldKey, boolean>>(initialDecisions);

  // The cover image dimensions load asynchronously, so `hasCoverChoice` and
  // therefore `fieldStatus.cover` can flip from "unchanged" to "new"/"changed"
  // after the dialog has already mounted. The initial `decisions.cover` was
  // computed against the pre-load state (false), leaving the user with an
  // unchecked cover row that should have defaulted ON. When the cover row
  // becomes available and the user hasn't explicitly chosen yet, sync the
  // decision to match the smart default.
  const hasCoverChoiceRef = useRef(hasCoverChoice);
  useEffect(() => {
    if (
      !hasCoverChoiceRef.current &&
      hasCoverChoice &&
      !isDisabled("cover") &&
      userCoverSelection === null
    ) {
      const desired = defaultDecision({
        scope: "file",
        status: initialFieldStatus.cover,
        isPrimaryFile,
      });
      setDecisions((prev) =>
        prev.cover === desired ? prev : { ...prev, cover: desired },
      );
    }
    hasCoverChoiceRef.current = hasCoverChoice;
    // isDisabled is recreated each render; the closure captures its current
    // value at effect commit, which is what we want. Intentionally omitted
    // from deps to avoid re-running on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    hasCoverChoice,
    initialFieldStatus.cover,
    isPrimaryFile,
    userCoverSelection,
  ]);

  const setDecision = (k: FieldKey, v: boolean) => {
    if (isDisabled(k)) return;
    setDecisions((prev) => ({ ...prev, [k]: v }));
  };

  const setSectionDecisions = (keys: FieldKey[], v: boolean) => {
    setDecisions((prev) => {
      const next = { ...prev };
      for (const k of keys) {
        if (!isDisabled(k)) next[k] = v;
      }
      return next;
    });
  };

  // ---- Section / Apply-all aggregations ----
  const visibleFileFields = useMemo(() => {
    // Narrators only render for audiobooks; exclude from counts otherwise.
    return FILE_FIELDS.filter((k) => {
      if (k === "narrators") return isAudiobook;
      if (k === "cover") return hasCoverChoice;
      return true;
    });
  }, [isAudiobook, hasCoverChoice]);

  const bookApplicableKeys = useMemo(
    () => BOOK_FIELDS.filter((k) => !isDisabled(k)),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [disabledFieldsRaw],
  );
  const fileApplicableKeys = useMemo(
    () => visibleFileFields.filter((k) => !isDisabled(k)),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [visibleFileFields, disabledFieldsRaw],
  );
  const allApplicableKeys: FieldKey[] = useMemo(
    () => [...bookApplicableKeys, ...fileApplicableKeys],
    [bookApplicableKeys, fileApplicableKeys],
  );

  const bookVisibleKeys = useMemo(
    () =>
      filterMode === "all"
        ? bookApplicableKeys
        : bookApplicableKeys.filter((k) => fieldStatus[k] !== "unchanged"),
    [filterMode, bookApplicableKeys, fieldStatus],
  );
  const fileVisibleKeys = useMemo(
    () =>
      filterMode === "all"
        ? fileApplicableKeys
        : fileApplicableKeys.filter((k) => fieldStatus[k] !== "unchanged"),
    [filterMode, fileApplicableKeys, fieldStatus],
  );
  const allVisibleKeys: FieldKey[] = useMemo(
    () => [...bookVisibleKeys, ...fileVisibleKeys],
    [bookVisibleKeys, fileVisibleKeys],
  );

  const bookSelectedCount = bookVisibleKeys.filter((k) => decisions[k]).length;
  const fileSelectedCount = fileVisibleKeys.filter((k) => decisions[k]).length;
  const totalSelected = bookSelectedCount + fileSelectedCount;
  const totalApplicable = allVisibleKeys.length;

  const bookCheckboxState = aggregateDecisions(
    bookVisibleKeys.map((k) => decisions[k]),
  );
  const fileCheckboxState = aggregateDecisions(
    fileVisibleKeys.map((k) => decisions[k]),
  );
  const globalCheckboxState = aggregateDecisions(
    allVisibleKeys.map((k) => decisions[k]),
  );

  // ---- Section collapse state (initial: collapsed iff selected count is 0) ----
  const initialBookCollapsed =
    bookApplicableKeys.length > 0 &&
    bookApplicableKeys.every((k) => !initialDecisions[k]);
  const initialFileCollapsed =
    fileApplicableKeys.length > 0 &&
    fileApplicableKeys.every((k) => !initialDecisions[k]);
  const [bookCollapsed, setBookCollapsed] = useState(initialBookCollapsed);
  const [fileCollapsed, setFileCollapsed] = useState(initialFileCollapsed);

  // ---- File section hint ----
  const fileSectionHint = useMemo(() => {
    if (!file) return null;
    const parts: string[] = [];
    parts.push(file.file_type.toUpperCase());
    const trimmedName = file.name?.trim();
    if (trimmedName) parts.push(trimmedName);
    if (file.audiobook_duration_seconds != null) {
      parts.push(formatDuration(file.audiobook_duration_seconds));
    }
    if (file.audiobook_bitrate_bps != null) {
      parts.push(`${Math.round(file.audiobook_bitrate_bps / 1000)} kbps`);
    }
    if (file.page_count != null) {
      parts.push(`${file.page_count} pages`);
    }
    return parts.join(" · ");
  }, [file]);

  // ---- Restore suggestions ----
  const restoreSuggestions = () => {
    setDecisions(initialDecisions);
    setBookCollapsed(initialBookCollapsed);
    setFileCollapsed(initialFileCollapsed);
    setTitle(defaults.title.value);
    setSubtitle(defaults.subtitle.value);
    setDescription(defaults.description.value);
    setAuthors(defaults.authors.value);
    setNarrators(defaults.narrators.value);
    setSeriesEntries(defaults.series.value);
    setGenres(defaults.genres.value);
    setTags(defaults.tags.value);
    setPublisher(defaults.publisher.value);
    setImprint(defaults.imprint.value);
    setReleaseDate(defaults.releaseDate.value);
    setUrl(defaults.url.value);
    setLanguage(defaults.language.value);
    setAbridged(defaults.abridged.value);
    setIdentifiers(defaults.identifiers.value);
    setUserCoverSelection(null);
    setName(initialName);
  };

  // ---- Unsaved changes tracking ----
  const hasChanges = useMemo(() => {
    if (name !== initialName) return true;
    for (const k of [...BOOK_FIELDS, ...FILE_FIELDS] as FieldKey[]) {
      if (decisions[k] !== initialDecisions[k]) return true;
    }
    return (
      title !== defaults.title.value ||
      subtitle !== defaults.subtitle.value ||
      description !== defaults.description.value ||
      !equal(authors, defaults.authors.value) ||
      !equal(narrators, defaults.narrators.value) ||
      !equal(seriesEntries, defaults.series.value) ||
      !equal(genres, defaults.genres.value) ||
      !equal(tags, defaults.tags.value) ||
      publisher !== defaults.publisher.value ||
      imprint !== defaults.imprint.value ||
      releaseDate !== defaults.releaseDate.value ||
      url !== defaults.url.value ||
      language !== defaults.language.value ||
      abridged !== defaults.abridged.value ||
      !equal(identifiers, defaults.identifiers.value) ||
      coverSelection !== defaultCoverSelection
    );
  }, [
    name,
    initialName,
    decisions,
    initialDecisions,
    title,
    subtitle,
    description,
    authors,
    narrators,
    seriesEntries,
    genres,
    tags,
    publisher,
    imprint,
    releaseDate,
    url,
    language,
    abridged,
    identifiers,
    coverSelection,
    defaults,
    defaultCoverSelection,
  ]);

  useEffect(() => {
    onHasChangesChange?.(hasChanges);
  }, [hasChanges, onHasChangesChange]);

  // ---- Submit ----
  const handleSubmit = async () => {
    const fields: Record<string, unknown> = {};
    if (decisions.title) fields.title = title;
    if (decisions.subtitle) fields.subtitle = subtitle;
    if (decisions.description) fields.description = description;
    if (decisions.authors) {
      fields.authors = authors.map((a) => ({ name: a.name, role: a.role }));
    }
    if (decisions.narrators) fields.narrators = narrators;
    if (decisions.series) {
      fields.series = seriesEntries
        .filter((s) => s.name.trim())
        .map((s) => ({
          name: s.name,
          number: s.number ? parseFloat(s.number) : undefined,
          series_number_unit: s.unit !== "" ? s.unit : undefined,
        }));
    }
    if (decisions.genres) fields.genres = genres;
    if (decisions.tags) fields.tags = tags;
    if (decisions.publisher) fields.publisher = publisher;
    if (decisions.imprint) fields.imprint = imprint;
    if (decisions.release_date) fields.release_date = releaseDate;
    if (decisions.url) fields.url = url;
    if (decisions.language) fields.language = language;
    if (decisions.abridged && abridged !== null) fields.abridged = abridged;
    if (decisions.identifiers) {
      fields.identifiers = identifiers.map((id) => ({
        type: id.type,
        value: id.value,
      }));
    }
    if (decisions.cover && coverSelection === "new") {
      if (newCoverUrl) {
        fields.cover_url = newCoverUrl;
      } else if (newCoverPage != null) {
        fields.cover_page = newCoverPage;
      }
    }

    const payload: PluginApplyPayload = {
      book_id: book.id,
      file_id: fileId,
      fields,
      plugin_scope: result.plugin_scope,
      plugin_id: result.plugin_id,
    };

    if (decisions.name && name.trim()) {
      payload.file_name = name;
      payload.file_name_source = name === initialName ? "plugin" : "user";
    }

    try {
      await applyMutation.mutateAsync(payload);
      toast.success(
        `Updated ${totalSelected} field${totalSelected === 1 ? "" : "s"}.`,
      );
      onClose();
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "Failed to apply metadata.";
      toast.error(message);
    }
  };

  // ---- Render rows ----
  const titleInlineAction =
    !isDisabled("title") && !isDisabled("subtitle") ? (
      <ExtractSubtitleButton
        onExtract={(t, s) => {
          setTitle(t);
          setSubtitle(s);
          if (!decisions.subtitle) setDecision("subtitle", true);
        }}
        title={title}
      />
    ) : null;

  const nameInlineAction =
    !isDisabled("name") && title.trim() && title !== name ? (
      <Button
        className="h-6 px-2 text-xs"
        onClick={() => setName(title)}
        size="sm"
        type="button"
        variant="ghost"
      >
        <Copy className="h-3 w-3" />
        Copy from book title
      </Button>
    ) : null;

  // Header subtitle bits
  const mainFileCount = (book.files ?? []).filter(
    (f) => f.file_role === "main",
  ).length;
  const proposedChangesCount = allApplicableKeys.filter(
    (k) => fieldStatus[k] !== "unchanged",
  ).length;

  // "Changed" / "All" filter: only changes which rows render. Disabled
  // rows are treated as unchanged for this purpose.
  const isRowVisible = (k: FieldKey) => {
    if (filterMode === "all") return true;
    if (isDisabled(k)) return false;
    return fieldStatus[k] !== "unchanged";
  };

  return (
    <>
      <DialogHeader className="flex-row items-center gap-3 pl-10">
        <DialogTitle className="sr-only">Identify {book.title}</DialogTitle>
        <DialogDescription className="sr-only">
          Review and apply metadata from {pluginDisplayName}
        </DialogDescription>
        {/* Back button mirrors the close button's positioning (absolute, same
            offset and styling) so they appear symmetric across the header. */}
        <button
          aria-label="Back"
          className="absolute left-3 top-1/2 -translate-y-1/2 rounded-sm opacity-70 ring-offset-background transition-opacity hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 cursor-pointer"
          onClick={onBack}
          type="button"
        >
          <ArrowLeft className="h-4 w-4" />
        </button>
        <div className="min-w-0 flex-1">
          <h3 aria-hidden className="truncate text-sm font-semibold">
            Identify {book.title}
          </h3>
          <p className="truncate text-xs text-muted-foreground">
            {mainFileCount} file{mainFileCount === 1 ? "" : "s"} ·{" "}
            {proposedChangesCount} change
            {proposedChangesCount === 1 ? "" : "s"} proposed
          </p>
        </div>
        {/* Source pill */}
        <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-primary/30 bg-primary/15 px-2.5 py-1 text-[11px] font-semibold text-primary">
          <span className="h-1.5 w-1.5 rounded-full bg-primary shadow-[0_0_8px_var(--primary)]" />
          {pluginDisplayName}
        </span>
      </DialogHeader>

      {/* Select-all bar — sits outside the scroll body so there is no gap
          between the dialog header and this bar. */}
      <div className="flex shrink-0 items-center gap-3.5 border-b bg-muted px-5 py-2.5">
        <Checkbox
          aria-label="Apply all"
          checked={globalCheckboxState}
          onCheckedChange={(v) =>
            setSectionDecisions(allVisibleKeys, v === true)
          }
        />
        <span className="text-xs font-medium">Apply all</span>
        <span className="whitespace-nowrap text-[11.5px] tabular-nums text-muted-foreground">
          <span className="font-semibold text-foreground">{totalSelected}</span>{" "}
          of {totalApplicable} selected
        </span>
        <div className="ml-auto flex items-center gap-1 rounded-md bg-background p-0.5">
          <button
            aria-pressed={filterMode === "changed"}
            className={cn(
              "cursor-pointer rounded px-2 py-1 text-[11px] font-medium transition-colors",
              filterMode === "changed"
                ? "bg-muted text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
            onClick={() => setFilterMode("changed")}
            type="button"
          >
            Changed
          </button>
          <button
            aria-pressed={filterMode === "all"}
            className={cn(
              "cursor-pointer rounded px-2 py-1 text-[11px] font-medium transition-colors",
              filterMode === "all"
                ? "bg-muted text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
            onClick={() => setFilterMode("all")}
            type="button"
          >
            All
          </button>
        </div>
      </div>

      {/* Scroll body — section banners stick to top-0 since the select-all
          bar is outside the scroll container. */}
      <div className="relative min-h-0 flex-1 overflow-y-auto">
        {/* Book section */}
        {bookVisibleKeys.length > 0 && (
          <>
            <IdentifySectionBanner
              checkboxState={bookCheckboxState}
              collapsed={bookCollapsed}
              hint="applies to all files"
              label="BOOK"
              onCheckedChange={(v) => setSectionDecisions(bookVisibleKeys, v)}
              onToggleCollapse={() => setBookCollapsed((c) => !c)}
              selectedCount={bookSelectedCount}
              totalCount={bookVisibleKeys.length}
            />
            {!bookCollapsed && (
              <div id="identify-section-book">
                {/* Title */}
                <FieldRow
                  currentValue={book.title || undefined}
                  decision={decisions.title}
                  disabled={isDisabled("title")}
                  hidden={!isRowVisible("title")}
                  inlineAction={titleInlineAction}
                  label={formatMetadataFieldLabel("title")}
                  onDecisionChange={(v) => setDecision("title", v)}
                  status={fieldStatus.title}
                >
                  <Input
                    disabled={isDisabled("title")}
                    onChange={(e) => setTitle(e.target.value)}
                    value={title}
                  />
                </FieldRow>

                {/* Subtitle */}
                <FieldRow
                  currentValue={book.subtitle || undefined}
                  decision={decisions.subtitle}
                  disabled={isDisabled("subtitle")}
                  hidden={!isRowVisible("subtitle")}
                  label={formatMetadataFieldLabel("subtitle")}
                  onDecisionChange={(v) => setDecision("subtitle", v)}
                  status={fieldStatus.subtitle}
                >
                  <Input
                    disabled={isDisabled("subtitle")}
                    onChange={(e) => setSubtitle(e.target.value)}
                    value={subtitle}
                  />
                </FieldRow>

                {/* Authors */}
                <FieldRow
                  currentValue={
                    currentAuthors.length > 0
                      ? currentAuthors
                          .map((a) => {
                            const role = getAuthorRoleLabel(a.role);
                            return role ? `${a.name} (${role})` : a.name;
                          })
                          .join(", ")
                      : undefined
                  }
                  decision={decisions.authors}
                  disabled={isDisabled("authors")}
                  hidden={!isRowVisible("authors")}
                  label={formatMetadataFieldLabel("authors")}
                  onDecisionChange={(v) => setDecision("authors", v)}
                  status={fieldStatus.authors}
                >
                  <SortableEntityList<AuthorEntry>
                    comboboxProps={{
                      getOptionKey: (p) => p.name,
                      getOptionLabel: (p) => p.name,
                      getOptionDescription: (p) => {
                        const c = (
                          p as AuthorEntry & { authored_book_count?: number }
                        ).authored_book_count;
                        return c != null
                          ? `${c} ${c === 1 ? "book" : "books"}`
                          : undefined;
                      },
                      hook: function useAuthorOptions(q) {
                        return usePeopleSearch(book.library_id, true, q);
                      },
                      label: "Author",
                    }}
                    items={authors}
                    onAppend={(next) => {
                      const n = "__create" in next ? next.__create : next.name;
                      if (!n.trim()) return;
                      // Case-insensitive duplicate check.
                      if (
                        authors.some(
                          (a) => a.name.toLowerCase() === n.toLowerCase(),
                        )
                      ) {
                        return;
                      }
                      const role = isCbz ? AuthorRoleWriter : undefined;
                      setAuthors([...authors, { name: n, role }]);
                    }}
                    onRemove={(idx) =>
                      setAuthors(authors.filter((_, i) => i !== idx))
                    }
                    onReorder={setAuthors}
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
                                  <SelectItem
                                    className="cursor-pointer"
                                    value="none"
                                  >
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
                    status={undefined}
                  />
                </FieldRow>

                {/* Series */}
                <FieldRow
                  currentValue={
                    currentSeriesEntries.length > 0
                      ? currentSeriesEntries
                          .map(
                            (s) =>
                              `${s.name}${
                                s.number
                                  ? ` ${formatSeriesNumber(parseFloat(s.number), s.unit || null, primaryFileType)}`
                                  : ""
                              }`,
                          )
                          .join(", ")
                      : undefined
                  }
                  decision={decisions.series}
                  disabled={isDisabled("series")}
                  hidden={!isRowVisible("series")}
                  label={formatMetadataFieldLabel("series")}
                  onDecisionChange={(v) => setDecision("series", v)}
                  status={fieldStatus.series}
                >
                  <SortableEntityList<SeriesEntry>
                    comboboxProps={{
                      getOptionKey: (s) => s.name,
                      getOptionLabel: (s) => s.name,
                      getOptionDescription: (s) => {
                        const c = (s as SeriesEntry & { book_count?: number })
                          .book_count;
                        return c != null
                          ? `${c} ${c === 1 ? "book" : "books"}`
                          : undefined;
                      },
                      hook: function useSeriesOptions(q) {
                        const result = useSeriesSearch(
                          book.library_id,
                          true,
                          q,
                        );
                        return result as {
                          data?: SeriesEntry[];
                          isLoading: boolean;
                        };
                      },
                      label: "Series",
                    }}
                    items={seriesEntries}
                    onAppend={(next) => {
                      const name =
                        "__create" in next ? next.__create : next.name;
                      if (!name.trim()) return;
                      if (seriesEntries.some((s) => s.name === name)) return;
                      setSeriesEntries([
                        ...seriesEntries,
                        { name, number: "", unit: "" },
                      ]);
                    }}
                    onRemove={(index) => {
                      setSeriesEntries(
                        seriesEntries.filter((_, i) => i !== index),
                      );
                    }}
                    onReorder={setSeriesEntries}
                    renderExtras={(entry, idx) => (
                      <>
                        <Input
                          className="w-24"
                          onChange={(e) => {
                            const updated = [...seriesEntries];
                            updated[idx].number = e.target.value;
                            setSeriesEntries(updated);
                          }}
                          placeholder="#"
                          type="number"
                          value={entry.number}
                        />
                        <div className="w-32">
                          <Select
                            onValueChange={(value) => {
                              const updated = [...seriesEntries];
                              updated[idx].unit =
                                value === "unspecified"
                                  ? ""
                                  : (value as "volume" | "chapter");
                              setSeriesEntries(updated);
                            }}
                            value={
                              entry.unit === "" ? "unspecified" : entry.unit
                            }
                          >
                            <SelectTrigger>
                              <SelectValue placeholder="Unit" />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="unspecified">
                                Unspecified
                              </SelectItem>
                              <SelectItem value="volume">Volume</SelectItem>
                              <SelectItem value="chapter">Chapter</SelectItem>
                            </SelectContent>
                          </Select>
                        </div>
                      </>
                    )}
                  />
                </FieldRow>

                {/* Genres */}
                <FieldRow
                  currentValue={
                    currentGenres.length > 0
                      ? currentGenres.join(", ")
                      : undefined
                  }
                  decision={decisions.genres}
                  disabled={isDisabled("genres")}
                  hidden={!isRowVisible("genres")}
                  label={formatMetadataFieldLabel("genres")}
                  onDecisionChange={(v) => setDecision("genres", v)}
                  status={fieldStatus.genres}
                >
                  <MultiSelectCombobox<NameWithBookCount>
                    getOptionCount={(g) => g.book_count}
                    getOptionDescription={(g) =>
                      `${g.book_count} ${g.book_count === 1 ? "book" : "books"}`
                    }
                    getOptionLabel={(g) => g.name}
                    hook={function useGenreOptions(q) {
                      return useGenreSearch(book.library_id, true, q);
                    }}
                    label="Genre"
                    onChange={setGenres}
                    placeholder="Add genres..."
                    values={genres}
                  />
                </FieldRow>

                {/* Tags */}
                <FieldRow
                  currentValue={
                    currentTags.length > 0 ? currentTags.join(", ") : undefined
                  }
                  decision={decisions.tags}
                  disabled={isDisabled("tags")}
                  hidden={!isRowVisible("tags")}
                  label={formatMetadataFieldLabel("tags")}
                  onDecisionChange={(v) => setDecision("tags", v)}
                  status={fieldStatus.tags}
                >
                  <MultiSelectCombobox<NameWithBookCount>
                    getOptionCount={(t) => t.book_count}
                    getOptionDescription={(t) =>
                      `${t.book_count} ${t.book_count === 1 ? "book" : "books"}`
                    }
                    getOptionLabel={(t) => t.name}
                    hook={function useTagOptions(q) {
                      return useTagSearch(book.library_id, true, q);
                    }}
                    label="Tag"
                    onChange={setTags}
                    placeholder="Add tags..."
                    values={tags}
                  />
                </FieldRow>

                {/* Description */}
                <FieldRow
                  decision={decisions.description}
                  disabled={isDisabled("description")}
                  hidden={!isRowVisible("description")}
                  label={formatMetadataFieldLabel("description")}
                  onDecisionChange={(v) => setDecision("description", v)}
                  status={fieldStatus.description}
                >
                  <Textarea
                    className="min-h-[100px]"
                    disabled={isDisabled("description")}
                    onChange={(e) => setDescription(e.target.value)}
                    value={description}
                  />
                  {(book.description ?? "").trim() &&
                    fieldStatus.description !== "unchanged" && (
                      <CollapsibleCurrentText text={book.description ?? ""} />
                    )}
                </FieldRow>
              </div>
            )}
          </>
        )}

        {/* File section */}
        {fileVisibleKeys.length > 0 && (
          <>
            <IdentifySectionBanner
              checkboxState={fileCheckboxState}
              collapsed={fileCollapsed}
              hint={fileSectionHint}
              label="FILE"
              onCheckedChange={(v) => setSectionDecisions(fileVisibleKeys, v)}
              onToggleCollapse={() => setFileCollapsed((c) => !c)}
              selectedCount={fileSelectedCount}
              totalCount={fileVisibleKeys.length}
            />
            {!fileCollapsed && (
              <div id="identify-section-file">
                {/* Cover */}
                {hasCoverChoice && (
                  <FieldRow
                    decision={decisions.cover}
                    disabled={isDisabled("cover")}
                    hidden={!isRowVisible("cover")}
                    label={formatMetadataFieldLabel("cover")}
                    onDecisionChange={(v) => setDecision("cover", v)}
                    status={fieldStatus.cover}
                  >
                    <div className="flex gap-4">
                      {currentCoverUrl && (
                        <button
                          className={cn(
                            "relative cursor-pointer overflow-hidden rounded-md border-2 transition-colors",
                            coverSelection === "current"
                              ? "border-primary"
                              : "border-border hover:border-muted-foreground/50",
                            isDisabled("cover") &&
                              "cursor-not-allowed opacity-60",
                          )}
                          disabled={isDisabled("cover")}
                          onClick={() => {
                            setUserCoverSelection("current");
                          }}
                          type="button"
                        >
                          <img
                            alt="Current cover"
                            className={cn(
                              "w-24 bg-muted object-cover",
                              isAudiobook ? "h-24" : "h-36",
                            )}
                            src={currentCoverUrl}
                          />
                          <span className="absolute inset-x-0 bottom-0 bg-black/60 py-0.5 text-center text-[0.6rem] text-white">
                            Keep current
                          </span>
                        </button>
                      )}
                      <button
                        className={cn(
                          "relative cursor-pointer overflow-hidden rounded-md border-2 transition-colors",
                          coverSelection === "new"
                            ? "border-primary"
                            : "border-border hover:border-muted-foreground/50",
                          isDisabled("cover") &&
                            "cursor-not-allowed opacity-60",
                        )}
                        disabled={isDisabled("cover")}
                        onClick={() => {
                          setUserCoverSelection("new");
                          // Picking "Use new" should apply the cover. Sync
                          // the row checkbox to match. setDecision is a
                          // no-op when isDisabled, so no extra guard needed.
                          setDecision("cover", true);
                        }}
                        type="button"
                      >
                        <img
                          alt="New cover"
                          className={cn(
                            "w-24 bg-muted object-cover",
                            isAudiobook ? "h-24" : "h-36",
                          )}
                          src={newCoverPreviewUrl}
                        />
                        <span className="absolute inset-x-0 bottom-0 bg-black/60 py-0.5 text-center text-[0.6rem] text-white">
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
                              : " "
                            : currentCoverDims
                              ? `${currentCoverDims.w} × ${currentCoverDims.h}`
                              : " "}
                        </span>
                      )}
                      <span className="w-[calc(6rem+4px)] text-center">
                        {isPageBasedCoverChoice
                          ? `Page ${(newCoverPage ?? 0) + 1}`
                          : newCoverDims
                            ? `${newCoverDims.w} × ${newCoverDims.h}`
                            : " "}
                      </span>
                    </div>
                  </FieldRow>
                )}

                {/* Name (file.Name) */}
                <FieldRow
                  currentValue={file?.name || undefined}
                  decision={decisions.name}
                  disabled={isDisabled("name")}
                  hidden={!isRowVisible("name")}
                  inlineAction={nameInlineAction}
                  label="Name"
                  onDecisionChange={(v) => setDecision("name", v)}
                  status={fieldStatus.name}
                >
                  <Input
                    disabled={isDisabled("name")}
                    onChange={(e) => setName(e.target.value)}
                    value={name}
                  />
                </FieldRow>

                {/* Narrators (audiobooks only) */}
                {isAudiobook && (
                  <FieldRow
                    currentValue={
                      currentNarrators.length > 0
                        ? currentNarrators.join(", ")
                        : undefined
                    }
                    decision={decisions.narrators}
                    disabled={isDisabled("narrators")}
                    hidden={!isRowVisible("narrators")}
                    label={formatMetadataFieldLabel("narrators")}
                    onDecisionChange={(v) => setDecision("narrators", v)}
                    status={fieldStatus.narrators}
                  >
                    <SortableEntityList<NameOption>
                      comboboxProps={{
                        getOptionKey: (p) => p.name,
                        getOptionLabel: (p) => p.name,
                        getOptionDescription: (p) => {
                          const c = (
                            p as NameOption & { narrated_file_count?: number }
                          ).narrated_file_count;
                          return c != null
                            ? `${c} ${c === 1 ? "file" : "files"}`
                            : undefined;
                        },
                        hook: function useNarratorOptions(q) {
                          return usePeopleSearch(book.library_id, true, q);
                        },
                        label: "Narrator",
                      }}
                      items={narratorItems}
                      onAppend={(next) => {
                        const n =
                          "__create" in next ? next.__create : next.name;
                        if (!n.trim()) return;
                        // Case-insensitive duplicate check.
                        if (
                          narrators.some(
                            (x) => x.toLowerCase() === n.toLowerCase(),
                          )
                        ) {
                          return;
                        }
                        setNarrators([...narrators, n]);
                      }}
                      onRemove={(idx) =>
                        setNarrators(narrators.filter((_, i) => i !== idx))
                      }
                      onReorder={(next) =>
                        setNarrators(next.map((n) => n.name))
                      }
                      status={undefined}
                    />
                  </FieldRow>
                )}

                {/* Publisher */}
                <FieldRow
                  currentValue={file?.publisher?.name || undefined}
                  decision={decisions.publisher}
                  disabled={isDisabled("publisher")}
                  hidden={!isRowVisible("publisher")}
                  label={formatMetadataFieldLabel("publisher")}
                  onDecisionChange={(v) => setDecision("publisher", v)}
                  status={fieldStatus.publisher}
                >
                  <div className="flex items-center gap-2">
                    <div className="flex-1">
                      <EntityCombobox<NameOption>
                        getOptionDescription={(p) => {
                          const c = (p as NameWithFileCount).file_count;
                          return c != null
                            ? `${c} ${c === 1 ? "file" : "files"}`
                            : undefined;
                        }}
                        getOptionKey={(p) => p.name}
                        getOptionLabel={(p) => p.name}
                        hook={function usePublisherOptions(q) {
                          return usePublisherSearch(book.library_id, true, q);
                        }}
                        label="Publisher"
                        onChange={(next) =>
                          setPublisher(
                            "__create" in next ? next.__create : next.name,
                          )
                        }
                        value={publisher ? { name: publisher } : null}
                      />
                    </div>
                    {publisher && !isDisabled("publisher") && (
                      <Button
                        aria-label="Clear publisher"
                        className="shrink-0 cursor-pointer"
                        onClick={() => setPublisher("")}
                        size="icon"
                        type="button"
                        variant="ghost"
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    )}
                  </div>
                </FieldRow>

                {/* Imprint */}
                <FieldRow
                  currentValue={file?.imprint?.name || undefined}
                  decision={decisions.imprint}
                  disabled={isDisabled("imprint")}
                  hidden={!isRowVisible("imprint")}
                  label={formatMetadataFieldLabel("imprint")}
                  onDecisionChange={(v) => setDecision("imprint", v)}
                  status={fieldStatus.imprint}
                >
                  <div className="flex items-center gap-2">
                    <div className="flex-1">
                      <EntityCombobox<NameOption>
                        getOptionDescription={(p) => {
                          const c = (p as NameWithFileCount).file_count;
                          return c != null
                            ? `${c} ${c === 1 ? "file" : "files"}`
                            : undefined;
                        }}
                        getOptionKey={(p) => p.name}
                        getOptionLabel={(p) => p.name}
                        hook={function useImprintOptions(q) {
                          return useImprintSearch(book.library_id, true, q);
                        }}
                        label="Imprint"
                        onChange={(next) =>
                          setImprint(
                            "__create" in next ? next.__create : next.name,
                          )
                        }
                        value={imprint ? { name: imprint } : null}
                      />
                    </div>
                    {imprint && !isDisabled("imprint") && (
                      <Button
                        aria-label="Clear imprint"
                        className="shrink-0 cursor-pointer"
                        onClick={() => setImprint("")}
                        size="icon"
                        type="button"
                        variant="ghost"
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    )}
                  </div>
                </FieldRow>

                {/* Language */}
                <FieldRow
                  currentValue={
                    file?.language
                      ? getLanguageName(file.language)
                        ? `${getLanguageName(file.language)} (${file.language})`
                        : file.language
                      : undefined
                  }
                  decision={decisions.language}
                  disabled={isDisabled("language")}
                  hidden={!isRowVisible("language")}
                  label={formatMetadataFieldLabel("language")}
                  onDecisionChange={(v) => setDecision("language", v)}
                  status={fieldStatus.language}
                >
                  <LanguageCombobox
                    disabled={isDisabled("language")}
                    libraryId={book.library_id}
                    onChange={setLanguage}
                    value={language}
                  />
                </FieldRow>

                {/* Release date */}
                <FieldRow
                  currentValue={
                    file?.release_date
                      ? file.release_date.split("T")[0]
                      : undefined
                  }
                  decision={decisions.release_date}
                  disabled={isDisabled("release_date")}
                  hidden={!isRowVisible("release_date")}
                  label={formatMetadataFieldLabel("releaseDate")}
                  onDecisionChange={(v) => setDecision("release_date", v)}
                  status={fieldStatus.release_date}
                >
                  <Input
                    disabled={isDisabled("release_date")}
                    onChange={(e) => setReleaseDate(e.target.value)}
                    placeholder="YYYY-MM-DD"
                    value={releaseDate}
                  />
                </FieldRow>

                {/* URL */}
                <FieldRow
                  currentValue={file?.url || undefined}
                  decision={decisions.url}
                  disabled={isDisabled("url")}
                  hidden={!isRowVisible("url")}
                  label={formatMetadataFieldLabel("url")}
                  onDecisionChange={(v) => setDecision("url", v)}
                  status={fieldStatus.url}
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
                        <a
                          href={url.trim()}
                          rel="noopener noreferrer"
                          target="_blank"
                        >
                          <ExternalLink className="h-4 w-4" />
                        </a>
                      ) : (
                        <span>
                          <ExternalLink className="h-4 w-4" />
                        </span>
                      )}
                    </Button>
                  </div>
                </FieldRow>

                {/* Identifiers */}
                <FieldRow
                  currentValue={
                    currentIdentifiers.length > 0
                      ? currentIdentifiers
                          .map((id) => {
                            const label =
                              availableIdentifierTypes.find(
                                (t) => t.id === id.type,
                              )?.label ?? id.type;
                            return `${label}: ${id.value}`;
                          })
                          .join(", ")
                      : undefined
                  }
                  decision={decisions.identifiers}
                  disabled={isDisabled("identifiers")}
                  hidden={!isRowVisible("identifiers")}
                  label={formatMetadataFieldLabel("identifiers")}
                  onDecisionChange={(v) => setDecision("identifiers", v)}
                  status={fieldStatus.identifiers}
                >
                  <IdentifierEditor
                    hideHeader
                    identifierTypes={availableIdentifierTypes}
                    onChange={setIdentifiers}
                    value={identifiers}
                  />
                </FieldRow>

                {/* Abridged */}
                <FieldRow
                  currentValue={
                    file?.abridged != null
                      ? file.abridged
                        ? "Abridged"
                        : "Unabridged"
                      : undefined
                  }
                  decision={decisions.abridged}
                  disabled={isDisabled("abridged")}
                  hidden={!isRowVisible("abridged")}
                  label={formatMetadataFieldLabel("abridged")}
                  onDecisionChange={(v) => setDecision("abridged", v)}
                  status={fieldStatus.abridged}
                >
                  <div
                    className={cn(
                      "flex items-center gap-2",
                      !decisions.abridged && "pointer-events-none",
                    )}
                  >
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <div>
                          <Checkbox
                            aria-label="Mark as abridged"
                            checked={abridged === true}
                            disabled={
                              isDisabled("abridged") || !decisions.abridged
                            }
                            id="identify-abridged"
                            onCheckedChange={(checked) =>
                              setAbridged(checked === true ? true : null)
                            }
                          />
                        </div>
                      </TooltipTrigger>
                      {!decisions.abridged && (
                        <TooltipContent>
                          Apply this field first to edit
                        </TooltipContent>
                      )}
                    </Tooltip>
                    <Label
                      className="cursor-pointer text-sm font-normal text-muted-foreground"
                      htmlFor="identify-abridged"
                    >
                      This is an abridged edition
                    </Label>
                  </div>
                </FieldRow>
              </div>
            )}
          </>
        )}
      </div>

      <DialogFooter className="flex-row items-center justify-between gap-3 sm:justify-between">
        <Button
          className="text-xs"
          onClick={restoreSuggestions}
          size="sm"
          type="button"
          variant="ghost"
        >
          <RefreshCcw className="mr-1.5 h-3.5 w-3.5" />
          Restore suggestions
        </Button>
        <div className="flex items-center gap-3">
          <span className="hidden text-xs text-muted-foreground sm:block">
            <strong className="font-semibold text-foreground">
              {bookSelectedCount} book change
              {bookSelectedCount === 1 ? "" : "s"}
            </strong>{" "}
            ·{" "}
            <strong className="font-semibold text-foreground">
              {fileSelectedCount} file change
              {fileSelectedCount === 1 ? "" : "s"}
            </strong>{" "}
            selected
          </span>
          <Button
            disabled={applyMutation.isPending}
            onClick={onClose}
            size="sm"
            type="button"
            variant="outline"
          >
            Cancel
          </Button>
          <Button
            disabled={applyMutation.isPending || totalSelected === 0}
            onClick={handleSubmit}
            size="sm"
            type="button"
          >
            {applyMutation.isPending ? (
              <>
                <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                Applying…
              </>
            ) : totalSelected === 0 ? (
              "Apply changes"
            ) : (
              `Apply ${totalSelected} change${totalSelected === 1 ? "" : "s"}`
            )}
          </Button>
        </div>
      </DialogFooter>
    </>
  );
}
