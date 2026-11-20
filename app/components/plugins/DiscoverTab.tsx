import { BadgeCheck, Check, Download, Loader2, Puzzle } from "lucide-react";
import { useCallback, useId, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import EmptyState, { NoMatchesState } from "@/components/library/EmptyState";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import { CapabilitiesWarning } from "@/components/plugins/CapabilitiesWarning";
import { filterPlugins } from "@/components/plugins/discoverFilters";
import { EnrichExistingBooksPrompt } from "@/components/plugins/EnrichExistingBooksPrompt";
import {
  deriveCapabilityLabels,
  isMetadataEnricher,
} from "@/components/plugins/pluginCapabilities";
import { PluginRow } from "@/components/plugins/PluginRow";
import { detectionSummary } from "@/components/setup/detectionSummary";
import {
  pluginChoices,
  pluginKey,
  recommendPlugins,
} from "@/components/setup/pluginRecommendations";
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
  DETECT_MAX_PATHS,
  useDetectFileTypes,
} from "@/hooks/queries/filesystem";
import { useLibraries } from "@/hooks/queries/libraries";
import {
  useInstallPlugin,
  usePluginOrder,
  usePluginRepositories,
  usePluginsAvailable,
  usePluginsInstalled,
  useUpdatePluginVersion,
  type AvailablePlugin,
} from "@/hooks/queries/plugins";
import { useCan } from "@/hooks/useCan";
import { toastRequestError } from "@/libraries/api";
import { PluginHookMetadataEnricher, PluginStatusActive } from "@/types";

interface DiscoverTabProps {
  canWrite: boolean;
}

export const DiscoverTab = ({ canWrite }: DiscoverTabProps) => {
  const availableQuery = usePluginsAvailable();
  const { data: available = [], isLoading } = availableQuery;
  const installedQuery = usePluginsInstalled();
  const { data: installed = [] } = installedQuery;
  const { data: repos = [] } = usePluginRepositories();
  const installPlugin = useInstallPlugin();
  const updatePluginVersion = useUpdatePluginVersion();

  const [capability, setCapability] = useState<string>("all");
  const [installTarget, setInstallTarget] = useState<AvailablePlugin | null>(
    null,
  );
  // The metadata enricher just installed, to offer enriching existing Books
  // to a role that can start the scan (the resync button's permissions).
  const canScan = useCan(["jobs:read", "jobs:write"]);
  const [installedEnricher, setInstalledEnricher] = useState<string | null>(
    null,
  );
  const closeEnrichPrompt = useCallback(() => setInstalledEnricher(null), []);
  const [search, setSearch] = useState("");
  const [source, setSource] = useState<string>("all");

  const installedByKey = useMemo(
    () => new Map(installed.map((p) => [pluginKey(p), p])),
    [installed],
  );

  const repoNameByScope = useMemo(
    () => new Map(repos.map((r) => [r.scope, r.name || r.scope])),
    [repos],
  );

  const sources = useMemo(() => {
    const scopesWithPlugins = new Set(available.map((p) => p.scope));
    const scopeMeta = new Map(
      repos.map((r) => [
        r.scope,
        { isOfficial: r.is_official, name: r.name || r.scope },
      ]),
    );
    const present = Array.from(scopesWithPlugins).map((scope) => {
      const meta = scopeMeta.get(scope);
      return {
        isOfficial: meta?.isOfficial ?? false,
        name: meta?.name ?? scope,
        scope,
      };
    });
    present.sort((a, b) => {
      if (a.isOfficial !== b.isOfficial) return a.isOfficial ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    return present;
  }, [available, repos]);

  const filtered = useMemo(
    () => filterPlugins(available, search, capability, source),
    [available, capability, search, source],
  );

  // Setup's plugin recommendations, for as long as Getting Started's "Add
  // metadata plugins" step is open (until a metadata enricher is installed),
  // since the step leads here. They're worked out from what the libraries'
  // folders hold, as Setup works them out from the folders it's given.
  // Hook order lists each installed enricher, the same rule the step uses.
  const enricherOrder = usePluginOrder(PluginHookMetadataEnricher);
  const noEnricher =
    installedQuery.isSuccess &&
    enricherOrder.isSuccess &&
    enricherOrder.data.length === 0;
  const filtering = search !== "" || capability !== "all" || source !== "all";
  const offerRecommendations = canWrite && !filtering && noEnricher;
  const librariesQuery = useLibraries({ enabled: offerRecommendations });
  const { libraryPaths, truncated } = useMemo(() => {
    const paths = [
      ...new Set(
        (librariesQuery.data?.items ?? []).flatMap((library) =>
          (library.library_paths ?? []).map((path) => path.filepath),
        ),
      ),
    ];
    return {
      libraryPaths: paths.slice(0, DETECT_MAX_PATHS),
      truncated: paths.length > DETECT_MAX_PATHS,
    };
  }, [librariesQuery.data]);
  // A role that can't list libraries gets the main plugins, as Setup gives
  // when it can't look through the folders. One that can, with none, has
  // nothing to recommend for: Setup covers a server's first library.
  const listsLibraries = librariesQuery.isEnabled;
  const hasLibrary = !listsLibraries || libraryPaths.length > 0;
  const detection = useDetectFileTypes(
    offerRecommendations && libraryPaths.length > 0 ? libraryPaths : [],
  );
  const detecting =
    (listsLibraries && librariesQuery.isPending) ||
    (detection.isEnabled && detection.isPending);
  // With more folders than one detection takes, it counted only some, which
  // the summary says as it does for a walk that stopped early.
  const detected = useMemo(
    () =>
      detection.data && truncated
        ? { ...detection.data, stopped_early: true }
        : detection.data,
    [detection.data, truncated],
  );
  const recommended = useMemo(() => {
    if (!offerRecommendations || !hasLibrary || detecting) return [];
    return pluginChoices(
      available,
      recommendPlugins(detected),
    ).recommended.filter((p) => !installedByKey.has(pluginKey(p)));
  }, [
    available,
    detected,
    detecting,
    hasLibrary,
    installedByKey,
    offerRecommendations,
  ]);
  const showRecommendations =
    offerRecommendations && hasLibrary && (detecting || recommended.length > 0);
  const rest = useMemo(() => {
    if (!showRecommendations) return filtered;
    const keys = new Set(recommended.map(pluginKey));
    return filtered.filter((p) => !keys.has(pluginKey(p)));
  }, [filtered, recommended, showRecommendations]);
  const recommendedId = useId();
  const restId = useId();

  const handleInstallConfirm = () => {
    if (!installTarget) return;
    const compatibleVersion = installTarget.versions.find((v) => v.compatible);
    if (!compatibleVersion) return;
    const { name } = installTarget;
    installPlugin.mutate(
      {
        id: installTarget.id,
        name,
        scope: installTarget.scope,
        version: compatibleVersion.version,
      },
      {
        onError: (err) => {
          toastRequestError(err, "Failed to install plugin");
        },
        onSuccess: (installed) => {
          setInstallTarget(null);
          if (
            installed.status === PluginStatusActive &&
            isMetadataEnricher(compatibleVersion.capabilities)
          ) {
            setInstalledEnricher(name);
          }
        },
      },
    );
  };

  const renderRow = (p: AvailablePlugin) => {
    const key = pluginKey(p);
    const installedPlugin = installedByKey.get(key);
    const isInstalled = !!installedPlugin;
    const updateAvailable =
      installedPlugin?.update_available_version ?? undefined;
    const incompatible = p.compatible === false;
    const caps = deriveCapabilityLabels(p.versions[0]?.capabilities);
    const isThisInstalling =
      installPlugin.isPending &&
      installTarget?.scope === p.scope &&
      installTarget?.id === p.id;
    const isThisUpdating =
      updatePluginVersion.isPending &&
      updatePluginVersion.variables?.scope === p.scope &&
      updatePluginVersion.variables?.id === p.id;

    return (
      <PluginRow
        actions={
          canWrite ? (
            updateAvailable ? (
              <Button
                disabled={updatePluginVersion.isPending}
                onClick={() =>
                  updatePluginVersion.mutate(
                    { id: p.id, scope: p.scope },
                    {
                      onError: (err) =>
                        toastRequestError(err, "Failed to update plugin"),
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
                  <Loader2
                    aria-hidden="true"
                    className="h-3 w-3 animate-spin"
                  />
                ) : (
                  <Download aria-hidden="true" className="mr-1 h-3 w-3" />
                )}
                Update
              </Button>
            ) : isInstalled ? (
              <Button
                className="border-transparent bg-green-100 text-green-700 hover:bg-green-100 dark:bg-green-900/40 dark:text-green-400 dark:hover:bg-green-900/40"
                disabled
                size="sm"
                variant="outline"
              >
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
              >
                {isThisInstalling ? "Installing…" : "Install"}
              </Button>
            )
          ) : null
        }
        capabilities={caps}
        description={p.description || undefined}
        href={`/admin/plugins/${p.scope}/${p.id}`}
        id={p.id}
        imageUrl={p.imageUrl || null}
        isOfficial={p.is_official}
        key={key}
        name={p.name}
        repoName={repoNameByScope.get(p.scope)}
        scope={p.scope}
        updateAvailable={updateAvailable}
        version={installedPlugin?.version ?? p.versions[0]?.version}
      />
    );
  };

  if (isLoading) return <LoadingSpinner />;
  if (availableQuery.error && !availableQuery.data) {
    return (
      <QueryError
        fallback="Failed to load available plugins"
        query={availableQuery}
      />
    );
  }

  if (available.length === 0) {
    return (
      <EmptyState
        action={
          canWrite && (
            <Button asChild size="sm" variant="outline">
              <Link to={{ search: "?advanced=repositories" }}>
                Manage Repositories
              </Link>
            </Button>
          )
        }
        description="Plugins come from repositories. The plugins each repository offers show up here; add a repository to see more."
        icon={Puzzle}
        title="No plugins available"
      />
    );
  }

  return (
    <>
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <Input
            aria-label="Search plugins"
            className="max-w-xs"
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search plugins…"
            value={search}
          />
          <Select onValueChange={setCapability} value={capability}>
            <SelectTrigger aria-label="Capability" className="w-[180px]">
              <SelectValue placeholder="Capability" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All capabilities</SelectItem>
              <SelectItem value="metadataEnricher">
                Metadata enricher
              </SelectItem>
              <SelectItem value="inputConverter">Input converter</SelectItem>
              <SelectItem value="fileParser">File parser</SelectItem>
            </SelectContent>
          </Select>
          <Select onValueChange={setSource} value={source}>
            <SelectTrigger aria-label="Source" className="w-[220px]">
              <SelectValue placeholder="Source" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All sources</SelectItem>
              {sources.map((s) => (
                <SelectItem key={s.scope} value={s.scope}>
                  <span className="inline-flex items-center gap-1.5">
                    {s.isOfficial && (
                      <BadgeCheck
                        aria-label="Official repository"
                        className="h-3.5 w-3.5 text-primary"
                      />
                    )}
                    {s.name}
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {showRecommendations && (
          <section
            aria-labelledby={recommendedId}
            className="space-y-2"
            role="group"
          >
            <h2 className="text-sm font-medium" id={recommendedId}>
              Recommended for your library
            </h2>
            {detecting ? (
              <div className="flex flex-col items-center gap-3 py-6 text-center">
                <LoadingSpinner />
                <p className="text-sm text-muted-foreground">
                  Looking through your libraries…
                </p>
              </div>
            ) : (
              <>
                {detection.isEnabled && (
                  <p className="text-sm text-muted-foreground">
                    {detectionSummary(detected, "libraries")}
                  </p>
                )}
                {recommended.map(renderRow)}
              </>
            )}
          </section>
        )}

        {filtered.length === 0 ? (
          <NoMatchesState itemLabel="plugins" />
        ) : showRecommendations ? (
          rest.length > 0 && (
            <section
              aria-labelledby={restId}
              className="space-y-2"
              role="group"
            >
              <h2 className="text-sm font-medium" id={restId}>
                All plugins
              </h2>
              {rest.map(renderRow)}
            </section>
          )
        ) : (
          <div className="space-y-2">{rest.map(renderRow)}</div>
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
      {canScan && (
        <EnrichExistingBooksPrompt
          onClose={closeEnrichPrompt}
          pluginName={installedEnricher}
        />
      )}
    </>
  );
};
