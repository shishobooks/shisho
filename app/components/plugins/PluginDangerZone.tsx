import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { useUninstallPlugin } from "@/hooks/queries/plugins";
import type { Plugin } from "@/types/generated/models";

export interface PluginDangerZoneProps {
  plugin: Plugin;
}

export const PluginDangerZone = ({ plugin }: PluginDangerZoneProps) => {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const navigate = useNavigate();
  const uninstall = useUninstallPlugin();

  const handleUninstall = () => {
    uninstall.mutate(
      { id: plugin.id, scope: plugin.scope },
      {
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : "Uninstall failed");
        },
        onSuccess: () => {
          toast.success(`${plugin.name} uninstalled`);
          setConfirmOpen(false);
          navigate("/settings/plugins");
        },
      },
    );
  };

  return (
    <section className="space-y-3">
      <h2 className="text-lg font-semibold text-destructive">Danger zone</h2>
      <div className="flex items-center justify-between gap-4 rounded-md border border-destructive/40 p-4">
        <div>
          <div className="text-sm font-medium">Uninstall plugin</div>
          <div className="text-xs text-muted-foreground">
            Removes the plugin and its files. Plugin configuration will be
            discarded.
          </div>
        </div>
        <Button onClick={() => setConfirmOpen(true)} variant="destructive">
          Uninstall
        </Button>
      </div>
      <ConfirmDialog
        confirmLabel="Uninstall"
        description={`Are you sure you want to uninstall "${plugin.name}"? This cannot be undone.`}
        isPending={uninstall.isPending}
        onConfirm={handleUninstall}
        onOpenChange={setConfirmOpen}
        open={confirmOpen}
        title="Uninstall plugin"
        variant="destructive"
      />
    </section>
  );
};
