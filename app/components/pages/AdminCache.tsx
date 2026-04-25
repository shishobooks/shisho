import { useState } from "react";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { useCaches, useClearCache } from "@/hooks/queries/cache";
import { useAuth } from "@/hooks/useAuth";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { Info as CacheInfo } from "@/types/generated/cache";

const formatBytes = (bytes: number): string => {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(
    sizes.length - 1,
    Math.floor(Math.log(bytes) / Math.log(k)),
  );
  const value = bytes / Math.pow(k, i);
  return `${value >= 10 ? value.toFixed(0) : value.toFixed(1)} ${sizes[i]}`;
};

const AdminCache = () => {
  usePageTitle("Cache");
  const { hasPermission } = useAuth();
  const canClear = hasPermission("config", "write");

  const { data, isLoading, error } = useCaches();
  const clearMutation = useClearCache();

  const [pending, setPending] = useState<CacheInfo | null>(null);

  if (isLoading) {
    return <LoadingSpinner />;
  }

  if (error) {
    return (
      <div className="text-center">
        <h1 className="text-2xl font-semibold mb-4">Error loading caches</h1>
        <p className="text-muted-foreground">{error.message}</p>
      </div>
    );
  }

  if (!data) return null;

  const handleConfirm = async () => {
    if (!pending) return;
    const target = pending;
    try {
      const result = await clearMutation.mutateAsync(target.id);
      toast.success(
        `Cleared ${formatBytes(result.cleared_bytes)} (${result.cleared_files} files) from ${target.name}`,
      );
      setPending(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to clear cache");
      setPending(null);
    }
  };

  return (
    <div>
      <div className="mb-6 md:mb-8">
        <h1 className="text-xl md:text-2xl font-semibold mb-1 md:mb-2">
          Cache
        </h1>
        <p className="text-sm md:text-base text-muted-foreground">
          Inspect and clear server caches. Content will be regenerated on next
          access.
        </p>
      </div>

      <div className="grid gap-6">
        {data.caches.map((cache) => {
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
                      {formatBytes(cache.size_bytes)}
                    </span>
                    <span className="mx-2 text-muted-foreground/50">·</span>
                    <span>
                      {cache.file_count}{" "}
                      {cache.file_count === 1 ? "file" : "files"}
                    </span>
                  </div>
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
            ? `This will delete ${pending.file_count} files (${formatBytes(pending.size_bytes)}). Content will be regenerated on next access.`
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
