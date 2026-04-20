import { ArrowDown, ArrowUp, ListOrdered, Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  useAllPluginOrders,
  usePluginOrder,
  usePluginsInstalled,
  useSetPluginOrder,
  type PluginHookType,
  type PluginMode,
  type PluginOrder,
} from "@/hooks/queries/plugins";
import { useAuth } from "@/hooks/useAuth";
import { cn } from "@/libraries/utils";

const HOOK_TYPES: { label: string; value: PluginHookType }[] = [
  { label: "Metadata Enricher", value: "metadataEnricher" },
  { label: "File Parser", value: "fileParser" },
  { label: "Input Converter", value: "inputConverter" },
  { label: "Output Generator", value: "outputGenerator" },
];

export const AdvancedOrderSection = () => {
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
          <ListOrdered
            aria-hidden="true"
            className="mx-auto mb-3 h-8 w-8 text-muted-foreground"
          />
          <p className="text-sm text-muted-foreground">
            No plugins registered for this hook type.
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {displayOrder.map((item, index) => (
            <div
              className={cn(
                "flex items-center justify-between gap-3 rounded-md border p-3",
                item.mode === "disabled"
                  ? "border-border/50 opacity-60"
                  : item.mode === "manual_only"
                    ? "border-border/70 opacity-80"
                    : "border-border",
              )}
              key={`${item.scope}/${item.plugin_id}`}
            >
              <div className="flex items-center gap-3">
                <span className="font-mono text-xs text-muted-foreground">
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
                    <SelectTrigger
                      aria-label="When this plugin runs"
                      className="h-8 w-auto gap-2 text-xs"
                    >
                      <span className="text-muted-foreground">Runs:</span>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="enabled">
                        <div>
                          <div>On every scan</div>
                          <div className="text-xs text-muted-foreground">
                            Also available for manual identification
                          </div>
                        </div>
                      </SelectItem>
                      {selectedHookType === "metadataEnricher" && (
                        <SelectItem value="manual_only">
                          <div>
                            <div>Only when manually identifying</div>
                            <div className="text-xs text-muted-foreground">
                              Skipped during automated scans
                            </div>
                          </div>
                        </SelectItem>
                      )}
                      <SelectItem value="disabled">
                        <div>
                          <div>Never</div>
                          <div className="text-xs text-muted-foreground">
                            Plugin is inactive for this hook
                          </div>
                        </div>
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <Button
                    disabled={index === 0}
                    onClick={() => handleMove(index, "up")}
                    size="sm"
                    variant="ghost"
                  >
                    <ArrowUp aria-hidden="true" className="h-4 w-4" />
                  </Button>
                  <Button
                    disabled={index === displayOrder.length - 1}
                    onClick={() => handleMove(index, "down")}
                    size="sm"
                    variant="ghost"
                  >
                    <ArrowDown aria-hidden="true" className="h-4 w-4" />
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
              <Loader2
                aria-hidden="true"
                className="mr-2 h-4 w-4 animate-spin"
              />
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
