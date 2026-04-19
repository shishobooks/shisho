import {
  Film,
  FolderOpen,
  Globe,
  Terminal,
  type LucideIcon,
} from "lucide-react";
import { useMemo } from "react";

import type {
  AvailablePlugin,
  PluginCapabilities,
} from "@/hooks/queries/plugins";
import type { Plugin } from "@/types/generated/models";

interface PluginPermissionsProps {
  available?: AvailablePlugin;
  installed?: Plugin;
}

interface PermissionRow {
  detail: string;
  icon: LucideIcon;
  label: string;
}

const buildRows = (caps: PluginCapabilities | null): PermissionRow[] => {
  if (!caps) return [];
  const rows: PermissionRow[] = [];
  if (caps.httpAccess?.domains?.length) {
    rows.push({
      detail: caps.httpAccess.domains.join(", "),
      icon: Globe,
      label: "Network access",
    });
  }
  if (caps.fileAccess?.level) {
    rows.push({
      detail: caps.fileAccess.level,
      icon: FolderOpen,
      label: "Filesystem access",
    });
  }
  if (caps.ffmpegAccess) {
    rows.push({ detail: "", icon: Film, label: "FFmpeg access" });
  }
  if (caps.shellAccess?.commands?.length) {
    rows.push({
      detail: caps.shellAccess.commands.join(", "),
      icon: Terminal,
      label: "Shell commands",
    });
  }
  return rows;
};

export const PluginPermissions = ({
  available,
  installed,
}: PluginPermissionsProps) => {
  const caps = useMemo<PluginCapabilities | null>(() => {
    if (installed && available) {
      const match = available.versions.find(
        (v) => v.version === installed.version,
      );
      if (match?.capabilities) return match.capabilities;
    }
    return available?.versions?.[0]?.capabilities ?? null;
  }, [installed, available]);

  const rows = buildRows(caps);
  if (rows.length === 0) return null;

  return (
    <section className="space-y-3">
      <h2 className="text-lg font-semibold">Permissions</h2>
      <div className="space-y-2">
        {rows.map(({ detail, icon: Icon, label }) => (
          <div
            className="flex items-start gap-3 rounded-md border p-3"
            key={label}
          >
            <Icon className="mt-0.5 h-4 w-4 text-muted-foreground" />
            <div className="flex-1">
              <div className="text-sm font-medium">{label}</div>
              {detail && (
                <div className="text-xs text-muted-foreground">{detail}</div>
              )}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
};
