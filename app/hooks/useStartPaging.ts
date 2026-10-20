import { useLayoutEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";

import type { PageChangeHandler } from "@/components/library/PaginationFooter";
import { useContentWidth } from "@/contexts/ContentWidth";
import { useIsPhone } from "@/hooks/useIsPhone";
import {
  parsePageParam,
  parseStartParam,
  startOffset,
} from "@/libraries/pagination";

interface Options {
  /** The columns `width` CSS pixels of content fit. */
  columnsFor: (width: number, phone: boolean) => number;
  /** Items per page for that many columns: whole rows of them. */
  pageSizeFor: (columns: number, phone: boolean) => number;
  /**
   * The page size of an old `?page=` link, from before pages were sized to
   * the window, so the link opens on the items it named.
   */
  legacyPageSize: number;
}

/**
 * Paging for a list laid out in columns that fill the content width (see
 * `ContentWidth`). A page is whole rows, so its size changes with the window,
 * and the URL names a page by the index of its first item (`start`) rather
 * than a page number: a link opens on the same items on any screen, and the
 * item stays in view when the page size changes. `start` is omitted on the
 * first page.
 */
export const useStartPaging = ({
  columnsFor,
  pageSizeFor,
  legacyPageSize,
}: Options) => {
  const [searchParams, setSearchParams] = useSearchParams();
  const phone = useIsPhone();

  // null until this page's content has been measured.
  const measuredColumns = useContentWidth((width) =>
    width === null ? null : columnsFor(width, phone),
  );
  // A page's first render can still see the width the previous page
  // measured, until its own ContentWidth reports in a layout effect. Layout
  // effects run child first, so once this one has run the width is current.
  const [mounted, setMounted] = useState(false);
  useLayoutEffect(() => setMounted(true), []);
  const measured = mounted && measuredColumns !== null;

  const columns = measuredColumns ?? 1;
  const pageSize = pageSizeFor(columns, phone);
  const legacyPage = searchParams.has("start")
    ? null
    : parsePageParam(searchParams.get("page"));
  const start = legacyPage
    ? (legacyPage - 1) * legacyPageSize
    : parseStartParam(searchParams.get("start"));
  const offset = startOffset(start, pageSize);
  const currentPage = offset / pageSize + 1;

  const goToPage: PageChangeHandler = (page, options) => {
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      params.delete("page");
      if (page <= 1) {
        params.delete("start");
      } else {
        params.set("start", String((page - 1) * pageSize));
      }
      return params;
    }, options);
  };

  return { columns, pageSize, offset, currentPage, measured, goToPage };
};
