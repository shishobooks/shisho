import { ReactNode, useEffect, useMemo, useState } from "react";

import { Theme, ThemeContext } from "@/components/contexts/Theme/context";

interface Props {
  children: ReactNode;
  defaultTheme?: Theme;
  storageKey?: string;
}

const ThemeProvider = ({
  children,
  defaultTheme = "system",
  storageKey = "ui-theme",
}: Props) => {
  const [theme, setTheme] = useState<Theme>(
    () => (localStorage.getItem(storageKey) as Theme) || defaultTheme,
  );

  useEffect(() => {
    const root = window.document.documentElement;

    root.classList.remove("light", "dark");

    if (theme === "system") {
      const systemTheme = window.matchMedia("(prefers-color-scheme: dark)")
        .matches
        ? "dark"
        : "light";

      root.classList.add(systemTheme);
      return;
    }

    root.classList.add(theme);
  }, [theme]);

  const value = useMemo(
    () => ({
      theme,
      setTheme: (theme: Theme) => {
        localStorage.setItem(storageKey, theme);
        setTheme(theme);
      },
    }),
    [storageKey, theme, setTheme],
  );

  return (
    <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
  );
};

export default ThemeProvider;
