import { Download, FolderSearch, Loader2, Package } from "lucide-react";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Button } from "@/components/ui/button";
import {
  PluginStatusActive,
  usePluginRepositories,
  usePluginsAvailable,
  usePluginsInstalled,
  useScanPlugins,
  useUpdatePluginVersion,
} from "@/hooks/queries/plugins";
import { useAuth } from "@/hooks/useAuth";
import type { Plugin } from "@/types/generated/models";

import {
  deriveCapabilityLabels,
  resolveInstalledPluginCapabilities,
} from "./pluginCapabilities";
import { PluginRow } from "./PluginRow";

export const InstalledTab = () => {
  const { hasPermission } = useAuth();
  const canWrite = hasPermission("config", "write");
  const { data: plugins, error, isLoading } = usePluginsInstalled();
  const { data: available = [] } = usePluginsAvailable();
  const { data: repos = [] } = usePluginRepositories();
  const updatePluginVersion = useUpdatePluginVersion();
  const scanPlugins = useScanPlugins();

  const repoNameByScope = new Map(
    repos.map((r) => [r.scope, r.name || r.scope]),
  );

  const handleScan = () => {
    scanPlugins.mutate(undefined, {
      onError: (err) => {
        toast.error(`Scan failed: ${err.message}`);
      },
      onSuccess: (discovered) => {
        if (discovered.length === 0) {
          toast.info("No new local plugins found.");
        } else {
          toast.success(
            `Discovered ${discovered.length} new local plugin${discovered.length > 1 ? "s" : ""}.`,
          );
        }
      },
    });
  };

  const renderRow = (plugin: Plugin) => {
    const availableEntry = available.find(
      (a) => a.scope === plugin.scope && a.id === plugin.id,
    );
    const caps = resolveInstalledPluginCapabilities(plugin, availableEntry);
    const capabilityLabels = deriveCapabilityLabels(caps);
    const imageUrl = availableEntry?.imageUrl || undefined;
    const isOfficial = availableEntry?.is_official ?? false;
    const isDisabled = plugin.status !== PluginStatusActive;
    const isThisUpdating =
      updatePluginVersion.isPending &&
      updatePluginVersion.variables?.scope === plugin.scope &&
      updatePluginVersion.variables?.id === plugin.id;

    return (
      <PluginRow
        actions={
          canWrite && plugin.update_available_version ? (
            <Button
              disabled={updatePluginVersion.isPending}
              onClick={() =>
                updatePluginVersion.mutate(
                  { id: plugin.id, scope: plugin.scope },
                  {
                    onError: (err) =>
                      toast.error(`Failed to update plugin: ${err.message}`),
                    onSuccess: (updated) =>
                      toast.success(
                        `Updated ${updated.name} to v${updated.version}`,
                      ),
                  },
                )
              }
              size="sm"
              variant="outline"
            >
              {isThisUpdating ? (
                <Loader2 aria-hidden="true" className="h-3 w-3 animate-spin" />
              ) : (
                <Download aria-hidden="true" className="mr-1 h-3 w-3" />
              )}
              Update
            </Button>
          ) : undefined
        }
        capabilities={capabilityLabels}
        description={plugin.description}
        disabled={isDisabled}
        href={`/settings/plugins/${plugin.scope}/${plugin.id}`}
        id={plugin.id}
        imageUrl={imageUrl}
        isOfficial={isOfficial}
        key={`${plugin.scope}/${plugin.id}`}
        name={plugin.name}
        repoName={repoNameByScope.get(plugin.scope)}
        scope={plugin.scope}
        updateAvailable={plugin.update_available_version ?? undefined}
        version={plugin.version}
      />
    );
  };

  if (isLoading) return <LoadingSpinner />;
  if (error) {
    return (
      <p className="text-sm text-destructive">
        Failed to load plugins: {error.message}
      </p>
    );
  }

  if (!plugins || plugins.length === 0) {
    return (
      <div className="space-y-4">
        <div className="py-8 text-center">
          <Package
            aria-hidden="true"
            className="mx-auto mb-3 h-8 w-8 text-muted-foreground"
          />
          <p className="text-sm text-muted-foreground">
            No plugins installed yet. Browse available plugins to get started.
          </p>
        </div>
        {canWrite && (
          <div className="flex justify-end">
            <Button
              disabled={scanPlugins.isPending}
              onClick={handleScan}
              size="sm"
              variant="outline"
            >
              {scanPlugins.isPending ? (
                <Loader2
                  aria-hidden="true"
                  className="mr-2 h-4 w-4 animate-spin"
                />
              ) : (
                <FolderSearch aria-hidden="true" className="mr-2 h-4 w-4" />
              )}
              Scan for Local Plugins
            </Button>
          </div>
        )}
      </div>
    );
  }

  // Sort: enabled-first alphabetical, then disabled alphabetical
  const enabled = plugins
    .filter((p) => p.status === PluginStatusActive)
    .sort((a, b) => a.name.localeCompare(b.name));
  const disabled = plugins
    .filter((p) => p.status !== PluginStatusActive)
    .sort((a, b) => a.name.localeCompare(b.name));

  return (
    <>
      {canWrite && (
        <div className="mb-4 flex justify-end">
          <Button
            disabled={scanPlugins.isPending}
            onClick={handleScan}
            size="sm"
            variant="outline"
          >
            {scanPlugins.isPending ? (
              <Loader2
                aria-hidden="true"
                className="mr-2 h-4 w-4 animate-spin"
              />
            ) : (
              <FolderSearch aria-hidden="true" className="mr-2 h-4 w-4" />
            )}
            Scan for Local Plugins
          </Button>
        </div>
      )}
      <div className="space-y-3">
        {enabled.map(renderRow)}

        {enabled.length > 0 && disabled.length > 0 && (
          <hr className="border-border" />
        )}

        {disabled.map(renderRow)}
      </div>
    </>
  );
};
