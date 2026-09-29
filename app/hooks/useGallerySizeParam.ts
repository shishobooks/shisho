import { useSearchParams } from "react-router-dom";

import {
  DEFAULT_GALLERY_SIZE,
  ITEMS_PER_PAGE_BY_SIZE,
} from "@/constants/gallerySize";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { toastRequestError } from "@/libraries/api";
import { pageForSizeChange, parseGallerySize } from "@/libraries/gallerySize";
import { parsePageParam } from "@/libraries/pagination";
import type { GallerySize } from "@/types";

interface Options {
  /** Called with the new size and page after the size changes. */
  onChange?: (size: GallerySize, page: number) => void;
}

/**
 * The gallery size of a paginated cover gallery, read from the `size` URL
 * param with the user's saved size as the default, plus the page math that
 * depends on it. `applyGallerySize` keeps the first item in view on screen
 * when the page size changes, and `saveSizeAsDefault` saves the current size
 * to the user's settings (toasting a failure) and drops the URL param.
 *
 * The `size` param is omitted when it equals the saved size, and the `page`
 * param when it is 1, so a default gallery keeps a clean URL.
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
  const itemsPerPage = ITEMS_PER_PAGE_BY_SIZE[effectiveSize];
  const currentPage = parsePageParam(searchParams.get("page"));
  const offset = (currentPage - 1) * itemsPerPage;
  // The saved size decides the page size, so a gallery waits for the settings
  // query to settle before it fetches, or it would fetch twice.
  const settingsResolved =
    userSettingsQuery.isSuccess || userSettingsQuery.isError;

  const applyGallerySize = (next: GallerySize) => {
    const newPage = pageForSizeChange(offset, ITEMS_PER_PAGE_BY_SIZE[next]);
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      if (next === savedSize) {
        params.delete("size");
      } else {
        params.set("size", next);
      }
      if (newPage === 1) {
        params.delete("page");
      } else {
        params.set("page", String(newPage));
      }
      return params;
    });
    onChange?.(next, newPage);
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
    itemsPerPage,
    currentPage,
    offset,
    settingsResolved,
    isSaving: updateUserSettings.isPending,
    applyGallerySize,
    saveSizeAsDefault,
  };
};
