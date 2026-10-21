import {
  RESOURCE_ENTRY_GAP_PX,
  RESOURCE_ENTRY_MIN_WIDTH_PX,
  RESOURCE_PAGE_TARGET,
} from "@/constants/resourceList";

/** How many resource-list entries fit side by side in `width` CSS pixels. */
export const resourceListColumns = (width: number, phone: boolean): number => {
  if (phone) return 1;
  const column = RESOURCE_ENTRY_MIN_WIDTH_PX + RESOURCE_ENTRY_GAP_PX;
  return Math.max(1, Math.floor((width + RESOURCE_ENTRY_GAP_PX) / column));
};

/** Entries per resource-list page: whole rows, close to fifty. */
export const resourceListPageSize = (columns: number): number =>
  columns * Math.max(1, Math.floor(RESOURCE_PAGE_TARGET / columns));
