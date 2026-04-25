import type { GallerySize } from "@/types";

export const GALLERY_SIZES: readonly GallerySize[] = ["s", "m", "l", "xl"];

export const DEFAULT_GALLERY_SIZE: GallerySize = "m";

export const GALLERY_SIZE_LABELS: Record<GallerySize, string> = {
  s: "S",
  m: "M",
  l: "L",
  xl: "XL",
};

// sm: applies at >= 640px. Below sm:, BookItem uses w-[calc(50%-0.5rem)]
// (forced 2-col), so size has no visual effect on mobile.
export const COVER_WIDTH_CLASS: Record<GallerySize, string> = {
  s: "sm:w-24",
  m: "sm:w-32",
  l: "sm:w-44",
  xl: "sm:w-56",
};

// Counts are tuned to fill exactly 3 rows at the column count each size
// produces on a typical desktop gallery width (S=11, M=8, L=6, XL=5). Three
// rows everywhere keeps perceived density consistent across sizes and avoids
// half-empty trailing rows.
//
// Backend max list limit is 50. If you raise any of these to >50 the API
// will silently clip and pagination will break.
export const ITEMS_PER_PAGE_BY_SIZE: Record<GallerySize, number> = {
  s: 33,
  m: 24,
  l: 18,
  xl: 15,
};
