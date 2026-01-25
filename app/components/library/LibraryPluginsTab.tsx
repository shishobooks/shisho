import { ArrowDown, ArrowUp, Loader2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

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
import { Switch } from "@/components/ui/switch";
import {
  useLibraryPluginOrder,
  useResetLibraryPluginOrder,
  useSetLibraryPluginOrder,
  type LibraryPluginOrderPlugin,
  type PluginHookType,
} from "@/hooks/queries/plugins";

const HOOK_TYPES: { label: string; value: PluginHookType }[] = [
  { label: "Input Converter", value: "inputConverter" },
  { label: "File Parser", value: "fileParser" },
  { label: "Output Generator", value: "outputGenerator" },
  { label: "Metadata Enricher", value: "metadataEnricher" },
];

interface Props {
  libraryId: string;
}

const LibraryPluginsTab = ({ libraryId }: Props) => {
  const [selectedHookType, setSelectedHookType] =
    useState<PluginHookType>("metadataEnricher");
  const { data, isLoading, error } = useLibraryPluginOrder(
    libraryId,
    selectedHookType,
  );
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
          item.enabled !== data?.plugins?.[i]?.enabled,
      ));

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

  const handleToggle = (index: number) => {
    const newPlugins = [...displayPlugins];
    newPlugins[index] = {
      ...newPlugins[index],
      enabled: !newPlugins[index].enabled,
    };
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
          enabled: p.enabled,
        })),
      },
      {
        onSuccess: () => {
          setLocalPlugins(null);
          toast.success("Library plugin order saved.");
        },
        onError: (err) => {
          toast.error(`Failed to save: ${err.message}`);
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
          toast.error(`Failed to reset: ${err.message}`);
        },
      },
    );
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

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
              className={`flex items-center justify-between gap-3 rounded-md border p-3 ${
                plugin.enabled ? "border-border" : "border-border/50 opacity-60"
              }`}
              key={`${plugin.scope}/${plugin.id}`}
            >
              <div className="flex items-center gap-3">
                <span className="text-xs font-mono text-muted-foreground">
                  {index + 1}
                </span>
                <span className="text-sm">{plugin.name}</span>
                <Badge variant="secondary">{plugin.scope}</Badge>
              </div>
              {(isCustomized || localPlugins) && (
                <div className="flex items-center gap-2">
                  <Switch
                    checked={plugin.enabled}
                    onCheckedChange={() => handleToggle(index)}
                  />
                  <Button
                    disabled={index === 0}
                    onClick={() => handleMove(index, "up")}
                    size="sm"
                    variant="ghost"
                  >
                    <ArrowUp className="h-4 w-4" />
                  </Button>
                  <Button
                    disabled={index === displayPlugins.length - 1}
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
