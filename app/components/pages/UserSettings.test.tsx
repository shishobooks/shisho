import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useNavLibraries } from "@/hooks/queries/libraries";
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

vi.mock("@/hooks/queries/libraries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/queries/libraries")>()),
  useNavLibraries: vi.fn(),
}));

beforeEach(() => {
  vi.mocked(useNavLibraries).mockReturnValue({ data: [] } as never);
});

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
      <RouterProvider
        router={createMemoryRouter([{ path: "/", element: <UserSettings /> }])}
      />
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

describe("UserSettings – theme section", () => {
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

describe("UserSettings – reader settings", () => {
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

describe("UserSettings – default library", () => {
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

describe("UserSettings Delivery Rules", () => {
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
