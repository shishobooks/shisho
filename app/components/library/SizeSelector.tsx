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
      // inline-flex keeps the segmented control at its intrinsic width
      // so clicking between sizes never resizes the buttons under the
      // user's cursor — even when the parent (e.g. the size popover)
      // grows or shrinks as the save-as-default card appears/disappears.
      //
      // overflow-hidden clips the active button's bg to the rounded
      // outer corners (otherwise the first/last selected button shows
      // square corners outside the rounded border).
      //
      // When the parent is itself a stretching flex container (e.g. the
      // User Settings Appearance card uses flex flex-col), the
      // align-items: stretch default fills the parent width naturally —
      // no className override needed for that case.
      className={cn(
        "inline-flex overflow-hidden rounded-md border bg-background",
        className,
      )}
      role="group"
    >
      {GALLERY_SIZES.map((size, index) => {
        const isActive = size === value;
        return (
          <Button
            aria-pressed={isActive}
            className={cn(
              "h-8 flex-1 rounded-none border-0 px-3 text-xs font-semibold",
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
