import { ArrowDown, ArrowUp, ListOrdered, Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
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
import { useCan } from "@/hooks/useCan";
import { toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";

// Each option's description shows only in the open list. The trigger shows
// the label alone, or the description would set its width.
const MODE_OPTIONS: {
  description: string;
  label: string;
  value: PluginMode;
}[] = [
  {
    description: "Also available for manual identification",
    label: "For every new file",
    value: "enabled",
  },
  {
    description: "Skipped when new files are added",
    label: "Only when manually identifying",
    value: "manual_only",
  },
  {
    description: "Plugin is inactive for this hook",
    label: "Never",
    value: "disabled",
  },
];

const HOOK_TYPES: { label: string; value: PluginHookType }[] = [
  { label: "Metadata Enricher", value: "metadataEnricher" },
  { label: "File Parser", value: "fileParser" },
  { label: "Input Converter", value: "inputConverter" },
];

export const AdvancedOrderSection = () => {
  const canWrite = useCan("config:write");
  const canReadBooks = useCan("books:read");
  const [selectedHookType, setSelectedHookType] =
    useState<PluginHookType>("metadataEnricher");
  const [hasAutoSelected, setHasAutoSelected] = useState(false);
  const orderQuery = usePluginOrder(selectedHookType);
  const { data: order, isLoading } = orderQuery;
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
          toastRequestError(err, "Failed to save order");
        },
      },
    );
  };

  const getPluginName = (scope: string, pluginId: string) => {
    const plugin = plugins?.find((p) => p.scope === scope && p.id === pluginId);
    return plugin?.name ?? `${scope}/${pluginId}`;
  };

  // The order routes are the Books Read lookups the identify dialog uses, so a
  // role that opens this page with Config Read alone cannot load them.
  if (!canReadBooks) {
    return (
      <p className="text-sm text-muted-foreground">
        Viewing the plugin order requires Books Read.
      </p>
    );
  }
  if (isLoading) return <LoadingSpinner />;
  if (orderQuery.error && !order) {
    return (
      <QueryError fallback="Failed to load plugin order" query={orderQuery} />
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
                "flex flex-wrap items-center gap-x-3 gap-y-2 rounded-md border p-3",
                item.mode === "disabled"
                  ? "border-border/50 opacity-60"
                  : item.mode === "manual_only"
                    ? "border-border/70 opacity-80"
                    : "border-border",
              )}
              key={`${item.scope}/${item.plugin_id}`}
            >
              <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1 sm:flex-auto">
                <span className="font-mono text-xs text-muted-foreground">
                  {index + 1}
                </span>
                <span className="min-w-0 break-words text-sm">
                  {getPluginName(item.scope, item.plugin_id)}
                </span>
                <Badge
                  className="max-w-full whitespace-normal break-all"
                  variant="secondary"
                >
                  {item.scope}
                </Badge>
              </div>
              {canWrite && (
                <>
                  {/* On phones the select takes a line of its own below the
                      name and arrows, and its label wraps rather than being
                      cut off on the narrowest screens. */}
                  <Select
                    onValueChange={(value) =>
                      handleModeChange(index, value as PluginMode)
                    }
                    value={item.mode}
                  >
                    <SelectTrigger
                      aria-label="When this plugin runs"
                      className="order-last h-auto min-h-8 basis-full justify-start gap-2 whitespace-normal text-left text-xs sm:order-none sm:h-8 sm:w-auto sm:basis-auto sm:whitespace-nowrap [&>span]:line-clamp-2 sm:[&>span]:line-clamp-1 [&>svg]:ml-auto"
                    >
                      <span className="shrink-0 text-muted-foreground">
                        Runs:
                      </span>
                      <SelectValue>
                        {
                          MODE_OPTIONS.find(
                            (option) => option.value === item.mode,
                          )?.label
                        }
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      {MODE_OPTIONS.filter(
                        (option) =>
                          option.value !== "manual_only" ||
                          selectedHookType === "metadataEnricher",
                      ).map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          <div>
                            <div>{option.label}</div>
                            <div className="text-xs text-muted-foreground">
                              {option.description}
                            </div>
                          </div>
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <div className="flex shrink-0 items-center gap-1">
                    <Button
                      aria-label="Move plugin up"
                      disabled={index === 0}
                      onClick={() => handleMove(index, "up")}
                      size="sm"
                      variant="ghost"
                    >
                      <ArrowUp aria-hidden="true" className="h-4 w-4" />
                    </Button>
                    <Button
                      aria-label="Move plugin down"
                      disabled={index === displayOrder.length - 1}
                      onClick={() => handleMove(index, "down")}
                      size="sm"
                      variant="ghost"
                    >
                      <ArrowDown aria-hidden="true" className="h-4 w-4" />
                    </Button>
                  </div>
                </>
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
