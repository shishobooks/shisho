import { useContext, useSyncExternalStore } from "react";

import { ContentWidthContext, FALLBACK_WIDTH } from "./context";

const noSubscribe = () => () => {};

/**
 * Reads what `select` derives from the content width (null until measured),
 * re-rendering only when that value changes. Return a primitive, such as a
 * column count, so resizing within a column does not re-render the page.
 */
export const useContentWidth = <T>(select: (width: number | null) => T): T => {
  const store = useContext(ContentWidthContext);
  return useSyncExternalStore(store?.subscribe ?? noSubscribe, () =>
    select(store ? store.get() : FALLBACK_WIDTH),
  );
};
