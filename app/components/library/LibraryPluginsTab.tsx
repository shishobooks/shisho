import { ArrowDown, ArrowUp, Loader2 } from "lucide-react";
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
  useLibraryPluginOrder,
  useResetLibraryPluginOrder,
  useSetLibraryPluginOrder,
  type LibraryPluginOrderPlugin,
  type PluginHookType,
} from "@/hooks/queries/plugins";
import { toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";

const HOOK_TYPES: { label: string; value: PluginHookType }[] = [
  { label: "Input Converter", value: "inputConverter" },
  { label: "File Parser", value: "fileParser" },
  { label: "Metadata Enricher", value: "metadataEnricher" },
];

interface Props {
  libraryId: string;
  onHasChangesChange?: (hasChanges: boolean) => void;
}

const LibraryPluginsTab = ({ libraryId, onHasChangesChange }: Props) => {
  const [selectedHookType, setSelectedHookType] =
    useState<PluginHookType>("metadataEnricher");
  const orderQuery = useLibraryPluginOrder(libraryId, selectedHookType);
  const { data, isLoading } = orderQuery;
  const setOrder = useSetLibraryPluginOrder();
  const resetOrder = useResetLibraryPluginOrder();

  const [localPlugins, setLocalPlugins] = useState<
    LibraryPluginOrderPlugin[] | null
  >(null);

  const displayPlugins = localPlugins ?? data?.plugins ?? [];
  const isCustomized =
    localPlugins !== null ? true : (data?.customized ?? false);

  const hasChanged =
    localPlugins !== null &&
    (localPlugins.length !== (data?.plugins?.length ?? 0) ||
      localPlugins.some(
        (item, i) =>
          item.scope !== data?.plugins?.[i]?.scope ||
          item.id !== data?.plugins?.[i]?.id ||
          item.mode !== data?.plugins?.[i]?.mode,
      ));

  // Notify parent of changes
  useEffect(() => {
    onHasChangesChange?.(hasChanged);
  }, [hasChanged, onHasChangesChange]);

  const handleMove = (index: number, direction: "up" | "down") => {
    const newPlugins = [...displayPlugins];
    const targetIndex = direction === "up" ? index - 1 : index + 1;
    if (targetIndex < 0 || targetIndex >= newPlugins.length) return;
    [newPlugins[index], newPlugins[targetIndex]] = [
      newPlugins[targetIndex],
      newPlugins[index],
    ];
    setLocalPlugins(newPlugins);
  };

  const handleModeChange = (
    index: number,
    mode: LibraryPluginOrderPlugin["mode"],
  ) => {
    const newPlugins = [...displayPlugins];
    newPlugins[index] = { ...newPlugins[index], mode };
    setLocalPlugins(newPlugins);
  };

  const handleCustomize = () => {
    setLocalPlugins([...(data?.plugins ?? [])]);
  };

  const handleSave = () => {
    setOrder.mutate(
      {
        libraryId,
        hookType: selectedHookType,
        plugins: displayPlugins.map((p) => ({
          scope: p.scope,
          id: p.id,
          mode: p.mode,
        })),
      },
      {
        onSuccess: () => {
          setLocalPlugins(null);
          toast.success("Library plugin order saved.");
        },
        onError: (err) => {
          toastRequestError(err, "Failed to save plugin order");
        },
      },
    );
  };

  const handleReset = () => {
    resetOrder.mutate(
      { libraryId, hookType: selectedHookType },
      {
        onSuccess: () => {
          setLocalPlugins(null);
          toast.success("Reset to global default.");
        },
        onError: (err) => {
          toastRequestError(err, "Failed to reset plugin order");
        },
      },
    );
  };

  if (isLoading) {
    return <LoadingSpinner />;
  }

  if (orderQuery.error && !data) {
    return (
      <QueryError fallback="Failed to load plugin order" query={orderQuery} />
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <Label className="text-sm" htmlFor="lib-hook-type-select">
          Hook Type
        </Label>
        <Select
          onValueChange={(value) => {
            setSelectedHookType(value as PluginHookType);
            setLocalPlugins(null);
          }}
          value={selectedHookType}
        >
          <SelectTrigger className="w-[200px]" id="lib-hook-type-select">
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
        {isCustomized && !localPlugins && (
          <Badge variant="secondary">Customized</Badge>
        )}
      </div>

      {displayPlugins.length === 0 ? (
        <div className="py-8 text-center">
          <p className="text-sm text-muted-foreground">
            No plugins registered for this hook type.
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {displayPlugins.map((plugin, index) => (
            <div
              className={cn(
                "flex flex-wrap items-center gap-x-3 gap-y-2 rounded-md border p-3",
                plugin.mode === "disabled"
                  ? "border-border/50 opacity-60"
                  : plugin.mode === "manual_only"
                    ? "border-border/70 opacity-80"
                    : "border-border",
              )}
              key={`${plugin.scope}/${plugin.id}`}
            >
              <div className="flex min-w-0 flex-auto flex-wrap items-center gap-x-3 gap-y-1">
                <span className="text-xs font-mono text-muted-foreground">
                  {index + 1}
                </span>
                <span className="min-w-0 break-words text-sm">
                  {plugin.name}
                </span>
                <Badge
                  className="max-w-full whitespace-normal break-all"
                  variant="secondary"
                >
                  {plugin.scope}
                </Badge>
              </div>
              {(isCustomized || localPlugins) && (
                <div className="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-2">
                  <Select
                    onValueChange={(value) =>
                      handleModeChange(
                        index,
                        value as LibraryPluginOrderPlugin["mode"],
                      )
                    }
                    value={plugin.mode}
                  >
                    <SelectTrigger
                      aria-label={`Mode for ${plugin.name}`}
                      className="w-[140px] h-8 text-xs"
                    >
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
                  <div className="flex shrink-0 items-center gap-2">
                    <Button
                      aria-label="Move plugin up"
                      disabled={index === 0}
                      onClick={() => handleMove(index, "up")}
                      size="sm"
                      variant="ghost"
                    >
                      <ArrowUp className="h-4 w-4" />
                    </Button>
                    <Button
                      aria-label="Move plugin down"
                      disabled={index === displayPlugins.length - 1}
                      onClick={() => handleMove(index, "down")}
                      size="sm"
                      variant="ghost"
                    >
                      <ArrowDown className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      <div className="flex gap-2">
        {!isCustomized && !localPlugins && displayPlugins.length > 0 && (
          <Button onClick={handleCustomize} size="sm" variant="outline">
            Customize
          </Button>
        )}
        {(isCustomized || localPlugins) && (
          <>
            <Button
              disabled={!hasChanged || setOrder.isPending}
              onClick={handleSave}
              size="sm"
            >
              {setOrder.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                "Save"
              )}
            </Button>
            <Button
              disabled={resetOrder.isPending}
              onClick={handleReset}
              size="sm"
              variant="outline"
            >
              Reset to Default
            </Button>
          </>
        )}
      </div>
    </div>
  );
};

export default LibraryPluginsTab;
