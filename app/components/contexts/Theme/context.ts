import { createContext, useContext } from "react";

export type Theme = "dark" | "light" | "system";

export interface ThemeContextState {
  theme: Theme;
  setTheme: (theme: Theme) => void;
}

export const ThemeContext = createContext<ThemeContextState>({
  theme: "dark",
  setTheme: () => {},
});

export const useTheme = () => {
  return useContext(ThemeContext);
};
