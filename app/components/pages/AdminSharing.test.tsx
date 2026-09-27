import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useAuth } from "@/hooks/useAuth";

import AdminSharing from "./AdminSharing";

vi.mock("@/hooks/useAuth", () => ({
  useAuth: vi.fn(),
}));

const mockSettings = { enabled: false, require_expiration: false };
const mockMutate = vi.fn();

vi.mock("@/hooks/queries/sharing", () => ({
  useSharingSettings: () => ({
    isLoading: false,
    isError: false,
    data: mockSettings,
  }),
  useUpdateSharingSettings: () => ({
    mutate: mockMutate,
    isPending: false,
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

const renderPage = () =>
  render(
    <MemoryRouter>
      <AdminSharing />
    </MemoryRouter>,
  );

describe("AdminSharing", () => {
  beforeEach(() => {
    mockMutate.mockReset();
    mockSettings.enabled = false;
    mockSettings.require_expiration = false;
    mockAuth(["config:read", "config:write"]);
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

  it("saves the enable switch as soon as it changes", async () => {
    renderPage();

    await userEvent.click(
      screen.getByRole("switch", { name: "Enable Share Links" }),
    );

    expect(mockMutate).toHaveBeenCalledWith(
      { enabled: true },
      expect.anything(),
    );
  });

  it("saves the require expiration switch as soon as it changes", async () => {
    renderPage();

    await userEvent.click(
      screen.getByRole("switch", { name: "Require expiration" }),
    );

    expect(mockMutate).toHaveBeenCalledWith(
      { require_expiration: true },
      expect.anything(),
    );
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
