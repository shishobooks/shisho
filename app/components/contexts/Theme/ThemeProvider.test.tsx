import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useTheme } from "@/components/contexts/Theme/context";
import { Button } from "@/components/ui/button";

import ThemeProvider from "./ThemeProvider";

// A prefers-color-scheme query the test can flip, as the OS would.
let systemDark = false;
const changeListeners = new Set<() => void>();

const setSystemDark = (dark: boolean) => {
  systemDark = dark;
  act(() => changeListeners.forEach((listener) => listener()));
};

beforeEach(() => {
  systemDark = false;
  changeListeners.clear();
  localStorage.clear();
  document.documentElement.className = "";
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    writable: true,
    value: vi.fn((query: string) => ({
      get matches() {
        return query === "(prefers-color-scheme: dark)" && systemDark;
      },
      media: query,
      addEventListener: (_: string, listener: () => void) =>
        changeListeners.add(listener),
      removeEventListener: (_: string, listener: () => void) =>
        changeListeners.delete(listener),
    })),
  });
});

afterEach(() => {
  // jsdom has no matchMedia; tests that add one take it away again.
  delete (window as { matchMedia?: unknown }).matchMedia;
});

const Appearance = () => {
  const { theme, resolvedTheme, setTheme } = useTheme();
  return (
    <>
      <p>
        {theme} {resolvedTheme}
      </p>
      <Button onClick={() => setTheme("light")}>Light</Button>
    </>
  );
};

const renderProvider = () =>
  render(
    <ThemeProvider>
      <Appearance />
    </ThemeProvider>,
  );

describe("ThemeProvider", () => {
  it.each(["light", "dark"] as const)(
    "resolves a chosen %s theme to itself",
    (theme) => {
      localStorage.setItem("ui-theme", theme);
      systemDark = theme === "light";
      renderProvider();
      expect(screen.getByText(`${theme} ${theme}`)).toBeInTheDocument();
      expect(document.documentElement).toHaveClass(theme);
    },
  );

  it("resolves the system theme to the OS appearance and follows it", () => {
    localStorage.setItem("ui-theme", "system");
    renderProvider();
    expect(screen.getByText("system light")).toBeInTheDocument();
    expect(document.documentElement).toHaveClass("light");

    setSystemDark(true);
    expect(screen.getByText("system dark")).toBeInTheDocument();
    expect(document.documentElement).toHaveClass("dark");
    expect(document.documentElement).not.toHaveClass("light");
  });

  it("stops following the OS once a theme is chosen", () => {
    localStorage.setItem("ui-theme", "system");
    renderProvider();
    fireEvent.click(screen.getByRole("button", { name: "Light" }));

    setSystemDark(true);
    expect(screen.getByText("light light")).toBeInTheDocument();
    expect(document.documentElement).toHaveClass("light");
  });
});
