import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { AvailablePlugin } from "@/hooks/queries/plugins";

import { CapabilityRow } from "./CapabilityRow";
import { CAPABILITY_DEFS, filterDefsByCapabilities } from "./capabilityRows";

interface CapabilitiesWarningProps {
  isPending: boolean;
  onConfirm: () => void;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  plugin: AvailablePlugin | null;
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

  const rows = filterDefsByCapabilities(
    CAPABILITY_DEFS,
    latestVersion?.capabilities,
  );

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="overflow-x-hidden">
        <DialogHeader>
          <DialogTitle>Install {plugin.name}?</DialogTitle>
          <DialogDescription>
            {rows.length > 0
              ? "This plugin will be granted the following capabilities on your system. Review them before proceeding."
              : "This plugin does not declare any specific capabilities."}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-4">
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
        </DialogBody>

        <DialogFooter>
          <Button
            disabled={isPending}
            onClick={() => onOpenChange(false)}
            size="sm"
            variant="outline"
          >
            Cancel
          </Button>
          <Button disabled={isPending} onClick={onConfirm} size="sm">
            {isPending ? "Installing..." : "Install Plugin"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
