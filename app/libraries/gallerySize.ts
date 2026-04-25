import { GALLERY_SIZES } from "@/constants/gallerySize";
import type { GallerySize } from "@/types";

export const parseGallerySize = (raw: string | null): GallerySize | null => {
  if (!raw) return null;
  return (GALLERY_SIZES as readonly string[]).includes(raw)
    ? (raw as GallerySize)
    : null;
};

export const pageForSizeChange = (
  oldOffset: number,
  newLimit: number,
): number => {
  return Math.floor(oldOffset / newLimit) + 1;
};
