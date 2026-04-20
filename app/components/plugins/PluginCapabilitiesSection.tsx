import { CapabilityRow } from "./CapabilityRow";
import { CAPABILITY_DEFS, filterDefsByCapabilities } from "./capabilityRows";
import { useMemo } from "react";

import type {
  AvailablePlugin,
  PluginCapabilities,
} from "@/hooks/queries/plugins";
import type { Plugin } from "@/types/generated/models";

interface PluginCapabilitiesSectionProps {
  available?: AvailablePlugin;
  installed?: Plugin;
}

export const PluginCapabilitiesSection = ({
  available,
  installed,
}: PluginCapabilitiesSectionProps) => {
  const caps = useMemo<PluginCapabilities | null>(() => {
    if (installed && available) {
      const match = available.versions.find(
        (v) => v.version === installed.version,
      );
      if (match?.capabilities) return match.capabilities;
    }
    return available?.versions?.[0]?.capabilities ?? null;
  }, [installed, available]);

  const rows = filterDefsByCapabilities(CAPABILITY_DEFS, caps ?? undefined);
  if (rows.length === 0) return null;

  return (
    <section className="space-y-3">
      <h2 className="text-lg font-semibold">Capabilities</h2>
      <div className="space-y-2">
        {rows.map((def) => (
          <CapabilityRow
            description={def.description}
            detail={caps ? def.detail(caps) : undefined}
            icon={def.icon}
            key={def.key}
            label={def.label}
          />
        ))}
      </div>
    </section>
  );
};
