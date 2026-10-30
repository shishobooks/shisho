import { useCallback, useEffect, useRef } from "react";
import { useSearchParams } from "react-router-dom";

/**
 * Returns a function that edits the URL's search params in place.
 *
 * react-router hands an updater the params the page last rendered, so two
 * clicks before the first navigation renders (filter toggles clicked fast)
 * would each build on the same old URL and the second would drop the first.
 * This builds on the params it last asked for until the page catches up, so
 * every update lands. The update must read what it changes from the params
 * it's given, not from the rendered page.
 */
export function useUpdateSearchParams() {
  const [searchParams, setSearchParams] = useSearchParams();
  const pending = useRef<URLSearchParams | null>(null);
  const rendered = searchParams.toString();

  // Once the page renders the params last asked for, they're the base again.
  useEffect(() => {
    if (pending.current?.toString() === rendered) pending.current = null;
  }, [rendered]);

  return useCallback(
    (
      update: (params: URLSearchParams) => void,
      options?: { replace?: boolean },
    ) => {
      const params = new URLSearchParams(pending.current ?? rendered);
      update(params);
      pending.current = params;
      setSearchParams(params, options);
    },
    [rendered, setSearchParams],
  );
}
