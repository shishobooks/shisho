import type { GallerySize } from "@/types";

export const GALLERY_SIZES: readonly GallerySize[] = ["s", "m", "l", "xl"];

export const DEFAULT_GALLERY_SIZE: GallerySize = "m";

export const GALLERY_SIZE_LABELS: Record<GallerySize, string> = {
  s: "S",
  m: "M",
  l: "L",
  xl: "XL",
};

// The narrowest a cover may get at each size, in CSS pixels. A gallery fits
// as many columns as the width holds and stretches them to fill the row, so
// covers render at least this wide. Phones always show two columns.
export const COVER_MIN_WIDTH_PX: Record<GallerySize, number> = {
  s: 96,
  m: 128,
  l: 176,
  xl: 224,
};

// Matches the grid's gap-4.
export const GALLERY_GAP_PX = 16;

// A page is this many full rows, so it never ends on a partial row.
export const GALLERY_ROWS = 3;
export const GALLERY_PHONE_ROWS = 12;

// The most items a list endpoint returns per request (`max=100` on the Go
// list payloads). A page drops rows rather than exceed it.
export const GALLERY_MAX_PAGE_SIZE = 100;

// Page sizes before galleries filled the window, when a page was three rows
// of a fixed 1280px column. An old `?page=` link is read with these, so it
// opens on the books it named.
export const LEGACY_ITEMS_PER_PAGE_BY_SIZE: Record<GallerySize, number> = {
  s: 33,
  m: 24,
  l: 18,
  xl: 15,
};
