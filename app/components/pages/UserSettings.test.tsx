import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";

import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { setAuth } from "@/testing/auth";
import { rejectingMutate, REJECTION_MESSAGE } from "@/testing/mutations";

import UserSettings from "./UserSettings";

vi.mock("@/hooks/queries/settings", () => ({
  useUserSettings: vi.fn(),
  useUpdateUserSettings: vi.fn(),
}));

// TopNav (rendered inside UserSettings) calls useAuth and useMobileNav; mock
// both so tests don't require a full AuthProvider / MobileNavProvider.
vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

setAuth({ user: null });

vi.mock("@/contexts/MobileNav/useMobileNav", () => ({
  useMobileNav: () => ({
    isOpen: false,
    open: vi.fn(),
    close: vi.fn(),
    toggle: vi.fn(),
  }),
}));

// UserSettings uses useTheme from the Theme context; the default context value
// (theme: "dark", setTheme: noop) is sufficient for these tests — no provider
// needed.

const renderPage = () => {
  const client = new QueryClient();
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <UserSettings />
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

describe("UserSettings – gallery size section", () => {
  it("renders four size buttons (S, M, L, XL)", () => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: { gallery_size: "m" },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({
      mutate: vi.fn(),
    } as never);

    renderPage();

    expect(screen.getByRole("button", { name: "S" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "M" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "L" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "XL" })).toBeInTheDocument();
  });

  it("active button reflects the saved size from useUserSettings", () => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: { gallery_size: "l" },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({
      mutate: vi.fn(),
    } as never);

    renderPage();

    expect(screen.getByRole("button", { name: "L" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "M" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });

  it("clicking a different size calls the update mutation with the right payload", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const mutate = vi.fn();

    vi.mocked(useUserSettings).mockReturnValue({
      data: { gallery_size: "m" },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({ mutate } as never);

    renderPage();

    await user.click(screen.getByRole("button", { name: "XL" }));

    expect(mutate).toHaveBeenCalledWith(
      { gallery_size: "xl" },
      expect.objectContaining({ onError: expect.any(Function) }),
    );
  });
});

describe("UserSettings – reader settings", () => {
  it("toasts when saving the auto-hide setting fails", async () => {
    const error = vi.spyOn(toast, "error");
    const user = userEvent.setup();
    vi.mocked(useUserSettings).mockReturnValue({
      data: { gallery_size: "m", viewer_hide_chrome: false },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({
      mutate: rejectingMutate(),
    } as never);

    renderPage();

    await user.click(screen.getByRole("switch", { name: /auto-hide/i }));

    expect(error).toHaveBeenCalledWith(REJECTION_MESSAGE, undefined);
  });
});
