import { useCallback } from "react";
import { useSearchParams } from "react-router-dom";

import { resetPaging } from "@/libraries/pagination";
import {
  parseSortSpecFor,
  serializeSortSpec,
  sortSpecsEqual,
  type SortConfig,
  type SortLevel,
} from "@/libraries/sortSpec";

/**
 * Hook that keeps a view's sort in the URL `?sort=` param. The param is
 * omitted when the sort equals the view's default, and a sort with a field
 * the view does not offer is ignored.
 */
export function useSortParam<F extends string>(config: SortConfig<F>) {
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
        // Galleries page with `start`, resource lists with `page`.
        resetPaging(params);
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
    /**
     * The effective sort to send to the API, or undefined when the default is
     * the endpoint's own order.
     */
    param: serializeSortSpec(levels) || undefined,
    apply,
    reset,
  };
}

export type SortParam<F extends string> = ReturnType<typeof useSortParam<F>>;
