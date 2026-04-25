import { Maximize2, Save } from "lucide-react";
import { useState } from "react";

import { SizeSelector } from "@/components/library/SizeSelector";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import type { GallerySize } from "@/types";

interface SizePopoverProps {
  effectiveSize: GallerySize;
  savedSize: GallerySize;
  isSaving: boolean;
  onChange: (size: GallerySize) => void;
  onSaveAsDefault: () => void;
  trigger: React.ReactNode;
}

export const SizePopover = ({
  effectiveSize,
  savedSize,
  isSaving,
  onChange,
  onSaveAsDefault,
  trigger,
}: SizePopoverProps) => {
  const [open, setOpen] = useState(false);
  const isDirty = effectiveSize !== savedSize;

  return (
    <Popover onOpenChange={setOpen} open={open}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent align="start" className="w-auto p-3">
        <div className="flex flex-col gap-3">
          <SizeSelector onChange={onChange} value={effectiveSize} />
          {isDirty && (
            <div className="border border-dashed rounded-md p-3">
              <p className="text-xs text-muted-foreground mb-2">
                Other users won't be affected.
              </p>
              <Button disabled={isSaving} onClick={onSaveAsDefault} size="sm">
                <Save className="h-4 w-4" />
                {isSaving ? "Saving…" : "Save as my default everywhere"}
              </Button>
            </div>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
};

export const SizeButton = ({
  isDirty,
  onClick,
}: {
  isDirty: boolean;
  onClick?: () => void;
}) => (
  <Button className="relative" onClick={onClick} size="sm" variant="outline">
    <Maximize2 className="h-4 w-4" />
    Size
    {isDirty && (
      <span
        aria-label="Size differs from default"
        className="absolute top-1 right-1 h-2 w-2 rounded-full bg-primary ring-2 ring-background"
      />
    )}
  </Button>
);
