import { BadgeCheck, ExternalLink } from "lucide-react";
import { Fragment, type ReactNode } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import type { AvailablePlugin } from "@/hooks/queries/plugins";
import { PluginStatusActive, type Plugin } from "@/types/generated/models";

import { pluginAlertContent } from "./pluginAlertContent";
import { PluginHeroActions } from "./PluginHeroActions";
import { PluginLogo } from "./PluginLogo";

export interface PluginDetailHeroProps {
  available?: AvailablePlugin;
  canWrite: boolean;
  id: string;
  installed?: Plugin;
  isInstalling?: boolean;
  isTogglingEnabled?: boolean;
  isUpdating?: boolean;
  onInstall?: () => void;
  onToggleEnabled?: (enabled: boolean) => void;
  onUpdate?: () => void;
  repoName?: string;
  scope: string;
}

export const PluginDetailHero = ({
  available,
  canWrite,
  id,
  installed,
  isInstalling,
  isTogglingEnabled,
  isUpdating,
  onInstall,
  onToggleEnabled,
  onUpdate,
  repoName,
  scope,
}: PluginDetailHeroProps) => {
  const name = installed?.name ?? available?.name ?? id;
  const description = installed?.description ?? available?.description ?? "";
  const homepage = installed?.homepage ?? available?.homepage;
  const imageUrl = available?.imageUrl ?? undefined;
  const isOfficial = available?.is_official ?? false;
  const version = installed?.version;
  const updateAvailable = installed?.update_available_version ?? undefined;

  const alert = pluginAlertContent(installed);

  const metaParts: ReactNode[] = [];
  if (version) metaParts.push(`v${version}`);
  if (repoName) {
    metaParts.push(
      <span className="inline-flex items-center gap-1">
        {isOfficial && (
          <BadgeCheck
            aria-label="Official plugin"
            className="h-4 w-4 text-primary"
          />
        )}
        {repoName}
      </span>,
    );
  }
  if (homepage) {
    metaParts.push(
      <a
        className="inline-flex items-center gap-1 underline"
        href={homepage}
        rel="noopener noreferrer"
        target="_blank"
      >
        home page <ExternalLink aria-hidden="true" className="h-3 w-3" />
      </a>,
    );
  }

  return (
    <div className="flex gap-4 rounded-md border border-border p-4 md:p-6">
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

        {alert && (
          <Alert className="p-3" variant="destructive">
            <AlertTitle>{alert.title}</AlertTitle>
            {alert.body && (
              <AlertDescription className="break-words font-mono text-xs">
                {alert.body}
              </AlertDescription>
            )}
          </Alert>
        )}

        {metaParts.length > 0 && (
          <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
            {metaParts.map((part, i) => (
              <Fragment key={i}>
                {i > 0 && (
                  <span aria-hidden="true" className="text-muted-foreground/50">
                    ·
                  </span>
                )}
                {part}
              </Fragment>
            ))}
          </div>
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

      {!installed && available && canWrite && (
        <div className="flex flex-col items-end gap-3">
          {available.versions.some((v) => v.compatible) ? (
            <Button disabled={isInstalling} onClick={onInstall}>
              {isInstalling ? "Installing…" : "Install"}
            </Button>
          ) : (
            <Button disabled variant="outline">
              Incompatible
            </Button>
          )}
        </div>
      )}
    </div>
  );
};
