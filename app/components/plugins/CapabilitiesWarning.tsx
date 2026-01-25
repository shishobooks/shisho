import {
  FileText,
  Globe,
  Layers,
  Shield,
  Video,
  type LucideIcon,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { AvailablePlugin } from "@/hooks/queries/plugins";

interface CapabilitiesWarningProps {
  isPending: boolean;
  onConfirm: () => void;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  plugin: AvailablePlugin | null;
}

interface CapabilityRowProps {
  description: string;
  icon: LucideIcon;
  label: string;
}

const CapabilityRow = ({
  description,
  icon: Icon,
  label,
}: CapabilityRowProps) => (
  <div className="flex items-start gap-3 rounded-md border border-border p-3">
    <Icon className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
    <div>
      <p className="text-sm font-medium">{label}</p>
      <p className="text-xs text-muted-foreground">{description}</p>
    </div>
  </div>
);

export const CapabilitiesWarning = ({
  isPending,
  onConfirm,
  onOpenChange,
  open,
  plugin,
}: CapabilitiesWarningProps) => {
  if (!plugin) return null;

  const latestVersion = plugin.versions[0];

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="overflow-x-hidden">
        <DialogHeader className="pr-8">
          <DialogTitle>Install {plugin.name}?</DialogTitle>
          <DialogDescription>
            This plugin will be granted the following capabilities on your
            system. Review them before proceeding.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-2">
          <CapabilityRow
            description="The plugin can hook into file processing pipelines"
            icon={Layers}
            label="Hook Integration"
          />
          <CapabilityRow
            description="The plugin can read and process files in your libraries"
            icon={FileText}
            label="File System Access"
          />
          <CapabilityRow
            description="The plugin may make network requests to external services"
            icon={Globe}
            label="Network Access"
          />
          <CapabilityRow
            description="The plugin may invoke FFmpeg for media processing"
            icon={Video}
            label="FFmpeg Execution"
          />
          <CapabilityRow
            description="The plugin runs in a sandboxed environment with limited system access"
            icon={Shield}
            label="Sandboxed Execution"
          />
        </div>

        {latestVersion && (
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <span>Version: {latestVersion.version}</span>
            <Badge variant="outline">
              Manifest v{latestVersion.manifest_version}
            </Badge>
          </div>
        )}

        <DialogFooter>
          <Button
            disabled={isPending}
            onClick={() => onOpenChange(false)}
            variant="outline"
          >
            Cancel
          </Button>
          <Button disabled={isPending} onClick={onConfirm}>
            {isPending ? "Installing..." : "Install Plugin"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
