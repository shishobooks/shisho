import { useCallback, useEffect, useMemo, useRef, type ReactNode } from "react";
import {
  NavigationType,
  useLocation,
  useNavigationType,
} from "react-router-dom";

import { NavigationHistoryContext } from "./context";

interface Entry {
  key: string;
  pathname: string;
}

// Mirrors the browser's history stack for this session, so a page can tell
// where Back would take the reader before following it. The browser hides
// the URLs of other entries, and a reload starts the mirror over, so an
// entry from before the reload reads as unknown.
export const NavigationHistoryProvider = ({
  children,
}: {
  children: ReactNode;
}) => {
  const location = useLocation();
  const navigationType = useNavigationType();
  const entries = useRef<Entry[]>([]);
  const index = useRef(-1);

  useEffect(() => {
    const list = entries.current;
    if (list[index.current]?.key === location.key) return;
    const entry = { key: location.key, pathname: location.pathname };
    if (navigationType === NavigationType.Push) {
      list.splice(index.current + 1, list.length, entry);
      index.current += 1;
    } else if (
      navigationType === NavigationType.Replace &&
      index.current >= 0
    ) {
      list[index.current] = entry;
    } else {
      const found = list.findIndex((e) => e.key === location.key);
      if (found >= 0) {
        index.current = found;
      } else {
        entries.current = [entry];
        index.current = 0;
      }
    }
  }, [location.key, location.pathname, navigationType]);

  const currentPathname = useCallback(
    () => entries.current[index.current]?.pathname,
    [],
  );
  const previousPathnames = useCallback(
    () =>
      entries.current
        .slice(0, Math.max(index.current, 0))
        .reverse()
        .map((e) => e.pathname),
    [],
  );
  const value = useMemo(
    () => ({ currentPathname, previousPathnames }),
    [currentPathname, previousPathnames],
  );

  return (
    <NavigationHistoryContext.Provider value={value}>
      {children}
    </NavigationHistoryContext.Provider>
  );
};
