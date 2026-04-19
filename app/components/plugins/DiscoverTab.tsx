import { Check, Package } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { CapabilitiesWarning } from "@/components/plugins/CapabilitiesWarning";
import { filterPlugins } from "@/components/plugins/discoverFilters";
import { deriveCapabilityLabels } from "@/components/plugins/pluginCapabilities";
import { PluginRow } from "@/components/plugins/PluginRow";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  useInstallPlugin,
  usePluginsAvailable,
  usePluginsInstalled,
  type AvailablePlugin,
} from "@/hooks/queries/plugins";

interface DiscoverTabProps {
  canWrite: boolean;
}

export const DiscoverTab = ({ canWrite }: DiscoverTabProps) => {
  const { data: available = [], error, isLoading } = usePluginsAvailable();
  const { data: installed = [] } = usePluginsInstalled();
  const installPlugin = useInstallPlugin();

  const [capability, setCapability] = useState<string>("all");
  const [installTarget, setInstallTarget] = useState<AvailablePlugin | null>(
    null,
  );
  const [search, setSearch] = useState("");
  const [source, setSource] = useState<string>("all");

  const installedKeys = useMemo(
    () => new Set(installed.map((p) => `${p.scope}/${p.id}`)),
    [installed],
  );

  const sources = useMemo(() => {
    const set = new Set(available.map((p) => p.scope));
    return ["all", ...Array.from(set).sort()];
  }, [available]);

  const filtered = useMemo(
    () => filterPlugins(available, search, capability, source),
    [available, capability, search, source],
  );

  const handleInstallConfirm = () => {
    if (!installTarget) return;
    const compatibleVersion = installTarget.versions.find((v) => v.compatible);
    if (!compatibleVersion) return;
    installPlugin.mutate(
      {
        id: installTarget.id,
        name: installTarget.name,
        scope: installTarget.scope,
        version: compatibleVersion.version,
      },
      {
        onError: (err) => {
          toast.error(`Failed to install plugin: ${err.message}`);
        },
        onSuccess: () => setInstallTarget(null),
      },
    );
  };

  if (isLoading) return <LoadingSpinner />;
  if (error) {
    return (
      <p className="text-sm text-destructive">
        Failed to load available plugins: {error.message}
      </p>
    );
  }

  if (available.length === 0) {
    return (
      <div className="py-8 text-center">
        <Package
          aria-hidden="true"
          className="mx-auto mb-3 h-8 w-8 text-muted-foreground"
        />
        <p className="text-sm text-muted-foreground">
          No plugins available. Add a repository to discover plugins.
        </p>
      </div>
    );
  }

  return (
    <>
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <Input
            className="max-w-xs"
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search plugins…"
            value={search}
          />
          <Select onValueChange={setCapability} value={capability}>
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="Capability" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All capabilities</SelectItem>
              <SelectItem value="metadataEnricher">
                Metadata enricher
              </SelectItem>
              <SelectItem value="inputConverter">Input converter</SelectItem>
              <SelectItem value="fileParser">File parser</SelectItem>
              <SelectItem value="outputGenerator">Output generator</SelectItem>
            </SelectContent>
          </Select>
          <Select onValueChange={setSource} value={source}>
            <SelectTrigger className="w-[160px]">
              <SelectValue placeholder="Source" />
            </SelectTrigger>
            <SelectContent>
              {sources.map((s) => (
                <SelectItem key={s} value={s}>
                  {s === "all" ? "All sources" : s}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {filtered.length === 0 ? (
          <div className="py-8 text-center">
            <Package
              aria-hidden="true"
              className="mx-auto mb-3 h-8 w-8 text-muted-foreground"
            />
            <p className="text-sm text-muted-foreground">
              No plugins match your filters.
            </p>
          </div>
        ) : (
          <div className="space-y-2">
            {filtered.map((p) => {
              const key = `${p.scope}/${p.id}`;
              const isInstalled = installedKeys.has(key);
              const incompatible = p.compatible === false;
              const caps = deriveCapabilityLabels(p.versions[0]?.capabilities);

              return (
                <PluginRow
                  actions={
                    canWrite ? (
                      isInstalled ? (
                        <Button disabled size="sm" variant="outline">
                          <Check aria-hidden="true" className="mr-1 h-3 w-3" />
                          Installed
                        </Button>
                      ) : incompatible ? (
                        <Button disabled size="sm" variant="outline">
                          Incompatible
                        </Button>
                      ) : (
                        <Button
                          disabled={installPlugin.isPending}
                          onClick={() => setInstallTarget(p)}
                          size="sm"
                          variant="outline"
                        >
                          Install
                        </Button>
                      )
                    ) : null
                  }
                  author={p.author || undefined}
                  capabilities={caps}
                  description={p.description || undefined}
                  href={`/settings/plugins/${p.scope}/${p.id}`}
                  id={p.id}
                  imageUrl={p.imageUrl || null}
                  isOfficial={p.is_official}
                  key={key}
                  name={p.name}
                  scope={p.scope}
                  version={p.versions[0]?.version}
                />
              );
            })}
          </div>
        )}
      </div>

      <CapabilitiesWarning
        isPending={installPlugin.isPending}
        onConfirm={handleInstallConfirm}
        onOpenChange={(open) => {
          if (!open) setInstallTarget(null);
        }}
        open={!!installTarget}
        plugin={installTarget}
      />
    </>
  );
};
