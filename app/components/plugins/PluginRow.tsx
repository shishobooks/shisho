import { PluginLogo } from "./PluginLogo";
import { ChevronRight } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/libraries/utils";

export interface PluginRowProps {
  actions?: ReactNode;
  author?: string;
  capabilities: string[];
  description?: string;
  disabled?: boolean;
  href: string;
  id: string;
  imageUrl?: string | null;
  name: string;
  scope: string;
  updateAvailable?: string;
  version?: string;
}

export const PluginRow = ({
  actions,
  author,
  capabilities,
  description,
  disabled,
  href,
  id,
  imageUrl,
  name,
  scope,
  updateAvailable,
  version,
}: PluginRowProps) => {
  return (
    <Link
      className={cn(
        "group flex items-center gap-4 rounded-md border border-border p-4 transition-colors hover:bg-accent/30",
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
          {author && <span>· {author}</span>}
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
          onClick={(e) => e.stopPropagation()}
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
