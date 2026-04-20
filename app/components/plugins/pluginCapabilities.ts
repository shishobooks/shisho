import type {
  AvailablePlugin,
  PluginCapabilities,
} from "@/hooks/queries/plugins";
import type { Plugin } from "@/types/generated/models";

export const deriveCapabilityLabels = (
  caps: PluginCapabilities | null | undefined,
): string[] => {
  if (!caps) return [];
  const labels: string[] = [];
  if (caps.metadataEnricher) labels.push("Metadata enricher");
  if (caps.inputConverter) labels.push("Input converter");
  if (caps.fileParser) labels.push("File parser");
  if (caps.outputGenerator) labels.push("Output generator");
  return labels;
};

/**
 * Resolve capabilities for an installed plugin by cross-referencing the
 * available-plugin repository entry. Prefers the version matching the installed
 * version, falling back to versions[0] when no exact match exists (or when the
 * plugin isn't in a repository at all).
 */
export const resolveInstalledPluginCapabilities = (
  installed: Plugin,
  available: AvailablePlugin | undefined,
): PluginCapabilities | null => {
  if (!available) return null;
  const match = available.versions.find((v) => v.version === installed.version);
  if (match?.capabilities) return match.capabilities;
  return available.versions[0]?.capabilities ?? null;
};
