import {
  ArrowDown,
  ArrowUp,
  ListOrdered,
  Loader2,
  Package,
  Plus,
  RefreshCw,
  Star,
  Trash2,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { DiscoverTab } from "@/components/plugins/DiscoverTab";
import { InstalledTab } from "@/components/plugins/InstalledTab";
import { TabUpdatePill } from "@/components/plugins/TabUpdatePill";
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  useAddRepository,
  useAllPluginOrders,
  usePluginOrder,
  usePluginRepositories,
  usePluginsInstalled,
  useRemoveRepository,
  useSetPluginOrder,
  useSyncRepository,
  type PluginHookType,
  type PluginMode,
  type PluginOrder,
} from "@/hooks/queries/plugins";
import { useAuth } from "@/hooks/useAuth";
import { usePageTitle } from "@/hooks/usePageTitle";

// --- Order Tab ---

const HOOK_TYPES: { label: string; value: PluginHookType }[] = [
  { label: "Metadata Enricher", value: "metadataEnricher" },
  { label: "File Parser", value: "fileParser" },
  { label: "Input Converter", value: "inputConverter" },
  { label: "Output Generator", value: "outputGenerator" },
];

const OrderTab = () => {
  const { hasPermission } = useAuth();
  const canWrite = hasPermission("config", "write");
  const [selectedHookType, setSelectedHookType] =
    useState<PluginHookType>("metadataEnricher");
  const [hasAutoSelected, setHasAutoSelected] = useState(false);
  const { data: order, isLoading, error } = usePluginOrder(selectedHookType);
  const setPluginOrder = useSetPluginOrder();
  const { data: plugins } = usePluginsInstalled();

  // Prefetch all hook type orders to find the first non-empty one
  const allOrders = useAllPluginOrders(HOOK_TYPES.map((ht) => ht.value));
  const firstNonEmptyHookType = allOrders.every((q) => q.isSuccess)
    ? (HOOK_TYPES.find((_, i) => (allOrders[i].data?.length ?? 0) > 0)?.value ??
      null)
    : undefined; // undefined = still loading, null = all empty

  // Auto-select the first non-empty hook type once data loads
  useEffect(() => {
    if (hasAutoSelected || firstNonEmptyHookType === undefined) return;
    if (firstNonEmptyHookType) {
      setSelectedHookType(firstNonEmptyHookType);
    }
    setHasAutoSelected(true);
  }, [firstNonEmptyHookType, hasAutoSelected]);

  const [localOrder, setLocalOrder] = useState<PluginOrder[] | null>(null);

  // Sync local order with fetched data
  const displayOrder = localOrder ?? order ?? [];

  const hasOrderChanged =
    localOrder !== null &&
    (localOrder.length !== (order?.length ?? 0) ||
      localOrder.some(
        (item, i) =>
          item.scope !== order?.[i]?.scope ||
          item.plugin_id !== order?.[i]?.plugin_id ||
          item.mode !== order?.[i]?.mode,
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

  const handleModeChange = (index: number, mode: PluginMode) => {
    const newOrder = [...displayOrder];
    newOrder[index] = { ...newOrder[index], mode };
    setLocalOrder(newOrder);
  };

  const handleSave = () => {
    setPluginOrder.mutate(
      {
        hookType: selectedHookType,
        order: displayOrder.map((o) => ({
          scope: o.scope,
          id: o.plugin_id,
          mode: o.mode,
        })),
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
          <ListOrdered className="mx-auto mb-3 h-8 w-8 text-muted-foreground" />
          <p className="text-sm text-muted-foreground">
            No plugins registered for this hook type.
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {displayOrder.map((item, index) => (
            <div
              className={`flex items-center justify-between gap-3 rounded-md border p-3 ${
                item.mode === "disabled"
                  ? "border-border/50 opacity-60"
                  : item.mode === "manual_only"
                    ? "border-border/70 opacity-80"
                    : "border-border"
              }`}
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
                  <Select
                    onValueChange={(value) =>
                      handleModeChange(index, value as PluginMode)
                    }
                    value={item.mode}
                  >
                    <SelectTrigger className="w-[140px] h-8 text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="enabled">Enabled</SelectItem>
                      {selectedHookType === "metadataEnricher" && (
                        <SelectItem value="manual_only">Manual Only</SelectItem>
                      )}
                      <SelectItem value="disabled">Disabled</SelectItem>
                    </SelectContent>
                  </Select>
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

// "browse" is kept for backward-compat URL slugs; both resolve to "discover".
const validTabs = [
  "installed",
  "discover",
  "browse",
  "order",
  "repositories",
] as const;
type TabValue = (typeof validTabs)[number];

const normalizeTab = (tab: string | undefined): TabValue => {
  if (tab === "browse") return "discover";
  return validTabs.includes(tab as TabValue) ? (tab as TabValue) : "installed";
};

const AdminPlugins = () => {
  usePageTitle("Plugins");

  const { hasPermission } = useAuth();
  const canWrite = hasPermission("config", "write");

  const { tab } = useParams<{ tab?: string }>();
  const navigate = useNavigate();

  const activeTab: TabValue = normalizeTab(tab);

  const { data: plugins = [] } = usePluginsInstalled();
  const updateCount = useMemo(
    () => plugins.filter((p) => !!p.update_available_version).length,
    [plugins],
  );

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
          Manage installed plugins, discover available plugins, configure
          execution order, and manage repositories.
        </p>
      </div>

      <Tabs onValueChange={handleTabChange} value={activeTab}>
        <TabsList className="w-full justify-start overflow-x-auto">
          <TabsTrigger className="text-xs sm:text-sm" value="installed">
            Installed <TabUpdatePill count={updateCount} />
          </TabsTrigger>
          <TabsTrigger className="text-xs sm:text-sm" value="discover">
            Discover
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

        <TabsContent value="discover">
          <DiscoverTab canWrite={canWrite} />
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
