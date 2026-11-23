import {
  ReactNode,
  useEffect,
  useMemo,
  useState,
  useSyncExternalStore,
} from "react";

import {
  ResolvedTheme,
  Theme,
  ThemeContext,
} from "@/components/contexts/Theme/context";

interface Props {
  children: ReactNode;
  defaultTheme?: Theme;
  storageKey?: string;
}

const DARK_QUERY = "(prefers-color-scheme: dark)";

// The OS's appearance, followed as it changes, so a switch to dark mode
// redraws an app set to System (and the ebook reader's System theme) without
// a reload.
const systemAppearance = (): ResolvedTheme =>
  window.matchMedia?.(DARK_QUERY).matches ? "dark" : "light";

const subscribeToSystem = (onChange: () => void) => {
  const query = window.matchMedia?.(DARK_QUERY);
  query?.addEventListener("change", onChange);
  return () => query?.removeEventListener("change", onChange);
};

const ThemeProvider = ({
  children,
  defaultTheme = "dark",
  storageKey = "ui-theme",
}: Props) => {
  const [theme, setTheme] = useState<Theme>(
    () => (localStorage.getItem(storageKey) as Theme) || defaultTheme,
  );
  const system = useSyncExternalStore(subscribeToSystem, systemAppearance);
  const resolvedTheme = theme === "system" ? system : theme;

  useEffect(() => {
    const root = window.document.documentElement;
    root.classList.remove("light", "dark");
    root.classList.add(resolvedTheme);
  }, [resolvedTheme]);

  const value = useMemo(
    () => ({
      theme,
      resolvedTheme,
      setTheme: (theme: Theme) => {
        localStorage.setItem(storageKey, theme);
        setTheme(theme);
      },
    }),
    [storageKey, theme, resolvedTheme, setTheme],
  );

  return (
    <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
  );
};

export default ThemeProvider;
