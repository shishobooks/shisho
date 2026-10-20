import { useCallback } from "react";
import { useSearchParams } from "react-router-dom";

import { resetPaging } from "@/libraries/pagination";

/**
 * Reads and writes the `search` URL param that list pages filter by.
 *
 * Starting or clearing a search adds a history entry, so back returns to the
 * page you were on. Refining a search replaces the entry, so back doesn't step
 * through every debounced prefix of the query.
 */
export function useSearchQueryParam() {
  const [searchParams, setSearchParams] = useSearchParams();
  const searchQuery = searchParams.get("search") ?? "";

  const setSearchQuery = useCallback(
    (value: string) => {
      if (value === searchQuery) return;
      setSearchParams(
        (prev) => {
          const newParams = new URLSearchParams(prev);
          if (value) {
            newParams.set("search", value);
          } else {
            newParams.delete("search");
          }
          // Galleries page with `start`, resource lists with `page`.
          resetPaging(newParams);
          return newParams;
        },
        { replace: Boolean(searchQuery) && Boolean(value) },
      );
    },
    [searchQuery, setSearchParams],
  );

  return { searchQuery, setSearchQuery };
}
