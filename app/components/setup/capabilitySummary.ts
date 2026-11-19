import type { PluginCapabilities } from "@/hooks/queries/plugins";
import { formatCount } from "@/utils/format";

// How many sites the summary names before it counts the rest.
const NAMED_SITES = 2;

const sites = (domains: string[]): string => {
  const names = [...new Set(domains.map((d) => d.replace(/^\*\./, "")))];
  if (names.length === 1) return names[0];
  if (names.length === 2) return `${names[0]} and ${names[1]}`;
  const named = names.slice(0, NAMED_SITES).join(", ");
  return `${named}, and ${formatCount(names.length - NAMED_SITES, "more site", "more sites")}`;
};

/**
 * One line saying what a plugin can reach beyond reading metadata, such as
 * the sites it connects to. The install dialog's capability rows give the
 * full list.
 */
export const summarizeCapabilities = (
  caps: PluginCapabilities | null | undefined,
): string => {
  const parts: string[] = [];
  if (caps?.httpAccess) {
    const domains = caps.httpAccess.domains ?? [];
    parts.push(
      domains.length > 0
        ? `Connects to ${sites(domains)}`
        : "Connects to the internet",
    );
  }
  if (caps?.fileAccess) {
    parts.push(
      caps.fileAccess.level === "readwrite"
        ? "Reads and writes files outside its folder"
        : "Reads files outside its folder",
    );
  }
  if (caps?.ffmpegAccess) parts.push("Runs FFmpeg");
  if (caps?.shellAccess) parts.push("Runs shell commands");
  return parts.length > 0 ? parts.join(" · ") : "Needs no special access";
};
