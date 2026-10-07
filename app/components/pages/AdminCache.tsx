import { useState } from "react";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import { Button } from "@/components/ui/button";
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
import type { Info as CacheInfo } from "@/types/generated/cache";
import { formatFileSize } from "@/utils/format";

const CoverThumbnailLimit = ({ canEdit }: { canEdit: boolean }) => {
  const settingsQuery = useCacheSettings();
  const mutation = useUpdateCacheSettings();
  const [draft, setDraft] = useState<string | null>(null);
  const current = settingsQuery.data?.cover_thumbnail_max_size_gb;
  const value = draft ?? String(current ?? "");
  const size = Number(value);
  const hasChanges =
    canEdit && draft !== null && (value.trim() === "" || size !== current);
  const { showBlockerDialog, proceedNavigation, cancelNavigation } =
    useUnsavedChanges(hasChanges);
  if (!settingsQuery.data) {
    return settingsQuery.error ? (
      <QueryError
        fallback="Failed to load cache settings"
        query={settingsQuery}
      />
    ) : (
      <LoadingSpinner />
    );
  }
  const valid =
    value.trim() !== "" && Number.isFinite(size) && size >= 0 && size <= 1024;
  const save = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!valid || !canEdit) return;
    try {
      await mutation.mutateAsync({ cover_thumbnail_max_size_gb: size });
      setDraft(null);
      toast.success("Cover thumbnail cache limit saved");
    } catch (err) {
      toastRequestError(err, "Failed to save cache limit");
    }
  };
  return (
    <>
      <form className="mt-4 space-y-2" onSubmit={save}>
        <Label htmlFor="cover-thumbnail-limit">Maximum size (GiB)</Label>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            className="w-32"
            disabled={!canEdit || mutation.isPending}
            id="cover-thumbnail-limit"
            max={1024}
            min={0}
            onChange={(event) => setDraft(event.target.value)}
            required
            step="any"
            type="number"
            value={value}
          />
          {canEdit && (
            <Button
              disabled={mutation.isPending || !valid || size === current}
              type="submit"
            >
              {mutation.isPending ? "Saving..." : "Save limit"}
            </Button>
          )}
        </div>
        <p className="text-xs text-muted-foreground">
          Least recently used thumbnails are removed above this limit. Changes
          apply immediately. Set 0 to keep no thumbnails on disk.
        </p>
      </form>
      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />
    </>
  );
};

const AdminCache = () => {
  usePageTitle("Cache");
  const canClear = useCan("config:write");

  const cachesQuery = useCaches();
  const { data, isLoading } = cachesQuery;
  const clearMutation = useClearCache();

  const [pending, setPending] = useState<CacheInfo | null>(null);

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

      <div className="grid gap-6">
        {data.map((cache) => {
          const isClearing =
            clearMutation.isPending && clearMutation.variables === cache.id;
          return (
            <div
              className="border border-border rounded-md p-4 md:p-6"
              key={cache.id}
            >
              <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-3">
                <div>
                  <h2 className="text-base md:text-lg font-semibold">
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
                  {cache.id === "cover_thumbnails" && (
                    <CoverThumbnailLimit canEdit={canClear} />
                  )}
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
            </div>
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
    </div>
  );
};

export default AdminCache;
