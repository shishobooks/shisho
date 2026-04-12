import { Loader2, RefreshCw } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { type RescanMode } from "@/hooks/queries/resync";

interface RescanDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: "book" | "file";
  entityName: string;
  onConfirm: (mode: RescanMode) => void;
  isPending: boolean;
}

const modes: { value: RescanMode; label: string; description: string }[] = [
  {
    value: "scan",
    label: "Scan for new metadata",
    description:
      "Pick up new metadata without overwriting manual edits. Use when file metadata has been updated externally.",
  },
  {
    value: "refresh",
    label: "Refresh all metadata",
    description:
      "Re-scan as if this were the first time. Use after installing or updating plugins to re-enrich metadata.",
  },
  {
    value: "reset",
    label: "Reset to file metadata",
    description:
      "Clear all metadata and re-scan as if this were a brand new file, without plugins. Manual edits and enricher data will be removed.",
  },
];

export function RescanDialog({
  open,
  onOpenChange,
  entityType,
  entityName,
  onConfirm,
  isPending,
}: RescanDialogProps) {
  const [selectedMode, setSelectedMode] = useState<RescanMode>("scan");

  return (
    <Dialog
      onOpenChange={(newOpen) => {
        if (!newOpen) setSelectedMode("scan");
        onOpenChange(newOpen);
      }}
      open={open}
    >
      <DialogContent className="max-w-md">
        <DialogHeader className="pr-8">
          <DialogTitle className="flex items-center gap-2">
            <RefreshCw className="h-5 w-5 shrink-0" />
            <span>Rescan {entityType}</span>
          </DialogTitle>
          <DialogDescription className="break-words">
            Choose how to rescan &ldquo;{entityName}&rdquo;
          </DialogDescription>
        </DialogHeader>

        <RadioGroup
          className="gap-3"
          onValueChange={(value) => setSelectedMode(value as RescanMode)}
          value={selectedMode}
        >
          {modes.map((mode) => (
            <Label
              className="flex items-start gap-3 rounded-lg border p-3 cursor-pointer has-[[data-state=checked]]:border-primary"
              htmlFor={`rescan-${entityType}-${mode.value}`}
              key={mode.value}
            >
              <RadioGroupItem
                className="mt-0.5"
                id={`rescan-${entityType}-${mode.value}`}
                value={mode.value}
              />
              <div className="space-y-0.5">
                <div className="text-sm font-medium leading-none">
                  {mode.label}
                </div>
                <div className="text-xs text-muted-foreground font-normal">
                  {mode.description}
                </div>
              </div>
            </Label>
          ))}
        </RadioGroup>

        <DialogFooter>
          <Button
            disabled={isPending}
            onClick={() => onOpenChange(false)}
            variant="outline"
          >
            Cancel
          </Button>
          <Button
            disabled={isPending}
            onClick={() => {
              onOpenChange(false);
              onConfirm(selectedMode);
            }}
          >
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Rescan
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
