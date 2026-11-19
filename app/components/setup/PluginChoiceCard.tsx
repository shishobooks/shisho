import { ChevronDown } from "lucide-react";
import { useId, useState } from "react";

import { CapabilityRow } from "@/components/plugins/CapabilityRow";
import {
  CAPABILITY_DEFS,
  filterDefsByCapabilities,
} from "@/components/plugins/capabilityRows";
import { PluginLogo } from "@/components/plugins/PluginLogo";
import { summarizeCapabilities } from "@/components/setup/capabilitySummary";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import type { AvailablePlugin } from "@/hooks/queries/plugins";
import { cn } from "@/libraries/utils";

interface PluginChoiceCardProps {
  plugin: AvailablePlugin;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  /** Already installed, so there's nothing to choose. */
  installed?: boolean;
}

/**
 * A plugin Setup can install. Ticking it installs it at Finish and approves
 * what it can access, which the card summarizes in one line and lists in full
 * on request, as the install dialog does.
 */
export const PluginChoiceCard = ({
  plugin,
  checked,
  onCheckedChange,
  installed,
}: PluginChoiceCardProps) => {
  const [expanded, setExpanded] = useState(false);
  const nameId = useId();
  const checkboxId = useId();
  const detailsId = useId();
  const version = plugin.versions.find((v) => v.compatible);
  const capabilities = (version ?? plugin.versions[0])?.capabilities;
  const rows = filterDefsByCapabilities(CAPABILITY_DEFS, capabilities);
  const unavailable = !version;

  return (
    <article
      aria-labelledby={nameId}
      className={cn(
        "rounded-md border p-3 transition-colors sm:p-4",
        checked || installed
          ? "border-primary/60 bg-primary/5"
          : "border-border",
      )}
    >
      <div className="flex items-start gap-3">
        <Checkbox
          aria-label={`Install ${plugin.name}`}
          checked={installed || checked}
          className="mt-1"
          disabled={installed || unavailable}
          id={checkboxId}
          onCheckedChange={(value) => onCheckedChange(value === true)}
        />
        <PluginLogo
          className="hidden sm:inline-flex"
          id={plugin.id}
          imageUrl={plugin.imageUrl}
          scope={plugin.scope}
          size={40}
        />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-baseline gap-x-2">
            <h3 className="text-sm font-medium" id={nameId}>
              <Label className="cursor-pointer" htmlFor={checkboxId}>
                {plugin.name}
              </Label>
            </h3>
            {installed && (
              <span className="text-xs text-muted-foreground">Installed</span>
            )}
            {unavailable && (
              <span className="text-xs text-muted-foreground">
                Needs a newer version of Shisho
              </span>
            )}
          </div>
          {plugin.overview && (
            <p className="text-sm text-muted-foreground">{plugin.overview}</p>
          )}
          <p className="mt-1 break-words text-xs text-muted-foreground">
            {summarizeCapabilities(capabilities)}
          </p>
          {rows.length > 0 && (
            <>
              <Button
                aria-controls={detailsId}
                aria-expanded={expanded}
                className="mt-1 gap-1 text-xs"
                onClick={() => setExpanded((v) => !v)}
                variant="link"
              >
                What it can access
                <ChevronDown
                  aria-hidden="true"
                  className={cn(
                    "transition-transform",
                    expanded && "rotate-180",
                  )}
                />
              </Button>
              {expanded && (
                <div className="divide-y divide-border" id={detailsId}>
                  {rows.map((def) => (
                    <CapabilityRow
                      description={def.description}
                      detail={
                        capabilities ? def.detail(capabilities) : undefined
                      }
                      flat
                      icon={def.icon}
                      key={def.key}
                      label={def.label}
                    />
                  ))}
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </article>
  );
};
