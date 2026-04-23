import { BadgeCheck, ChevronRight } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/libraries/utils";
import {
  PluginStatusActive,
  PluginStatusMalfunctioned,
  PluginStatusNotSupported,
  type PluginStatus,
} from "@/types/generated/models";

import { PluginLogo } from "./PluginLogo";

export interface PluginRowProps {
  actions?: ReactNode;
  capabilities: string[];
  description?: string;
  href: string;
  id: string;
  imageUrl?: string | null;
  isOfficial?: boolean;
  loadError?: string;
  name: string;
  repoName?: string;
  scope: string;
  status?: PluginStatus;
  updateAvailable?: string;
  version?: string;
}

const renderStatusBadge = (
  status: PluginStatus | undefined,
  loadError?: string,
) => {
  if (status === undefined || status === PluginStatusActive) return null;
  if (status === PluginStatusMalfunctioned) {
    return (
      <Badge title={loadError} variant="destructive">
        Error
      </Badge>
    );
  }
  if (status === PluginStatusNotSupported) {
    return <Badge variant="outline">Incompatible</Badge>;
  }
  return <Badge variant="secondary">Disabled</Badge>;
};

export const PluginRow = ({
  actions,
  capabilities,
  description,
  href,
  id,
  imageUrl,
  isOfficial,
  loadError,
  name,
  repoName,
  scope,
  status,
  updateAvailable,
  version,
}: PluginRowProps) => {
  const isInactive = status !== undefined && status !== PluginStatusActive;
  return (
    <Link
      className={cn(
        "group flex items-center gap-4 rounded-md border border-border px-4 py-3 transition-colors hover:bg-accent/30",
        isInactive && "opacity-50 saturate-50",
      )}
      to={href}
    >
      <PluginLogo
        id={id}
        imageUrl={imageUrl ?? undefined}
        scope={scope}
        size={40}
      />

      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate font-medium">{name}</span>
          {renderStatusBadge(status, loadError)}
          {updateAvailable && <Badge>Update {updateAvailable}</Badge>}
        </div>
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          {version && <span>v{version}</span>}
          {capabilities.map((cap) => (
            <Badge key={cap} variant="outline">
              {cap}
            </Badge>
          ))}
          {repoName && (
            <>
              <span aria-hidden="true" className="text-muted-foreground/50">
                ·
              </span>
              <span className="inline-flex items-center gap-1">
                {isOfficial && (
                  <BadgeCheck
                    aria-label="Official plugin"
                    className="h-3.5 w-3.5 text-primary"
                  />
                )}
                {repoName}
              </span>
            </>
          )}
        </div>
        {description && (
          <p className="line-clamp-2 text-xs text-muted-foreground">
            {description}
          </p>
        )}
      </div>

      {actions && (
        <div
          className="flex items-center gap-2"
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
          }}
          onMouseDown={(e) => e.stopPropagation()}
        >
          {actions}
        </div>
      )}

      <ChevronRight
        aria-hidden="true"
        className="h-4 w-4 flex-shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100"
      />
    </Link>
  );
};
