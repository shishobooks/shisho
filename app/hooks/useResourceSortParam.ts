import { useCallback } from "react";
import { useSearchParams } from "react-router-dom";

import type { ResourceSortConfig } from "@/libraries/resourceSort";
import {
  parseSortSpecFor,
  serializeSortSpec,
  sortSpecsEqual,
  type SortLevel,
} from "@/libraries/sortSpec";

/**
 * Hook that keeps a resource list's sort in the URL `?sort=` param. The param
 * is omitted when the sort equals the list's default, and a sort with a field
 * the list does not offer is ignored.
 */
export function useResourceSortParam<F extends string>(
  config: ResourceSortConfig<F>,
) {
  const [searchParams, setSearchParams] = useSearchParams();
  const urlSort = parseSortSpecFor(
    searchParams.get("sort") ?? "",
    config.fields,
  );
  const isDirty =
    urlSort !== null &&
    urlSort.length > 0 &&
    !sortSpecsEqual(urlSort, config.defaultSort);
  const levels: readonly SortLevel<F>[] = isDirty
    ? urlSort
    : config.defaultSort;

  const apply = useCallback(
    (next: readonly SortLevel<F>[]) => {
      setSearchParams((prev) => {
        const params = new URLSearchParams(prev);
        if (next.length > 0 && !sortSpecsEqual(next, config.defaultSort)) {
          params.set("sort", serializeSortSpec(next));
        } else {
          params.delete("sort");
        }
        params.set("page", "1");
        return params;
      });
    },
    [config.defaultSort, setSearchParams],
  );

  const reset = useCallback(() => apply([]), [apply]);

  return {
    config,
    levels,
    isDirty,
    /** The effective sort to send to the API. */
    param: serializeSortSpec(levels),
    apply,
    reset,
  };
}
