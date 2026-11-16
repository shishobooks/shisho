import { useCallback, useMemo, useSyncExternalStore } from "react";

import {
  readerSettingsFrom,
  type AccountReaderSettings,
  type ReaderSettings,
} from "@/libraries/readerSettings";

// The ebook reader's display settings, kept in this browser's localStorage.
// Every reader tab follows a change made in another.

export const READER_SETTINGS_KEY = "shisho-reader-settings";

// The latest settings when the browser refused to store them (a private
// window, or full storage), so changes still apply for the visit.
let unstored: string | null = null;
const listeners = new Set<() => void>();

const read = () => {
  if (unstored !== null) return unstored;
  try {
    return localStorage.getItem(READER_SETTINGS_KEY);
  } catch {
    return null;
  }
};

const write = (value: string) => {
  try {
    localStorage.setItem(READER_SETTINGS_KEY, value);
    unstored = null;
  } catch {
    unstored = value;
  }
  listeners.forEach((listener) => listener());
};

const subscribe = (listener: () => void) => {
  const onStorage = (e: StorageEvent) => {
    if (e.key !== READER_SETTINGS_KEY && e.key !== null) return;
    // Another tab stored newer settings.
    unstored = null;
    listener();
  };
  listeners.add(listener);
  window.addEventListener("storage", onStorage);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", onStorage);
  };
};

// `account` is the account's old reader settings, which a browser that has
// stored nothing yet starts from.
export function useReaderSettings(account?: AccountReaderSettings) {
  const raw = useSyncExternalStore(subscribe, read, () => null);
  const size = account?.viewer_reflowable_font_size;
  const theme = account?.viewer_reflowable_theme;
  const flow = account?.viewer_reflowable_flow;
  const settingsFrom = useCallback(
    (stored: string | null) =>
      readerSettingsFrom(stored, {
        viewer_reflowable_font_size: size,
        viewer_reflowable_theme: theme,
        viewer_reflowable_flow: flow,
      }),
    [size, theme, flow],
  );
  const settings = useMemo(() => settingsFrom(raw), [settingsFrom, raw]);
  // Merges into what's stored now, not what this render saw: a caller may
  // hold the callback across an await while other settings change.
  const update = useCallback(
    (change: Partial<ReaderSettings>) =>
      write(JSON.stringify({ ...settingsFrom(read()), ...change })),
    [settingsFrom],
  );
  return [settings, update] as const;
}
