import { ChangelogMarkdown } from "./ChangelogMarkdown";
import { ExternalLink } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { PluginVersion } from "@/hooks/queries/plugins";

export type PluginVersionCardState =
  | "installed"
  | "available"
  | "latest"
  | "older";

export interface PluginVersionCardProps {
  releaseUrl?: string;
  isUpdating?: boolean;
  onUpdate?: () => void;
  state: PluginVersionCardState;
  version: PluginVersion;
}

const formatReleaseDate = (
  raw: string,
): { absolute: string; relative: string } | null => {
  if (!raw) return null;
  // Date-only strings like "2026-04-14" are parsed as midnight UTC by
  // `new Date()`, which renders as the previous day west of UTC. Detect the
  // format and construct a local-midnight Date instead.
  const dateOnly = raw.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  const d = dateOnly
    ? new Date(+dateOnly[1], +dateOnly[2] - 1, +dateOnly[3])
    : new Date(raw);
  if (Number.isNaN(d.getTime())) return null;
  const absolute = d.toLocaleDateString(undefined, {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
  const diffDays = Math.floor(
    (Date.now() - d.getTime()) / (1000 * 60 * 60 * 24),
  );
  let relative: string;
  if (diffDays < 1) {
    relative = "today";
  } else if (diffDays === 1) {
    relative = "yesterday";
  } else if (diffDays < 30) {
    relative = `${diffDays} days ago`;
  } else if (diffDays < 365) {
    const months = Math.floor(diffDays / 30);
    relative = `${months} month${months === 1 ? "" : "s"} ago`;
  } else {
    const years = Math.floor(diffDays / 365);
    relative = `${years} year${years === 1 ? "" : "s"} ago`;
  }
  return { absolute, relative };
};

export const PluginVersionCard = ({
  releaseUrl,
  isUpdating,
  onUpdate,
  state,
  version,
}: PluginVersionCardProps) => {
  const date = formatReleaseDate(version.releaseDate);
  return (
    <div className="space-y-3 py-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="font-medium">v{version.version}</span>
          {state === "available" && <Badge>Available now</Badge>}
          {state === "latest" && <Badge>Latest</Badge>}
          {state === "installed" && <Badge variant="success">Installed</Badge>}
        </div>
        {date && (
          <span className="text-xs text-muted-foreground">
            Released {date.absolute} · {date.relative}
          </span>
        )}
      </div>

      {version.changelog && (
        <ChangelogMarkdown>{version.changelog}</ChangelogMarkdown>
      )}

      {(onUpdate || releaseUrl) && (
        <div className="flex items-center gap-2">
          {onUpdate && (
            <Button disabled={isUpdating} onClick={onUpdate} size="sm">
              {isUpdating ? "Updating…" : "Update now"}
            </Button>
          )}
          {releaseUrl && (
            <a
              className="inline-flex items-center gap-1 text-xs underline underline-offset-2"
              href={releaseUrl}
              rel="noopener noreferrer"
              target="_blank"
            >
              View release <ExternalLink className="h-3 w-3" />
            </a>
          )}
        </div>
      )}
    </div>
  );
};
