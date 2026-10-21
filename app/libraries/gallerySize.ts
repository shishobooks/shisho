import {
  COVER_MIN_WIDTH_PX,
  GALLERY_GAP_PX,
  GALLERY_MAX_PAGE_SIZE,
  GALLERY_PHONE_ROWS,
  GALLERY_ROWS,
  GALLERY_SIZES,
} from "@/constants/gallerySize";
import type { GallerySize } from "@/types";

export const parseGallerySize = (raw: string | null): GallerySize | null => {
  if (!raw) return null;
  return (GALLERY_SIZES as readonly string[]).includes(raw)
    ? (raw as GallerySize)
    : null;
};

interface LayoutOptions {
  /** Below the sm breakpoint, where the gallery is two columns. */
  phone?: boolean;
}

/** How many cover columns a gallery of `width` CSS pixels fits. */
export const galleryColumns = (
  size: GallerySize,
  width: number,
  { phone = false }: LayoutOptions = {},
): number => {
  if (phone) return 2;
  const column = COVER_MIN_WIDTH_PX[size] + GALLERY_GAP_PX;
  return Math.max(1, Math.floor((width + GALLERY_GAP_PX) / column));
};

/** Items per page: whole rows of `columns`, within the API's limit. */
export const galleryPageSize = (
  columns: number,
  { phone = false }: LayoutOptions = {},
): number => {
  const rows = phone ? GALLERY_PHONE_ROWS : GALLERY_ROWS;
  const fittingRows = Math.floor(GALLERY_MAX_PAGE_SIZE / columns);
  // Wider than the limit only on absurd widths; one capped row then.
  return Math.min(
    columns * Math.max(1, Math.min(rows, fittingRows)),
    GALLERY_MAX_PAGE_SIZE,
  );
};
