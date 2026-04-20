import {
  ArrowRightLeft,
  FileOutput,
  FileSearch,
  FolderOpen,
  Globe,
  Search,
  Terminal,
  Video,
  type LucideIcon,
} from "lucide-react";

import type { PluginCapabilities } from "@/hooks/queries/plugins";

export interface CapabilityDef {
  key: keyof PluginCapabilities;
  icon: LucideIcon;
  label: string;
  description: string;
  detail: (cap: PluginCapabilities) => string | undefined;
}

export const CAPABILITY_DEFS: CapabilityDef[] = [
  {
    key: "metadataEnricher",
    icon: Search,
    label: "Metadata Enrichment",
    description: "Searches external sources for book metadata",
    detail: (cap) =>
      cap.metadataEnricher?.fileTypes?.length
        ? cap.metadataEnricher.fileTypes.join(", ")
        : undefined,
  },
  {
    key: "inputConverter",
    icon: ArrowRightLeft,
    label: "Format Conversion",
    description: "Converts files between formats",
    detail: (cap) =>
      cap.inputConverter?.sourceTypes?.length && cap.inputConverter?.targetType
        ? `${cap.inputConverter.sourceTypes.join(", ")} \u2192 ${cap.inputConverter.targetType}`
        : undefined,
  },
  {
    key: "fileParser",
    icon: FileSearch,
    label: "File Parsing",
    description: "Extracts metadata from files",
    detail: (cap) =>
      cap.fileParser?.types?.length
        ? cap.fileParser.types.join(", ")
        : undefined,
  },
  {
    key: "outputGenerator",
    icon: FileOutput,
    label: "Output Generation",
    description: "Generates files in additional formats",
    detail: (cap) =>
      cap.outputGenerator?.sourceTypes?.length && cap.outputGenerator?.name
        ? `${cap.outputGenerator.sourceTypes.join(", ")} \u2192 ${cap.outputGenerator.name}`
        : undefined,
  },
  {
    key: "httpAccess",
    icon: Globe,
    label: "Network Access",
    description: "May make network requests to external services",
    detail: (cap) =>
      cap.httpAccess?.domains?.length
        ? cap.httpAccess.domains.join(", ")
        : undefined,
  },
  {
    key: "fileAccess",
    icon: FolderOpen,
    label: "File System Access",
    description: "Can access files beyond its sandboxed plugin directory",
    detail: (cap) => {
      if (cap.fileAccess?.level === "readwrite") return "read/write";
      if (cap.fileAccess?.level === "read") return "read-only";
      return undefined;
    },
  },
  {
    key: "ffmpegAccess",
    icon: Video,
    label: "FFmpeg Execution",
    description: "May invoke FFmpeg for media processing",
    detail: () => undefined,
  },
  {
    key: "shellAccess",
    icon: Terminal,
    label: "Shell Command Execution",
    description: "May execute shell commands on your system",
    detail: (cap) =>
      cap.shellAccess?.commands?.length
        ? cap.shellAccess.commands.join(", ")
        : undefined,
  },
];

// Permission-style capabilities: the ones that grant the plugin access to
// external systems (network, filesystem, ffmpeg, shell). The first four
// entries in CAPABILITY_DEFS describe what the plugin does for Shisho, not
// what it can reach outside of the sandbox.
const PERMISSION_KEYS: (keyof PluginCapabilities)[] = [
  "httpAccess",
  "fileAccess",
  "ffmpegAccess",
  "shellAccess",
];

export const PERMISSION_DEFS: CapabilityDef[] = CAPABILITY_DEFS.filter((def) =>
  PERMISSION_KEYS.includes(def.key),
);

export function filterDefsByCapabilities(
  defs: CapabilityDef[],
  capabilities: PluginCapabilities | undefined,
): CapabilityDef[] {
  if (!capabilities) return [];
  return defs.filter((def) => capabilities[def.key] != null);
}
