import { createContext } from "react";

export interface NavigationHistoryContextValue {
  /** Path of the current entry, once the provider has recorded it. */
  currentPathname: () => string | undefined;
  /**
   * Paths of the entries Back leads through, nearest first, as far back as
   * the provider saw them.
   */
  previousPathnames: () => string[];
}

export const NavigationHistoryContext =
  createContext<NavigationHistoryContextValue | null>(null);
