import { useCallback, useEffect, useRef } from "react";
import { useSearchParams } from "react-router-dom";

/**
 * Returns a function that edits the URL's search params in place, keeping
 * the scroll position as a filter or sort change should.
 *
 * react-router hands an updater the params the page last rendered, so two
 * clicks before the first navigation renders (filter toggles clicked fast)
 * would each build on the same old URL and the second would drop the first.
 * This builds on its own latest write while the page still shows the URL it
 * started from or one of its writes, and on the latest rendered URL
 * otherwise, so a search, sort, or Back made elsewhere is never undone, even
 * by an update called later from an earlier render (a save that finishes).
 * The update must read what it changes from the params it's given, not from
 * the rendered page. An update that changes nothing doesn't navigate.
 */
export function useUpdateSearchParams() {
  const [searchParams, setSearchParams] = useSearchParams();
  const rendered = searchParams.toString();
  const latestRendered = useRef(rendered);
  // The URL the current run of writes started from, and each write since.
  const run = useRef<{ from: string; writes: string[] } | null>(null);

  useEffect(() => {
    latestRendered.current = rendered;
    // Once the latest write renders, the page has caught up.
    const writes = run.current?.writes;
    if (writes && writes[writes.length - 1] === rendered) run.current = null;
  }, [rendered]);

  return useCallback(
    (
      update: (params: URLSearchParams) => void,
      options?: { replace?: boolean },
    ) => {
      const shown = latestRendered.current;
      const current = run.current;
      const ours =
        current !== null &&
        (shown === current.from || current.writes.includes(shown));
      const base = ours ? current.writes[current.writes.length - 1] : shown;
      const params = new URLSearchParams(base);
      update(params);
      const written = params.toString();
      if (written === base) return;
      run.current = ours
        ? { from: current.from, writes: [...current.writes, written] }
        : { from: shown, writes: [written] };
      setSearchParams(params, { preventScrollReset: true, ...options });
    },
    [setSearchParams],
  );
}
