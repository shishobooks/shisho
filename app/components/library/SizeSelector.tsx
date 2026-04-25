import { Button } from "@/components/ui/button";
import { GALLERY_SIZE_LABELS, GALLERY_SIZES } from "@/constants/gallerySize";
import { cn } from "@/libraries/utils";
import type { GallerySize } from "@/types";

interface SizeSelectorProps {
  value: GallerySize;
  onChange: (size: GallerySize) => void;
  className?: string;
}

export const SizeSelector = ({
  value,
  onChange,
  className,
}: SizeSelectorProps) => {
  return (
    <div
      className={cn("inline-flex rounded-md border bg-background", className)}
      role="group"
    >
      {GALLERY_SIZES.map((size, index) => {
        const isActive = size === value;
        return (
          <Button
            aria-pressed={isActive}
            className={cn(
              "h-8 rounded-none px-3 text-xs font-semibold border-0",
              index > 0 && "border-l",
              isActive
                ? "bg-primary text-primary-foreground hover:bg-primary"
                : "bg-transparent",
            )}
            key={size}
            onClick={() => onChange(size)}
            type="button"
            variant="ghost"
          >
            {GALLERY_SIZE_LABELS[size]}
          </Button>
        );
      })}
    </div>
  );
};
