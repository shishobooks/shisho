import { useState } from "react";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import ReadOnlyNotice from "@/components/library/ReadOnlyNotice";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";
import {
  useCaches,
  useCacheSettings,
  useClearCache,
  useUpdateCacheSettings,
} from "@/hooks/queries/cache";
import { useCan } from "@/hooks/useCan";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useUnsavedChanges } from "@/hooks/useUnsavedChanges";
import { toastRequestError } from "@/libraries/api";
import type {
  Info as CacheInfo,
  Limit,
  SettingsResponse,
} from "@/types/generated/cache";
import { formatFileSize } from "@/utils/format";

// The caches with an editable limit, keyed by their id in GET /cache, with
// what happens when a cache goes over its limit.
const limitHelp: Record<keyof SettingsResponse, string> = {
  cover_thumbnails:
    "Least recently used thumbnails are removed above this limit. Set 0 to keep no thumbnails on disk.",
  downloads:
    "After each generated download, if the cache is over this limit, the least recently used files are removed until it is at 80% of it.",
  cbz_pages:
    "Least recently used pages are removed above this limit. Pages opened in the last minute are kept, so the cache can briefly go over it.",
  pdf_pages:
    "Least recently used pages are removed above this limit. Pages opened in the last minute are kept, so the cache can briefly go over it.",
};

const isLimitedCache = (id: string): id is keyof SettingsResponse =>
  id in limitHelp;

// An edited limit, kept as typed so an empty or partial number stays editable.
interface LimitDraft {
  size: string;
  unlimited: boolean;
}

const MAX_LIMIT_GB = 1024;

const parseSize = (size: string): number | null => {
  const value = Number(size);
  return size.trim() !== "" &&
    Number.isFinite(value) &&
    value >= 0 &&
    value <= MAX_LIMIT_GB
    ? value
    : null;
};

const isDirty = (draft: LimitDraft | undefined, saved: Limit) =>
  draft !== undefined &&
  (draft.unlimited !== saved.unlimited ||
    (!draft.unlimited && parseSize(draft.size) !== saved.max_size_gb));

interface CacheLimitProps {
  id: keyof SettingsResponse;
  name: string;
  canEdit: boolean;
  draft: LimitDraft | undefined;
  onDraftChange: (draft: LimitDraft | undefined) => void;
  saved: Limit;
}

const CacheLimit = ({
  id,
  name,
  canEdit,
  draft,
  onDraftChange,
  saved,
}: CacheLimitProps) => {
  const mutation = useUpdateCacheSettings();
  const current = draft ?? {
    size: String(saved.max_size_gb),
    unlimited: saved.unlimited,
  };
  const size = parseSize(current.size);
  const valid = current.unlimited || size !== null;
  const dirty = isDirty(draft, saved);
  const sizeId = `${id}-limit`;
  const sizeErrorId = `${id}-limit-error`;
  const unlimitedId = `${id}-unlimited`;
  // Shown only for a size typed into an editable field: an untouched or
  // lifted limit has nothing to fix.
  const sizeInvalid =
    draft !== undefined && !current.unlimited && size === null;
  const disabled = !canEdit || mutation.isPending;

  const save = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!valid || !dirty || !canEdit) return;
    try {
      await mutation.mutateAsync({
        // A lifted limit keeps the last valid size to restore later.
        [id]: {
          max_size_gb: size ?? saved.max_size_gb,
          unlimited: current.unlimited,
        },
      });
      onDraftChange(undefined);
      toast.success(`${name} cache limit saved`);
    } catch (err) {
      toastRequestError(err, "Failed to save cache limit");
    }
  };

  return (
    <form className="mt-4 space-y-2" onSubmit={save}>
      <Label htmlFor={sizeId}>Maximum size (GiB)</Label>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <Input
          aria-describedby={sizeInvalid ? sizeErrorId : undefined}
          aria-invalid={sizeInvalid ? true : undefined}
          // Every card has the same visible labels, so each control's name
          // also names its cache.
          aria-label={`Maximum size (GiB) for ${name}`}
          className="w-32"
          disabled={disabled || current.unlimited}
          id={sizeId}
          max={MAX_LIMIT_GB}
          min={0}
          onChange={(event) =>
            onDraftChange({ ...current, size: event.target.value })
          }
          required={!current.unlimited}
          step="any"
          type="number"
          value={current.size}
        />
        <div className="flex items-center gap-2">
          <Checkbox
            aria-label={`No limit for ${name}`}
            checked={current.unlimited}
            disabled={disabled}
            id={unlimitedId}
            onCheckedChange={(checked) =>
              onDraftChange({ ...current, unlimited: checked === true })
            }
          />
          <Label
            className="text-sm font-normal cursor-pointer"
            htmlFor={unlimitedId}
          >
            No limit
          </Label>
        </div>
        {canEdit && (
          <Button
            aria-label={
              mutation.isPending ? undefined : `Save limit for ${name}`
            }
            disabled={mutation.isPending || !valid || !dirty}
            type="submit"
          >
            {mutation.isPending ? "Saving..." : "Save limit"}
          </Button>
        )}
      </div>
      {sizeInvalid && (
        <p className="text-xs text-destructive" id={sizeErrorId}>
          Enter a size from 0 to {MAX_LIMIT_GB} GiB.
        </p>
      )}
      <p className="text-xs text-muted-foreground">
        {current.unlimited
          ? "The cache can use as much disk space as it needs. Clear it here to free space."
          : `${limitHelp[id]} Changes apply immediately.`}
      </p>
    </form>
  );
};

const AdminCache = () => {
  usePageTitle("Cache");
  const canClear = useCan("config:write");

  const cachesQuery = useCaches();
  const { data, isLoading } = cachesQuery;
  const clearMutation = useClearCache();

  const [pending, setPending] = useState<CacheInfo | null>(null);

  const settingsQuery = useCacheSettings();
  const settings = settingsQuery.data;
  const [drafts, setDrafts] = useState<
    Partial<Record<keyof SettingsResponse, LimitDraft>>
  >({});
  // One guard for the page: a router supports only one blocker at a time.
  const hasChanges =
    canClear &&
    settings !== undefined &&
    (Object.keys(drafts) as (keyof SettingsResponse)[]).some((id) =>
      isDirty(drafts[id], settings[id]),
    );
  const { showBlockerDialog, proceedNavigation, cancelNavigation } =
    useUnsavedChanges(hasChanges);
  const setDraft = (
    id: keyof SettingsResponse,
    draft: LimitDraft | undefined,
  ) =>
    setDrafts((prev) => {
      const next = { ...prev };
      if (draft) {
        next[id] = draft;
      } else {
        delete next[id];
      }
      return next;
    });

  if (isLoading) {
    return <LoadingSpinner />;
  }

  const pageHeader = (
    <div className="mb-6 md:mb-8">
      <h1 className="text-2xl font-semibold mb-1 md:mb-2">Cache</h1>
      <p className="text-sm md:text-base text-muted-foreground">
        Inspect and clear server caches. Content will be regenerated on next
        access.
      </p>
    </div>
  );

  if (!data) {
    return (
      <div>
        {pageHeader}
        {cachesQuery.error && (
          <QueryError fallback="Failed to load caches" query={cachesQuery} />
        )}
      </div>
    );
  }

  const handleConfirm = async () => {
    if (!pending) return;
    const target = pending;
    try {
      const result = await clearMutation.mutateAsync(target.id);
      toast.success(
        `Cleared ${formatFileSize(result.cleared_bytes)} (${result.cleared_files} files) from ${target.name}`,
      );
      setPending(null);
    } catch (err) {
      toastRequestError(err, "Failed to clear cache");
      setPending(null);
    }
  };

  return (
    <div>
      {pageHeader}
      {settingsQuery.error && !settings && (
        <div className="mb-6">
          <QueryError
            fallback="Failed to load cache settings"
            query={settingsQuery}
          />
        </div>
      )}

      {!canClear && (
        <div className="mb-6">
          <ReadOnlyNotice>
            You can view these caches but not change their limits or clear them.
          </ReadOnlyNotice>
        </div>
      )}

      <div className="grid gap-6">
        {data.map((cache) => {
          const isClearing =
            clearMutation.isPending && clearMutation.variables === cache.id;
          const limitId = isLimitedCache(cache.id) ? cache.id : null;
          return (
            <section
              aria-labelledby={`${cache.id}-heading`}
              className="border border-border rounded-md p-4 md:p-6"
              key={cache.id}
            >
              <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-3">
                <div>
                  <h2
                    className="text-base md:text-lg font-semibold"
                    id={`${cache.id}-heading`}
                  >
                    {cache.name}
                  </h2>
                  <p className="text-xs md:text-sm text-muted-foreground mt-1">
                    {cache.description}
                  </p>
                  <div className="mt-3 text-sm text-muted-foreground">
                    <span className="font-mono">
                      {formatFileSize(cache.size_bytes)}
                    </span>
                    <span className="mx-2 text-muted-foreground/50">·</span>
                    <span>
                      {cache.file_count}{" "}
                      {cache.file_count === 1 ? "file" : "files"}
                    </span>
                  </div>
                  {limitId &&
                    (settings ? (
                      <CacheLimit
                        canEdit={canClear}
                        draft={drafts[limitId]}
                        id={limitId}
                        name={cache.name}
                        onDraftChange={(draft) => setDraft(limitId, draft)}
                        saved={settings[limitId]}
                      />
                    ) : (
                      !settingsQuery.error && <LoadingSpinner />
                    ))}
                </div>
                {canClear && (
                  <Button
                    aria-label={`Clear ${cache.name} cache`}
                    disabled={isClearing || cache.file_count === 0}
                    onClick={() => setPending(cache)}
                    variant="outline"
                  >
                    {isClearing ? "Clearing..." : "Clear"}
                  </Button>
                )}
              </div>
            </section>
          );
        })}
      </div>

      <ConfirmDialog
        confirmLabel="Clear"
        description={
          pending
            ? `This will delete ${pending.file_count} files (${formatFileSize(pending.size_bytes)}). Content will be regenerated on next access.`
            : ""
        }
        isPending={clearMutation.isPending}
        onConfirm={handleConfirm}
        onOpenChange={(open) => {
          if (!open) setPending(null);
        }}
        open={pending !== null}
        title={pending ? `Clear ${pending.name}?` : ""}
        variant="destructive"
      />
      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />
    </div>
  );
};

export default AdminCache;
