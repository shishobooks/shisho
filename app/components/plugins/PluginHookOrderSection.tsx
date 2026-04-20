import { resolveInstalledPluginCapabilities } from "./pluginCapabilities";
import { ExternalLink } from "lucide-react";
import { useMemo } from "react";
import { Link } from "react-router-dom";

import {
  useAllPluginOrders,
  usePluginsAvailable,
  type PluginHookType,
  type PluginMode,
} from "@/hooks/queries/plugins";
import type { Plugin } from "@/types/generated/models";

interface PluginHookOrderSectionProps {
  installed: Plugin;
}

const HOOK_TYPES: {
  capabilityKey:
    | "metadataEnricher"
    | "inputConverter"
    | "fileParser"
    | "outputGenerator";
  label: string;
  value: PluginHookType;
}[] = [
  {
    capabilityKey: "metadataEnricher",
    label: "Metadata enricher",
    value: "metadataEnricher",
  },
  {
    capabilityKey: "inputConverter",
    label: "Input converter",
    value: "inputConverter",
  },
  { capabilityKey: "fileParser", label: "File parser", value: "fileParser" },
  {
    capabilityKey: "outputGenerator",
    label: "Output generator",
    value: "outputGenerator",
  },
];

const modeLabel = (mode: PluginMode | undefined): string => {
  switch (mode) {
    case "enabled":
      return "Runs for every new file";
    case "manual_only":
      return "Runs only when manually identifying";
    case "disabled":
      return "Does not run";
    default:
      return "Not registered";
  }
};

export const PluginHookOrderSection = ({
  installed,
}: PluginHookOrderSectionProps) => {
  const { data: available } = usePluginsAvailable();
  const availableEntry = available?.find(
    (a) => a.scope === installed.scope && a.id === installed.id,
  );
  const caps = resolveInstalledPluginCapabilities(installed, availableEntry);

  const activeHooks = useMemo(
    () => HOOK_TYPES.filter((ht) => caps?.[ht.capabilityKey] != null),
    [caps],
  );

  const orderQueries = useAllPluginOrders(activeHooks.map((h) => h.value));

  if (activeHooks.length === 0) return null;

  return (
    <section className="space-y-3 rounded-md border border-border p-4 md:p-6">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">Hook execution order</h2>
        <Link
          className="inline-flex items-center gap-1 text-xs text-muted-foreground underline hover:text-foreground"
          to="/settings/plugins?advanced=order"
        >
          Edit in advanced settings
          <ExternalLink aria-hidden="true" className="h-3 w-3" />
        </Link>
      </div>

      <div className="divide-y divide-border">
        {activeHooks.map((ht, i) => {
          const order = orderQueries[i]?.data ?? [];
          const idx = order.findIndex(
            (o) => o.scope === installed.scope && o.plugin_id === installed.id,
          );
          const entry = idx >= 0 ? order[idx] : undefined;
          const position = idx >= 0 ? `#${idx + 1} of ${order.length}` : null;

          return (
            <div
              className="flex items-center justify-between gap-3 py-3"
              key={ht.value}
            >
              <div className="text-sm font-medium">{ht.label}</div>
              <div className="text-right text-xs text-muted-foreground">
                {position && <span className="mr-2">{position}</span>}
                <span>{modeLabel(entry?.mode as PluginMode | undefined)}</span>
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
};
