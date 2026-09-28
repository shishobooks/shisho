import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useAuth } from "@/hooks/useAuth";

import AdminSharing from "./AdminSharing";

vi.mock("@/hooks/useAuth", () => ({
  useAuth: vi.fn(),
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const mockSettings = { enabled: false, require_expiration: false };
const mockMutate = vi.fn();
let mockIsPending = false;

vi.mock("@/hooks/queries/sharing", () => ({
  useSharingSettings: () => ({
    isLoading: false,
    isError: false,
    data: mockSettings,
  }),
  useUpdateSharingSettings: () => ({
    mutate: mockMutate,
    isPending: mockIsPending,
  }),
}));

const mockAuth = (permissions: string[]) => {
  vi.mocked(useAuth).mockReturnValue({
    demoMode: false,
    hasLibraryAccess: vi.fn(),
    hasPermission: (resource: string, operation: string) =>
      permissions.includes(`${resource}:${operation}`),
    canWrite: vi.fn(),
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    needsSetup: false,
    refetch: vi.fn(),
    setAuthUser: vi.fn(),
    user: null,
  });
};

const createUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

const renderPage = () =>
  render(
    <MemoryRouter>
      <AdminSharing />
    </MemoryRouter>,
  );

describe("AdminSharing", () => {
  beforeEach(() => {
    vi.mocked(toast.success).mockReset();
    mockMutate.mockReset();
    mockIsPending = false;
    mockSettings.enabled = false;
    mockSettings.require_expiration = false;
    mockAuth(["config:read", "config:write", "users:read"]);
  });

  it("shows both switches with their saved state and the reachability disclaimer", () => {
    mockSettings.require_expiration = true;
    renderPage();

    expect(
      screen.getByRole("switch", { name: "Enable Share Links" }),
    ).not.toBeChecked();
    expect(
      screen.getByRole("switch", { name: "Require expiration" }),
    ).toBeChecked();
    expect(
      screen.getByText(/must be reachable by the people you share links with/),
    ).toBeInTheDocument();
  });

  it("explains the shares permission and links to Users", () => {
    renderPage();

    expect(screen.getByText(/Shares/, { selector: "strong" })).toBeVisible();
    expect(screen.getByRole("link", { name: "Users" })).toHaveAttribute(
      "href",
      "/settings/users",
    );
  });

  it("names Users as plain text when the user cannot open it", () => {
    mockAuth(["config:read", "config:write"]);
    renderPage();

    expect(
      screen.queryByRole("link", { name: "Users" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText(/grant it to them in Users\./)).toBeVisible();
  });

  it("saves the enable switch as soon as it changes", async () => {
    const user = createUser();
    renderPage();

    await user.click(
      screen.getByRole("switch", { name: "Enable Share Links" }),
    );

    expect(mockMutate).toHaveBeenCalledWith(
      { enabled: true },
      expect.anything(),
    );
  });

  it("saves the require expiration switch as soon as it changes", async () => {
    const user = createUser();
    renderPage();

    await user.click(
      screen.getByRole("switch", { name: "Require expiration" }),
    );

    expect(mockMutate).toHaveBeenCalledWith(
      { require_expiration: true },
      expect.anything(),
    );
  });

  // The mocked mutation resolves at once so the success callback runs.
  const resolveSaves = () =>
    mockMutate.mockImplementation(
      (
        _payload: { enabled?: boolean; require_expiration?: boolean },
        options: { onSuccess?: () => void },
      ) => {
        options.onSuccess?.();
      },
    );

  it("confirms turning each switch on with a toast", async () => {
    resolveSaves();
    const user = createUser();
    renderPage();

    await user.click(
      screen.getByRole("switch", { name: "Enable Share Links" }),
    );
    expect(toast.success).toHaveBeenLastCalledWith("Share Links turned on");

    await user.click(
      screen.getByRole("switch", { name: "Require expiration" }),
    );
    expect(toast.success).toHaveBeenLastCalledWith("New links must now expire");
  });

  it("confirms turning each switch off with a toast", async () => {
    resolveSaves();
    mockSettings.enabled = true;
    mockSettings.require_expiration = true;
    const user = createUser();
    renderPage();

    await user.click(
      screen.getByRole("switch", { name: "Enable Share Links" }),
    );
    expect(toast.success).toHaveBeenLastCalledWith("Share Links turned off");

    await user.click(
      screen.getByRole("switch", { name: "Require expiration" }),
    );
    expect(toast.success).toHaveBeenLastCalledWith(
      "New links may now last forever",
    );
  });

  it("disables both switches while a save is in flight", () => {
    mockIsPending = true;
    renderPage();

    for (const switchEl of screen.getAllByRole("switch")) {
      expect(switchEl).toBeDisabled();
    }
  });

  it("shows the saved state as text without config write", () => {
    mockAuth(["config:read"]);
    mockSettings.enabled = true;
    renderPage();

    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Enable Share Links")).toHaveTextContent("On");
    expect(screen.getByLabelText("Require expiration")).toHaveTextContent(
      "Off",
    );
  });
});
