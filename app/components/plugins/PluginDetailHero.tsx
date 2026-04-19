import { PluginHeroActions } from "./PluginHeroActions";
import { PluginLogo } from "./PluginLogo";
import { ExternalLink } from "lucide-react";
import type { ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import type { AvailablePlugin } from "@/hooks/queries/plugins";
import { PluginStatusActive, type Plugin } from "@/types/generated/models";

export interface PluginDetailHeroProps {
  available?: AvailablePlugin;
  canWrite: boolean;
  id: string;
  installed?: Plugin;
  isTogglingEnabled?: boolean;
  isUpdating?: boolean;
  onToggleEnabled?: (enabled: boolean) => void;
  onUpdate?: () => void;
  scope: string;
}

export const PluginDetailHero = ({
  available,
  canWrite,
  id,
  installed,
  isTogglingEnabled,
  isUpdating,
  onToggleEnabled,
  onUpdate,
  scope,
}: PluginDetailHeroProps) => {
  const name = installed?.name ?? available?.name ?? id;
  const description = installed?.description ?? available?.description ?? "";
  const author = installed?.author ?? available?.author;
  const homepage = installed?.homepage ?? available?.homepage;
  const imageUrl = available?.imageUrl ?? undefined;
  const version = installed?.version;
  const updateAvailable = installed?.update_available_version ?? undefined;

  const metaParts: ReactNode[] = [];
  if (version) metaParts.push(`v${version}`);
  if (author) metaParts.push(`by ${author}`);
  if (homepage) {
    metaParts.push(
      <a
        className="inline-flex items-center gap-1 underline"
        href={homepage}
        rel="noopener noreferrer"
        target="_blank"
      >
        homepage <ExternalLink aria-hidden="true" className="h-3 w-3" />
      </a>,
    );
  }

  return (
    <div className="flex gap-4 rounded-md border border-border p-6">
      <PluginLogo id={id} imageUrl={imageUrl} scope={scope} size={64} />

      <div className="flex-1 space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-xl font-semibold">{name}</h1>
          {updateAvailable && (
            <Badge variant="default">
              Update available — {updateAvailable}
            </Badge>
          )}
          {!installed && available && (
            <Badge variant="secondary">Not installed</Badge>
          )}
        </div>

        {metaParts.length > 0 && (
          <p className="text-sm text-muted-foreground">
            {metaParts.map((part, i) => (
              <span key={i}>
                {i > 0 && " · "}
                {part}
              </span>
            ))}
          </p>
        )}

        {description && (
          <p className="text-sm text-muted-foreground">{description}</p>
        )}
      </div>

      {installed && (
        <div className="flex flex-col items-end gap-3">
          {canWrite && updateAvailable && (
            <Button disabled={isUpdating} onClick={onUpdate}>
              {isUpdating ? "Updating…" : `Update to ${updateAvailable}`}
            </Button>
          )}
          {canWrite && (
            <div className="flex items-center gap-2">
              <Label htmlFor="plugin-enabled">Enabled</Label>
              <Switch
                checked={installed.status === PluginStatusActive}
                disabled={isTogglingEnabled}
                id="plugin-enabled"
                onCheckedChange={onToggleEnabled}
              />
            </div>
          )}
          <PluginHeroActions canWrite={canWrite} plugin={installed} />
        </div>
      )}
    </div>
  );
};
