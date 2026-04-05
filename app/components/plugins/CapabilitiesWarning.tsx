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

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type {
  AvailablePlugin,
  PluginCapabilities,
} from "@/hooks/queries/plugins";

interface CapabilitiesWarningProps {
  isPending: boolean;
  onConfirm: () => void;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  plugin: AvailablePlugin | null;
}

interface CapabilityRowProps {
  description: string;
  detail?: string;
  icon: LucideIcon;
  label: string;
}

const CapabilityRow = ({
  description,
  detail,
  icon: Icon,
  label,
}: CapabilityRowProps) => (
  <div className="flex items-start gap-3 rounded-md border border-border p-3">
    <Icon className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
    <div>
      <p className="text-sm font-medium">{label}</p>
      <p className="text-xs text-muted-foreground">{description}</p>
      {detail && (
        <p className="mt-1 font-mono text-xs text-muted-foreground/70">
          {detail}
        </p>
      )}
    </div>
  </div>
);

interface CapabilityDef {
  key: keyof PluginCapabilities;
  icon: LucideIcon;
  label: string;
  description: string;
  detail: (cap: PluginCapabilities) => string | undefined;
}

const CAPABILITY_DEFS: CapabilityDef[] = [
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

function getCapabilityRows(capabilities: PluginCapabilities | undefined) {
  if (!capabilities) return [];
  return CAPABILITY_DEFS.filter((def) => capabilities[def.key] != null);
}

export const CapabilitiesWarning = ({
  isPending,
  onConfirm,
  onOpenChange,
  open,
  plugin,
}: CapabilitiesWarningProps) => {
  if (!plugin) return null;

  const latestVersion =
    plugin.versions.find((v) => v.compatible) ?? plugin.versions[0];

  const rows = getCapabilityRows(latestVersion?.capabilities);

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="overflow-x-hidden">
        <DialogHeader className="pr-8">
          <DialogTitle>Install {plugin.name}?</DialogTitle>
          <DialogDescription>
            {rows.length > 0
              ? "This plugin will be granted the following capabilities on your system. Review them before proceeding."
              : "This plugin does not declare any specific capabilities."}
          </DialogDescription>
        </DialogHeader>

        {rows.length > 0 && (
          <div className="space-y-2">
            {rows.map((def) => (
              <CapabilityRow
                description={def.description}
                detail={
                  latestVersion?.capabilities
                    ? def.detail(latestVersion.capabilities)
                    : undefined
                }
                icon={def.icon}
                key={def.key}
                label={def.label}
              />
            ))}
          </div>
        )}

        {latestVersion && (
          <div className="text-xs text-muted-foreground">
            Version: {latestVersion.version}
          </div>
        )}

        <DialogFooter>
          <Button
            disabled={isPending}
            onClick={() => onOpenChange(false)}
            variant="outline"
          >
            Cancel
          </Button>
          <Button disabled={isPending} onClick={onConfirm}>
            {isPending ? "Installing..." : "Install Plugin"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
