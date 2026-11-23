import { createContext, useContext } from "react";

export type Theme = "dark" | "light" | "system";

// What the app is drawn in: the chosen theme, or for "system" the OS's
// appearance at the moment.
export type ResolvedTheme = Exclude<Theme, "system">;

export interface ThemeContextState {
  theme: Theme;
  resolvedTheme: ResolvedTheme;
  setTheme: (theme: Theme) => void;
}

export const ThemeContext = createContext<ThemeContextState>({
  theme: "dark",
  resolvedTheme: "dark",
  setTheme: () => {},
});

export const useTheme = () => {
  return useContext(ThemeContext);
};
