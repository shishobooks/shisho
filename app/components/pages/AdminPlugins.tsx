import {
  ArrowDown,
  ArrowUp,
  ChevronDown,
  Download,
  ExternalLink,
  FolderSearch,
  Loader2,
  Package,
  Plus,
  RefreshCw,
  Settings,
  Star,
  Trash2,
} from "lucide-react";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { CapabilitiesWarning } from "@/components/plugins/CapabilitiesWarning";
import { PluginConfigDialog } from "@/components/plugins/PluginConfigDialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  useAddRepository,
  useInstallPlugin,
  usePluginOrder,
  usePluginRepositories,
  usePluginsAvailable,
  usePluginsInstalled,
  useReloadPlugin,
  useRemoveRepository,
  useScanPlugins,
  useSetPluginOrder,
  useSyncRepository,
  useUninstallPlugin,
  useUpdatePlugin,
  useUpdatePluginVersion,
  type AvailablePlugin,
  type Plugin,
  type PluginHookType,
  type PluginOrder,
} from "@/hooks/queries/plugins";
import { useAuth } from "@/hooks/useAuth";
import { usePageTitle } from "@/hooks/usePageTitle";

// --- Installed Tab ---

const InstalledTab = () => {
  const { hasPermission } = useAuth();
  const canWrite = hasPermission("config", "write");
  const { data: plugins, isLoading, error } = usePluginsInstalled();
  const updatePlugin = useUpdatePlugin();
  const updatePluginVersion = useUpdatePluginVersion();
  const reloadPlugin = useReloadPlugin();
  const uninstallPlugin = useUninstallPlugin();
  const scanPlugins = useScanPlugins();

  const [configTarget, setConfigTarget] = useState<Plugin | null>(null);
  const [uninstallTarget, setUninstallTarget] = useState<Plugin | null>(null);

  const handleScan = () => {
    scanPlugins.mutate(undefined, {
      onSuccess: (discovered) => {
        if (discovered.length === 0) {
          toast.info("No new local plugins found.");
        } else {
          toast.success(
            `Discovered ${discovered.length} new local plugin${discovered.length > 1 ? "s" : ""}.`,
          );
        }
      },
      onError: (err) => {
        toast.error(`Scan failed: ${err.message}`);
      },
    });
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
          <Package className="mx-auto mb-3 h-8 w-8 text-muted-foreground" />
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
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <FolderSearch className="mr-2 h-4 w-4" />
              )}
              Scan for Local Plugins
            </Button>
          </div>
        )}
      </div>
    );
  }

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
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <FolderSearch className="mr-2 h-4 w-4" />
            )}
            Scan for Local Plugins
          </Button>
        </div>
      )}
      <div className="space-y-3">
        {plugins.map((plugin) => (
          <div
            className="flex items-start justify-between gap-4 rounded-md border border-border p-4"
            key={`${plugin.scope}/${plugin.id}`}
          >
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <h3 className="text-sm font-medium">{plugin.name}</h3>
                <Badge variant="outline">{plugin.version}</Badge>
                <Badge variant="secondary">{plugin.scope}</Badge>
                {plugin.update_available_version && (
                  <>
                    <Badge variant="default">
                      Update: {plugin.update_available_version}
                    </Badge>
                    {canWrite && (
                      <Button
                        disabled={updatePluginVersion.isPending}
                        onClick={() =>
                          updatePluginVersion.mutate({
                            scope: plugin.scope,
                            id: plugin.id,
                          })
                        }
                        size="sm"
                        variant="outline"
                      >
                        {updatePluginVersion.isPending ? (
                          <Loader2 className="h-3 w-3 animate-spin" />
                        ) : (
                          <Download className="mr-1 h-3 w-3" />
                        )}
                        Update
                      </Button>
                    )}
                  </>
                )}
              </div>
              {plugin.description && (
                <p className="mt-1 text-xs text-muted-foreground">
                  {plugin.description}
                </p>
              )}
              {plugin.author && (
                <p className="mt-0.5 text-xs text-muted-foreground">
                  by {plugin.author}
                </p>
              )}
              {plugin.load_error && (
                <p className="mt-1 text-xs text-destructive">
                  Error: {plugin.load_error}
                </p>
              )}
            </div>

            <div className="flex shrink-0 items-center gap-3">
              {canWrite && (
                <>
                  <div className="flex items-center gap-2">
                    <Label
                      className="text-xs text-muted-foreground"
                      htmlFor={`enable-${plugin.scope}-${plugin.id}`}
                    >
                      {plugin.enabled ? "Enabled" : "Disabled"}
                    </Label>
                    <Switch
                      checked={plugin.enabled}
                      id={`enable-${plugin.scope}-${plugin.id}`}
                      onCheckedChange={(checked) => {
                        updatePlugin.mutate({
                          scope: plugin.scope,
                          id: plugin.id,
                          payload: { enabled: checked },
                        });
                      }}
                    />
                  </div>
                  {plugin.enabled && (
                    <>
                      <Button
                        disabled={reloadPlugin.isPending}
                        onClick={() =>
                          reloadPlugin.mutate(
                            { scope: plugin.scope, id: plugin.id },
                            {
                              onSuccess: () =>
                                toast.success(`Reloaded ${plugin.name}`),
                              onError: (err) =>
                                toast.error(`Failed to reload: ${err.message}`),
                            },
                          )
                        }
                        size="sm"
                        title="Reload plugin from disk"
                        variant="ghost"
                      >
                        <RefreshCw className="h-4 w-4" />
                      </Button>
                      <Button
                        onClick={() => setConfigTarget(plugin)}
                        size="sm"
                        variant="ghost"
                      >
                        <Settings className="h-4 w-4" />
                      </Button>
                    </>
                  )}
                  <Button
                    onClick={() => setUninstallTarget(plugin)}
                    size="sm"
                    variant="ghost"
                  >
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                </>
              )}
              {plugin.homepage && (
                <a
                  className="text-muted-foreground hover:text-foreground"
                  href={plugin.homepage}
                  rel="noopener noreferrer"
                  target="_blank"
                >
                  <ExternalLink className="h-4 w-4" />
                </a>
              )}
            </div>
          </div>
        ))}
      </div>

      <ConfirmDialog
        confirmLabel="Uninstall"
        description={`Are you sure you want to uninstall "${uninstallTarget?.name}"? This action cannot be undone.`}
        isPending={uninstallPlugin.isPending}
        onConfirm={() => {
          if (uninstallTarget) {
            uninstallPlugin.mutate(
              { scope: uninstallTarget.scope, id: uninstallTarget.id },
              { onSuccess: () => setUninstallTarget(null) },
            );
          }
        }}
        onOpenChange={(open) => {
          if (!open) setUninstallTarget(null);
        }}
        open={!!uninstallTarget}
        title="Uninstall Plugin"
      />

      <PluginConfigDialog
        onOpenChange={(open) => {
          if (!open) setConfigTarget(null);
        }}
        open={!!configTarget}
        pluginId={configTarget?.id ?? ""}
        pluginName={configTarget?.name ?? ""}
        scope={configTarget?.scope ?? ""}
      />
    </>
  );
};

// --- Browse Tab ---

const BrowseTab = () => {
  const { hasPermission } = useAuth();
  const canWrite = hasPermission("config", "write");
  const { data: available, isLoading, error } = usePluginsAvailable();
  const { data: installed } = usePluginsInstalled();
  const installPlugin = useInstallPlugin();

  const [installTarget, setInstallTarget] = useState<AvailablePlugin | null>(
    null,
  );

  if (isLoading) return <LoadingSpinner />;
  if (error) {
    return (
      <p className="text-sm text-destructive">
        Failed to load available plugins: {error.message}
      </p>
    );
  }

  if (!available || available.length === 0) {
    return (
      <div className="py-8 text-center">
        <Package className="mx-auto mb-3 h-8 w-8 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">
          No plugins available. Add a repository to browse plugins.
        </p>
      </div>
    );
  }

  const isInstalled = (scope: string, id: string) =>
    installed?.some((p) => p.scope === scope && p.id === id) ?? false;

  const handleInstall = () => {
    if (!installTarget) return;
    const latestVersion = installTarget.versions[0];
    installPlugin.mutate(
      {
        scope: installTarget.scope,
        id: installTarget.id,
        name: installTarget.name,
        version: latestVersion?.version,
        download_url: latestVersion?.download_url,
        sha256: latestVersion?.sha256,
      },
      { onSuccess: () => setInstallTarget(null) },
    );
  };

  return (
    <>
      <div className="space-y-3">
        {available.map((plugin) => {
          const alreadyInstalled = isInstalled(plugin.scope, plugin.id);
          const latestVersion = plugin.versions[0];

          return (
            <div
              className="flex items-start justify-between gap-4 rounded-md border border-border p-4"
              key={`${plugin.scope}/${plugin.id}`}
            >
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <h3 className="text-sm font-medium">{plugin.name}</h3>
                  {latestVersion && (
                    <Badge variant="outline">{latestVersion.version}</Badge>
                  )}
                  <Badge variant="secondary">{plugin.scope}</Badge>
                  {alreadyInstalled && (
                    <Badge variant="subtle">Installed</Badge>
                  )}
                </div>
                {plugin.description && (
                  <p className="mt-1 text-xs text-muted-foreground">
                    {plugin.description}
                  </p>
                )}
                {plugin.author && (
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    by {plugin.author}
                  </p>
                )}
              </div>

              <div className="flex shrink-0 items-center gap-2">
                {canWrite && !alreadyInstalled && (
                  <Button
                    onClick={() => setInstallTarget(plugin)}
                    size="sm"
                    variant="outline"
                  >
                    Install
                  </Button>
                )}
                {plugin.homepage && (
                  <a
                    className="text-muted-foreground hover:text-foreground"
                    href={plugin.homepage}
                    rel="noopener noreferrer"
                    target="_blank"
                  >
                    <ExternalLink className="h-4 w-4" />
                  </a>
                )}
              </div>
            </div>
          );
        })}
      </div>

      <CapabilitiesWarning
        isPending={installPlugin.isPending}
        onConfirm={handleInstall}
        onOpenChange={(open) => {
          if (!open) setInstallTarget(null);
        }}
        open={!!installTarget}
        plugin={installTarget}
      />
    </>
  );
};

// --- Order Tab ---

const HOOK_TYPES: { label: string; value: PluginHookType }[] = [
  { label: "Input Converter", value: "inputConverter" },
  { label: "File Parser", value: "fileParser" },
  { label: "Output Generator", value: "outputGenerator" },
  { label: "Metadata Enricher", value: "metadataEnricher" },
];

const OrderTab = () => {
  const { hasPermission } = useAuth();
  const canWrite = hasPermission("config", "write");
  const [selectedHookType, setSelectedHookType] =
    useState<PluginHookType>("fileParser");
  const { data: order, isLoading, error } = usePluginOrder(selectedHookType);
  const setPluginOrder = useSetPluginOrder();
  const { data: plugins } = usePluginsInstalled();

  const [localOrder, setLocalOrder] = useState<PluginOrder[] | null>(null);

  // Sync local order with fetched data
  const displayOrder = localOrder ?? order ?? [];

  const hasOrderChanged =
    localOrder !== null &&
    (localOrder.length !== (order?.length ?? 0) ||
      localOrder.some(
        (item, i) =>
          item.scope !== order?.[i]?.scope ||
          item.plugin_id !== order?.[i]?.plugin_id,
      ));

  const handleMove = (index: number, direction: "up" | "down") => {
    const newOrder = [...displayOrder];
    const targetIndex = direction === "up" ? index - 1 : index + 1;
    if (targetIndex < 0 || targetIndex >= newOrder.length) return;
    [newOrder[index], newOrder[targetIndex]] = [
      newOrder[targetIndex],
      newOrder[index],
    ];
    setLocalOrder(newOrder);
  };

  const handleSave = () => {
    setPluginOrder.mutate(
      {
        hookType: selectedHookType,
        order: displayOrder.map((o) => ({ scope: o.scope, id: o.plugin_id })),
      },
      {
        onSuccess: () => {
          setLocalOrder(null);
          toast.success("Plugin order saved.");
        },
        onError: (err) => {
          toast.error(`Failed to save order: ${err.message}`);
        },
      },
    );
  };

  const getPluginName = (scope: string, pluginId: string) => {
    const plugin = plugins?.find((p) => p.scope === scope && p.id === pluginId);
    return plugin?.name ?? `${scope}/${pluginId}`;
  };

  if (isLoading) return <LoadingSpinner />;
  if (error) {
    return (
      <p className="text-sm text-destructive">
        Failed to load plugin order: {error.message}
      </p>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <Label className="text-sm" htmlFor="hook-type-select">
          Hook Type
        </Label>
        <Select
          onValueChange={(value) => {
            setSelectedHookType(value as PluginHookType);
            setLocalOrder(null);
          }}
          value={selectedHookType}
        >
          <SelectTrigger className="w-[200px]" id="hook-type-select">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {HOOK_TYPES.map((ht) => (
              <SelectItem key={ht.value} value={ht.value}>
                {ht.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {displayOrder.length === 0 ? (
        <div className="py-8 text-center">
          <ChevronDown className="mx-auto mb-3 h-8 w-8 text-muted-foreground" />
          <p className="text-sm text-muted-foreground">
            No plugins registered for this hook type.
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {displayOrder.map((item, index) => (
            <div
              className="flex items-center justify-between gap-3 rounded-md border border-border p-3"
              key={`${item.scope}/${item.plugin_id}`}
            >
              <div className="flex items-center gap-3">
                <span className="text-xs font-mono text-muted-foreground">
                  {index + 1}
                </span>
                <span className="text-sm">
                  {getPluginName(item.scope, item.plugin_id)}
                </span>
                <Badge variant="secondary">{item.scope}</Badge>
              </div>
              {canWrite && (
                <div className="flex items-center gap-1">
                  <Button
                    disabled={index === 0}
                    onClick={() => handleMove(index, "up")}
                    size="sm"
                    variant="ghost"
                  >
                    <ArrowUp className="h-4 w-4" />
                  </Button>
                  <Button
                    disabled={index === displayOrder.length - 1}
                    onClick={() => handleMove(index, "down")}
                    size="sm"
                    variant="ghost"
                  >
                    <ArrowDown className="h-4 w-4" />
                  </Button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {canWrite && displayOrder.length > 0 && (
        <Button
          disabled={!hasOrderChanged || setPluginOrder.isPending}
          onClick={handleSave}
          size="sm"
        >
          {setPluginOrder.isPending ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Saving...
            </>
          ) : (
            "Save Order"
          )}
        </Button>
      )}
    </div>
  );
};

// --- Repositories Tab ---

const RepositoriesTab = () => {
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
                    <Star className="h-4 w-4 shrink-0 text-yellow-500" />
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
                      <RefreshCw className="h-4 w-4" />
                    </Button>
                    {!repo.is_official && (
                      <Button
                        onClick={() => setRemoveTarget(repo.scope)}
                        size="sm"
                        variant="ghost"
                      >
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    )}
                  </>
                )}
              </div>
            </div>
          ))
        ) : (
          <div className="py-8 text-center">
            <Package className="mx-auto mb-3 h-8 w-8 text-muted-foreground" />
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
            <Plus className="mr-1 h-4 w-4" />
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

// --- Main Page ---

const validTabs = ["installed", "browse", "order", "repositories"] as const;
type TabValue = (typeof validTabs)[number];

const AdminPlugins = () => {
  usePageTitle("Plugins");

  const { tab } = useParams<{ tab?: string }>();
  const navigate = useNavigate();

  const activeTab: TabValue = validTabs.includes(tab as TabValue)
    ? (tab as TabValue)
    : "installed";

  const handleTabChange = (value: string) => {
    if (value === "installed") {
      navigate("/settings/plugins");
    } else {
      navigate(`/settings/plugins/${value}`);
    }
  };

  return (
    <div>
      <div className="mb-6 md:mb-8">
        <h1 className="text-xl md:text-2xl font-semibold mb-1 md:mb-2">
          Plugins
        </h1>
        <p className="text-sm md:text-base text-muted-foreground">
          Manage installed plugins, browse available plugins, configure
          execution order, and manage repositories.
        </p>
      </div>

      <Tabs onValueChange={handleTabChange} value={activeTab}>
        <TabsList className="w-full justify-start overflow-x-auto">
          <TabsTrigger className="text-xs sm:text-sm" value="installed">
            Installed
          </TabsTrigger>
          <TabsTrigger className="text-xs sm:text-sm" value="browse">
            Browse
          </TabsTrigger>
          <TabsTrigger className="text-xs sm:text-sm" value="order">
            Order
          </TabsTrigger>
          <TabsTrigger className="text-xs sm:text-sm" value="repositories">
            Repos
          </TabsTrigger>
        </TabsList>

        <TabsContent value="installed">
          <InstalledTab />
        </TabsContent>

        <TabsContent value="browse">
          <BrowseTab />
        </TabsContent>

        <TabsContent value="order">
          <OrderTab />
        </TabsContent>

        <TabsContent value="repositories">
          <RepositoriesTab />
        </TabsContent>
      </Tabs>
    </div>
  );
};

export default AdminPlugins;
