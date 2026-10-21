import { useSearchParams } from "react-router-dom";

import {
  DEFAULT_GALLERY_SIZE,
  LEGACY_ITEMS_PER_PAGE_BY_SIZE,
} from "@/constants/gallerySize";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { useStartPaging } from "@/hooks/useStartPaging";
import { toastRequestError } from "@/libraries/api";
import {
  galleryColumns,
  galleryPageSize,
  parseGallerySize,
} from "@/libraries/gallerySize";
import type { GallerySize } from "@/types";

interface Options {
  /** Called with the new size after the size changes. */
  onChange?: (size: GallerySize) => void;
}

/**
 * The layout and paging of a cover gallery. The size comes from the `size`
 * URL param with the user's saved size as the default. A page is whole rows
 * of the columns the content width fits (see `ContentWidth`), so the page
 * size changes with the window. The URL names a page by the index of an item
 * on it (`start`), not a page number, so a link opens on the same books on
 * any screen and the item stays in view when the page size changes.
 *
 * `size` is omitted when it equals the saved size, and `start` on the first
 * page, so a default gallery keeps a clean URL.
 */
export const useGallerySizeParam = ({ onChange }: Options = {}) => {
  const [searchParams, setSearchParams] = useSearchParams();
  const userSettingsQuery = useUserSettings();
  const updateUserSettings = useUpdateUserSettings();

  const urlSize = parseGallerySize(searchParams.get("size"));
  const savedSize: GallerySize =
    userSettingsQuery.data?.gallery_size ?? DEFAULT_GALLERY_SIZE;
  const effectiveSize: GallerySize = urlSize ?? savedSize;
  const isSizeDirty = urlSize !== null && urlSize !== savedSize;

  const { columns, pageSize, offset, currentPage, measured, goToPage } =
    useStartPaging({
      columnsFor: (width, phone) =>
        galleryColumns(effectiveSize, width, { phone }),
      pageSizeFor: (columns, phone) => galleryPageSize(columns, { phone }),
      legacyPageSize: LEGACY_ITEMS_PER_PAGE_BY_SIZE[effectiveSize],
    });
  const itemsPerPage = pageSize;
  // The saved size and the content width decide the page size, so a gallery
  // waits for both before it fetches, or it would fetch twice.
  const pageSizeResolved =
    measured && (userSettingsQuery.isSuccess || userSettingsQuery.isError);

  const applyGallerySize = (next: GallerySize) => {
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      if (next === savedSize) {
        params.delete("size");
      } else {
        params.set("size", next);
      }
      return params;
    });
    onChange?.(next);
  };

  const saveSizeAsDefault = () => {
    updateUserSettings.mutate(
      { gallery_size: effectiveSize },
      {
        onError: (error) =>
          toastRequestError(error, "Failed to save the default size"),
        onSuccess: () => {
          setSearchParams((prev) => {
            const params = new URLSearchParams(prev);
            params.delete("size");
            return params;
          });
        },
      },
    );
  };

  return {
    savedSize,
    effectiveSize,
    isSizeDirty,
    columns,
    itemsPerPage,
    currentPage,
    offset,
    pageSizeResolved,
    isSaving: updateUserSettings.isPending,
    goToPage,
    applyGallerySize,
    saveSizeAsDefault,
  };
};
