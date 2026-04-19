import { Package, Plus, RefreshCw, Star, Trash2 } from "lucide-react";
import { useState } from "react";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  useAddRepository,
  usePluginRepositories,
  useRemoveRepository,
  useSyncRepository,
} from "@/hooks/queries/plugins";
import { useAuth } from "@/hooks/useAuth";

export const AdvancedRepositoriesSection = () => {
  const { hasPermission } = useAuth();
  const canWrite = hasPermission("config", "write");
  const { data: repos, isLoading, error } = usePluginRepositories();
  const addRepository = useAddRepository();
  const removeRepository = useRemoveRepository();
  const syncRepository = useSyncRepository();

  const [newUrl, setNewUrl] = useState("");
  const [newScope, setNewScope] = useState("");
  const [removeTarget, setRemoveTarget] = useState<string | null>(null);

  if (isLoading) return <LoadingSpinner />;
  if (error) {
    return (
      <p className="text-sm text-destructive">
        Failed to load repositories: {error.message}
      </p>
    );
  }

  const handleAdd = (e: React.FormEvent) => {
    e.preventDefault();
    if (!newUrl.trim() || !newScope.trim()) return;
    addRepository.mutate(
      { url: newUrl.trim(), scope: newScope.trim() },
      {
        onSuccess: () => {
          setNewUrl("");
          setNewScope("");
        },
      },
    );
  };

  return (
    <>
      <div className="space-y-3">
        {repos && repos.length > 0 ? (
          repos.map((repo) => (
            <div
              className="flex items-start justify-between gap-4 rounded-md border border-border p-4"
              key={repo.scope}
            >
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  {repo.is_official && (
                    <Star
                      aria-hidden="true"
                      className="h-4 w-4 shrink-0 text-yellow-500"
                    />
                  )}
                  <h3 className="text-sm font-medium">
                    {repo.name ?? repo.scope}
                  </h3>
                  <Badge variant="secondary">{repo.scope}</Badge>
                </div>
                <p className="mt-1 truncate text-xs text-muted-foreground">
                  {repo.url}
                </p>
                {repo.last_fetched_at && (
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    Last synced:{" "}
                    {new Date(repo.last_fetched_at).toLocaleString()}
                  </p>
                )}
                {repo.fetch_error && (
                  <p className="mt-1 text-xs text-destructive">
                    Sync error: {repo.fetch_error}
                  </p>
                )}
              </div>

              <div className="flex shrink-0 items-center gap-2">
                {canWrite && (
                  <>
                    <Button
                      disabled={syncRepository.isPending}
                      onClick={() =>
                        syncRepository.mutate({ scope: repo.scope })
                      }
                      size="sm"
                      variant="outline"
                    >
                      <RefreshCw aria-hidden="true" className="h-4 w-4" />
                    </Button>
                    {!repo.is_official && (
                      <Button
                        onClick={() => setRemoveTarget(repo.scope)}
                        size="sm"
                        variant="ghost"
                      >
                        <Trash2
                          aria-hidden="true"
                          className="h-4 w-4 text-destructive"
                        />
                      </Button>
                    )}
                  </>
                )}
              </div>
            </div>
          ))
        ) : (
          <div className="py-8 text-center">
            <Package
              aria-hidden="true"
              className="mx-auto mb-3 h-8 w-8 text-muted-foreground"
            />
            <p className="text-sm text-muted-foreground">
              No repositories configured.
            </p>
          </div>
        )}
      </div>

      {canWrite && (
        <form
          className="mt-6 flex items-end gap-3 rounded-md border border-border p-4"
          onSubmit={handleAdd}
        >
          <div className="flex-1 space-y-1">
            <Label className="text-xs" htmlFor="repo-url">
              Repository URL
            </Label>
            <Input
              id="repo-url"
              onChange={(e) => setNewUrl(e.target.value)}
              placeholder="https://example.com/plugins/index.json"
              value={newUrl}
            />
          </div>
          <div className="w-40 space-y-1">
            <Label className="text-xs" htmlFor="repo-scope">
              Scope
            </Label>
            <Input
              id="repo-scope"
              onChange={(e) => setNewScope(e.target.value)}
              placeholder="my-scope"
              value={newScope}
            />
          </div>
          <Button
            disabled={
              addRepository.isPending || !newUrl.trim() || !newScope.trim()
            }
            size="sm"
            type="submit"
          >
            <Plus aria-hidden="true" className="mr-1 h-4 w-4" />
            Add
          </Button>
        </form>
      )}

      <ConfirmDialog
        confirmLabel="Remove"
        description={`Are you sure you want to remove the "${removeTarget}" repository? Plugins from this repository will no longer receive updates.`}
        isPending={removeRepository.isPending}
        onConfirm={() => {
          if (removeTarget) {
            removeRepository.mutate(
              { scope: removeTarget },
              { onSuccess: () => setRemoveTarget(null) },
            );
          }
        }}
        onOpenChange={(open) => {
          if (!open) setRemoveTarget(null);
        }}
        open={!!removeTarget}
        title="Remove Repository"
      />
    </>
  );
};
