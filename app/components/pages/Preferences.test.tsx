import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useNavLibraries } from "@/hooks/queries/libraries";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { auth, setAuth } from "@/testing/auth";
import { rejectingMutate, REJECTION_MESSAGE } from "@/testing/mutations";

import Preferences from "./Preferences";

vi.mock("@/hooks/queries/settings", () => ({
  useUserSettings: vi.fn(),
  useUpdateUserSettings: vi.fn(),
}));

// DeliveryRulesEditor.test.tsx covers the editor; here it only needs data.
vi.mock("@/hooks/queries/delivery", () => ({
  useDeliveryRules: () => ({
    data: {
      follows_built_in: true,
      rules: [{ source_file_type: "epub", delivered_file_type: "epub" }],
      options: [{ source_file_type: "epub", delivered_file_types: ["epub"] }],
    },
  }),
  useUpdateDeliveryRules: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useResetDeliveryRules: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

const resetPassword = vi.hoisted(() => vi.fn());
vi.mock("@/hooks/queries/users", () => ({
  useResetPassword: () => ({ mutateAsync: resetPassword, isPending: false }),
}));

vi.mock("@/hooks/queries/libraries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/queries/libraries")>()),
  useNavLibraries: vi.fn(),
}));

beforeEach(() => {
  vi.mocked(useNavLibraries).mockReturnValue({ data: [] } as never);
});

// TopNav (rendered inside Preferences) calls useAuth and useMobileNav; mock
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

// Preferences uses useTheme from the Theme context; the default context value
// (theme: "dark", setTheme: noop) is sufficient for these tests — no provider
// needed.

const renderPage = () => {
  const client = new QueryClient();
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider
        router={createMemoryRouter([{ path: "/", element: <Preferences /> }])}
      />
    </QueryClientProvider>,
  );
};

describe("Preferences – gallery size section", () => {
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

describe("Preferences – theme section", () => {
  it("marks the current theme as pressed within the Theme group", () => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: { gallery_size: "m" },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({
      mutate: vi.fn(),
    } as never);

    renderPage();

    const group = screen.getByRole("group", { name: "Theme" });
    // The default Theme context value is "dark".
    expect(within(group).getByRole("button", { name: "Dark" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(
      within(group).getByRole("button", { name: "Light" }),
    ).toHaveAttribute("aria-pressed", "false");
    expect(
      within(group).getByRole("button", { name: "System" }),
    ).toHaveAttribute("aria-pressed", "false");
  });
});

describe("Preferences – reader settings", () => {
  it("toasts when saving the auto-hide setting fails", async () => {
    const error = vi.spyOn(toast, "error");
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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

describe("Preferences – default library", () => {
  const twoLibraries = [
    { id: 4, name: "Fiction" },
    { id: 7, name: "Comics" },
  ];

  it("hides the picker when the user has fewer than two libraries", () => {
    vi.mocked(useNavLibraries).mockReturnValue({
      data: [{ id: 4, name: "Fiction" }],
    } as never);
    vi.mocked(useUserSettings).mockReturnValue({
      data: { gallery_size: "m", default_library_id: null },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({
      mutate: vi.fn(),
    } as never);

    renderPage();

    expect(
      screen.queryByRole("combobox", { name: /default library/i }),
    ).not.toBeInTheDocument();
  });

  it("shows the saved default library", () => {
    vi.mocked(useNavLibraries).mockReturnValue({ data: twoLibraries } as never);
    vi.mocked(useUserSettings).mockReturnValue({
      data: { gallery_size: "m", default_library_id: 7 },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({
      mutate: vi.fn(),
    } as never);

    renderPage();

    expect(
      screen.getByRole("combobox", { name: /default library/i }),
    ).toHaveTextContent("Comics");
  });

  it("shows First library when the saved default is no longer accessible", () => {
    vi.mocked(useNavLibraries).mockReturnValue({ data: twoLibraries } as never);
    vi.mocked(useUserSettings).mockReturnValue({
      data: { gallery_size: "m", default_library_id: 99 },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({
      mutate: vi.fn(),
    } as never);

    renderPage();

    expect(
      screen.getByRole("combobox", { name: /default library/i }),
    ).toHaveTextContent("First library");
  });

  it("shows an error when the libraries fail to load", () => {
    vi.mocked(useNavLibraries).mockReturnValue({
      data: undefined,
      error: new Error("boom"),
      isError: true,
      refetch: vi.fn(),
    } as never);
    vi.mocked(useUserSettings).mockReturnValue({
      data: { gallery_size: "m", default_library_id: null },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({
      mutate: vi.fn(),
    } as never);

    renderPage();

    expect(screen.getByText(/failed to load libraries/i)).toBeInTheDocument();
  });

  it("saves the chosen library", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const mutate = vi.fn();
    vi.mocked(useNavLibraries).mockReturnValue({ data: twoLibraries } as never);
    vi.mocked(useUserSettings).mockReturnValue({
      data: { gallery_size: "m", default_library_id: null },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({ mutate } as never);

    renderPage();

    const picker = screen.getByRole("combobox", { name: /default library/i });
    expect(picker).toHaveTextContent("First library");

    await user.click(picker);
    await user.click(await screen.findByRole("option", { name: "Comics" }));
    expect(mutate).toHaveBeenLastCalledWith(
      { default_library_id: 7 },
      expect.objectContaining({ onError: expect.any(Function) }),
    );
  });

  it("clears the default when First library is chosen", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const mutate = vi.fn();
    vi.mocked(useNavLibraries).mockReturnValue({ data: twoLibraries } as never);
    vi.mocked(useUserSettings).mockReturnValue({
      data: { gallery_size: "m", default_library_id: 7 },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({ mutate } as never);

    renderPage();

    await user.click(
      screen.getByRole("combobox", { name: /default library/i }),
    );
    await user.click(
      await screen.findByRole("option", { name: "First library" }),
    );
    expect(mutate).toHaveBeenLastCalledWith(
      { default_library_id: null },
      expect.objectContaining({ onError: expect.any(Function) }),
    );
  });
});

describe("Preferences Delivery Rules", () => {
  beforeEach(() => {
    vi.mocked(useUserSettings).mockReturnValue({ data: undefined } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({
      mutate: vi.fn(),
    } as never);
  });

  it("shows the default Delivery Rules editor", () => {
    renderPage();

    expect(
      screen.getByRole("heading", { name: "Delivery Rules" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Following Shisho's defaults")).toBeInTheDocument();
  });
});

describe("Preferences – Delivery Rules link target", () => {
  it("scrolls to the Delivery Rules section once the page has loaded", async () => {
    const scrolled: Element[] = [];
    vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(
      function (this: HTMLElement) {
        scrolled.push(this);
      },
    );
    const settings = { data: undefined as unknown, isLoading: true };
    vi.mocked(useUserSettings).mockImplementation(() => settings as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({
      mutate: vi.fn(),
    } as never);

    const client = new QueryClient();
    const router = createMemoryRouter(
      [{ path: "/user/preferences", element: <Preferences /> }],
      { initialEntries: ["/user/preferences#delivery-rules"] },
    );
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );
    expect(scrolled).toEqual([]);

    // The settings arrive; a navigation re-renders the page with them.
    settings.data = { gallery_size: "m" };
    settings.isLoading = false;
    await act(() =>
      router.navigate("/user/preferences#delivery-rules", {
        replace: true,
        state: { loaded: true },
      }),
    );

    expect(scrolled.map((el) => el.id)).toEqual(["delivery-rules"]);
  });
});

describe("Preferences – Password", () => {
  beforeEach(() => {
    vi.mocked(useUserSettings).mockReturnValue({ data: undefined } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({
      mutate: vi.fn(),
    } as never);
    resetPassword.mockReset();
    resetPassword.mockResolvedValue(undefined);
  });

  afterEach(() => setAuth({ user: null }));

  const fillAndSubmit = async (current?: string) => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    if (current !== undefined) {
      await user.type(screen.getByLabelText("Current Password"), current);
    }
    await user.type(screen.getByLabelText("New Password"), "new-password");
    await user.type(
      screen.getByLabelText("Confirm New Password"),
      "new-password",
    );
    await user.click(screen.getByRole("button", { name: "Change Password" }));
  };

  it("changes the password from the Password section", async () => {
    setAuth({ user: { id: 4 } });
    renderPage();

    expect(
      screen.getByRole("heading", { name: "Password" }),
    ).toBeInTheDocument();
    await fillAndSubmit("old-password");

    expect(resetPassword).toHaveBeenCalledWith({
      id: "4",
      payload: {
        current_password: "old-password",
        new_password: "new-password",
        require_password_reset: false,
      },
    });
  });

  it("hides the Password section in Demo Mode", () => {
    setAuth({ user: { id: 4 }, demoMode: true });
    renderPage();

    expect(screen.queryByRole("heading", { name: "Password" })).toBeNull();
    expect(screen.queryByLabelText("Current Password")).toBeNull();
  });

  const renderAtPath = (path: string) => {
    const client = new QueryClient();
    const router = createMemoryRouter(
      [
        { path: "/user/preferences", element: <Preferences /> },
        { path: "/lists", element: null },
      ],
      { initialEntries: [path] },
    );
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );
    return router;
  };

  it("shows only the reset screen while a password reset is required", async () => {
    setAuth({ user: { id: 4, must_change_password: true } });
    // In the app, refetching auth clears the flag and Preferences swaps the
    // reset screen for the full page before the redirect runs.
    vi.mocked(auth.refetch).mockImplementation(async () => {
      auth.user = { ...auth.user!, must_change_password: false };
    });
    const router = renderAtPath("/user/preferences?redirect=%2Flists");

    expect(
      screen.getByRole("heading", { name: "Password Reset Required" }),
    ).toBeInTheDocument();
    expect(document.title).toBe("Change Password - Shisho");
    expect(screen.queryByRole("heading", { name: "Appearance" })).toBeNull();
    expect(screen.queryByLabelText("Current Password")).toBeNull();

    await fillAndSubmit();

    expect(resetPassword).toHaveBeenCalledWith({
      id: "4",
      payload: {
        current_password: undefined,
        new_password: "new-password",
        require_password_reset: false,
      },
    });
    await waitFor(() => expect(router.state.location.pathname).toBe("/lists"));
  });

  it("stays on Preferences after a voluntary change, without asking to discard", async () => {
    setAuth({ user: { id: 4 } });
    const router = renderAtPath("/user/preferences?redirect=%2Flists");

    await fillAndSubmit("old-password");
    await act(() => vi.runOnlyPendingTimersAsync());

    expect(resetPassword).toHaveBeenCalled();
    expect(router.state.location.pathname).toBe("/user/preferences");
    expect(screen.queryByText("Unsaved Changes")).toBeNull();
  });

  it("asks before leaving with a new password typed but not saved", async () => {
    setAuth({ user: { id: 4 } });
    const router = renderAtPath("/user/preferences");
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    await user.type(screen.getByLabelText("New Password"), "half-typed");
    await act(() => router.navigate("/lists"));

    expect(await screen.findByText("Unsaved Changes")).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/user/preferences");
  });

  it("does not count an autofilled current password as unsaved", async () => {
    setAuth({ user: { id: 4 } });
    const router = renderAtPath("/user/preferences");
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    await user.type(screen.getByLabelText("Current Password"), "saved");
    await act(() => router.navigate("/lists"));

    expect(router.state.location.pathname).toBe("/lists");
  });
});

describe("Preferences – Password link target", () => {
  it("scrolls to the Password section once the page has loaded", async () => {
    const scrolled: Element[] = [];
    vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(
      function (this: HTMLElement) {
        scrolled.push(this);
      },
    );
    setAuth({ user: { id: 4 } });
    vi.mocked(useUserSettings).mockReturnValue({
      data: { gallery_size: "m" },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({
      mutate: vi.fn(),
    } as never);

    const client = new QueryClient();
    const router = createMemoryRouter(
      [{ path: "/user/preferences", element: <Preferences /> }],
      { initialEntries: ["/user/preferences#password"] },
    );
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );

    await waitFor(() =>
      expect(scrolled.map((el) => el.id)).toEqual(["password"]),
    );
    setAuth({ user: null });
  });
});
