import type { ReactNode } from "react";

import { cn } from "@/libraries/utils";

interface GalleryGridProps {
  /** From `useGallerySizeParam`, so the grid and the page size agree. */
  columns: number;
  children: ReactNode;
  className?: string;
}

// Cover cards in equal columns that span the content width edge to edge.
// The column count comes from the hook rather than CSS auto-fill, because the
// page size is whole rows of it.
const GalleryGrid = ({ columns, children, className }: GalleryGridProps) => (
  <div
    className={cn("mb-6 grid gap-4 md:mb-8", className)}
    style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}
  >
    {children}
  </div>
);

export default GalleryGrid;
