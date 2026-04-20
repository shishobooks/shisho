import { Braces, RotateCw } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useReloadPlugin } from "@/hooks/queries/plugins";
import type { Plugin } from "@/types/generated/models";

import { PluginManifestDialog } from "./PluginManifestDialog";

export interface PluginHeroActionsProps {
  canWrite: boolean;
  plugin: Plugin;
}

export const PluginHeroActions = ({
  canWrite,
  plugin,
}: PluginHeroActionsProps) => {
  const [manifestOpen, setManifestOpen] = useState(false);
  const reload = useReloadPlugin();

  const handleReload = async () => {
    try {
      await reload.mutateAsync({ id: plugin.id, scope: plugin.scope });
      toast.success(`${plugin.name} reloaded from disk`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Reload failed");
    }
  };

  return (
    <>
      <div className="flex items-center gap-1">
        {canWrite && plugin.scope === "local" && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                disabled={reload.isPending}
                onClick={handleReload}
                size="icon"
                variant="outline"
              >
                <RotateCw aria-hidden="true" className="h-4 w-4" />
                <span className="sr-only">Reload plugin from disk</span>
              </Button>
            </TooltipTrigger>
            <TooltipContent>Reload plugin from disk</TooltipContent>
          </Tooltip>
        )}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              onClick={() => setManifestOpen(true)}
              size="icon"
              variant="outline"
            >
              <Braces aria-hidden="true" className="h-4 w-4" />
              <span className="sr-only">View manifest</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>View manifest</TooltipContent>
        </Tooltip>
      </div>
      <PluginManifestDialog
        id={plugin.id}
        name={plugin.name}
        onOpenChange={setManifestOpen}
        open={manifestOpen}
        scope={plugin.scope}
      />
    </>
  );
};
