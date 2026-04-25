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
      // overflow-hidden clips the active button's bg to the parent's rounded
      // corners (otherwise the first/last selected button shows square corners
      // outside the rounded border).
      //
      // Default to inline-flex (intrinsic width) so the selector stays
      // compact on the User Settings page. The popover passes
      // className="flex w-full" to stretch buttons across the full popover
      // width when the save-as-default card forces the popover wider than
      // the buttons' natural size.
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
            // flex-1 only takes effect when the parent is a full-width
            // flex container (popover case). In intrinsic-width mode it's
            // harmless — there's no extra space to distribute.
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
