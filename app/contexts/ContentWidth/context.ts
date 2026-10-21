import { createContext } from "react";

// The content width of the old centered 1280px column. It stands in where
// nothing is measured: outside a ContentWidthProvider, and in jsdom, which
// lays nothing out.
export const FALLBACK_WIDTH = 1232;

/**
 * The page's content width, null until a ContentWidth has measured it. A
 * store rather than React state, so a page re-renders only when what it
 * derives from the width changes, not on every pixel of a resize.
 */
export interface ContentWidthStore {
  get: () => number | null;
  set: (width: number | null) => void;
  subscribe: (listener: () => void) => () => void;
}

export const createContentWidthStore = (): ContentWidthStore => {
  let width: number | null = null;
  const listeners = new Set<() => void>();
  return {
    get: () => width,
    set: (next) => {
      if (next === width) return;
      width = next;
      listeners.forEach((listener) => listener());
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
};

export const ContentWidthContext = createContext<ContentWidthStore | undefined>(
  undefined,
);
