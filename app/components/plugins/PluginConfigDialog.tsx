import { useState } from "react";

import { PluginConfigForm } from "@/components/plugins/PluginConfigForm";
import {
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormDialog } from "@/components/ui/form-dialog";

interface PluginConfigDialogProps {
  onOpenChange: (open: boolean) => void;
  open: boolean;
  pluginId: string;
  pluginName: string;
  scope: string;
}

export const PluginConfigDialog = ({
  onOpenChange,
  open,
  pluginId,
  pluginName,
  scope,
}: PluginConfigDialogProps) => {
  const [hasChanges, setHasChanges] = useState(false);

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent className="overflow-x-hidden">
        <DialogHeader className="pr-8">
          <DialogTitle>Configure {pluginName}</DialogTitle>
          <DialogDescription>
            Adjust plugin settings and configure which metadata fields it can
            modify.
          </DialogDescription>
        </DialogHeader>

        {open && pluginId && scope && (
          <PluginConfigForm
            id={pluginId}
            onDirtyChange={setHasChanges}
            onSaved={() => onOpenChange(false)}
            scope={scope}
          />
        )}
      </DialogContent>
    </FormDialog>
  );
};
