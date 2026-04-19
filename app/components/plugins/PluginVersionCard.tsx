import { ChangelogMarkdown } from "./ChangelogMarkdown";
import { ExternalLink } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { PluginVersion } from "@/hooks/queries/plugins";
import { cn } from "@/libraries/utils";

export type PluginVersionCardState =
  | "installed"
  | "available"
  | "latest"
  | "older";

export interface PluginVersionCardProps {
  gitHubReleaseUrl?: string;
  isUpdating?: boolean;
  onUpdate?: () => void;
  state: PluginVersionCardState;
  version: PluginVersion;
}

const formatReleaseDate = (
  raw: string,
): { absolute: string; relative: string } | null => {
  if (!raw) return null;
  const d = new Date(raw);
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
  gitHubReleaseUrl,
  isUpdating,
  onUpdate,
  state,
  version,
}: PluginVersionCardProps) => {
  const date = formatReleaseDate(version.releaseDate);
  return (
    <div
      className={cn(
        "space-y-3 rounded-md border p-4",
        (state === "available" || state === "latest") &&
          "border-primary/50 bg-primary/5",
      )}
    >
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="font-medium">v{version.version}</span>
          {state === "available" && <Badge>Available now</Badge>}
          {state === "latest" && <Badge>Latest</Badge>}
          {state === "installed" && (
            <Badge variant="secondary">Installed</Badge>
          )}
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

      {(onUpdate || gitHubReleaseUrl) && (
        <div className="flex items-center gap-2">
          {onUpdate && (
            <Button disabled={isUpdating} onClick={onUpdate} size="sm">
              {isUpdating ? "Updating…" : "Update now"}
            </Button>
          )}
          {gitHubReleaseUrl && (
            <a
              className="inline-flex items-center gap-1 text-xs underline underline-offset-2"
              href={gitHubReleaseUrl}
              rel="noopener noreferrer"
              target="_blank"
            >
              View release on GitHub <ExternalLink className="h-3 w-3" />
            </a>
          )}
        </div>
      )}
    </div>
  );
};
