import { BadgeCheck, ChevronRight } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/libraries/utils";

import { PluginLogo } from "./PluginLogo";

export interface PluginRowProps {
  actions?: ReactNode;
  capabilities: string[];
  description?: string;
  disabled?: boolean;
  href: string;
  id: string;
  imageUrl?: string | null;
  isOfficial?: boolean;
  name: string;
  repoName?: string;
  scope: string;
  updateAvailable?: string;
  version?: string;
}

export const PluginRow = ({
  actions,
  capabilities,
  description,
  disabled,
  href,
  id,
  imageUrl,
  isOfficial,
  name,
  repoName,
  scope,
  updateAvailable,
  version,
}: PluginRowProps) => {
  return (
    <Link
      className={cn(
        "group flex items-center gap-4 rounded-md border border-border px-4 py-3 transition-colors hover:bg-accent/30",
        disabled && "opacity-50 saturate-50",
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
          {disabled && <Badge variant="secondary">Disabled</Badge>}
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
