import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useAuth } from "@/hooks/useAuth";

import Login from "./Login";

vi.mock("@/hooks/useAuth", () => ({
  useAuth: vi.fn(),
}));

const renderLogin = () =>
  render(
    <MemoryRouter>
      <Login />
    </MemoryRouter>,
  );

describe("Login Demo Mode", () => {
  beforeEach(() => {
    vi.mocked(useAuth).mockReturnValue({
      demoMode: true,
      hasLibraryAccess: vi.fn(),
      hasPermission: vi.fn(),
      canWrite: vi.fn(),
      isAuthenticated: false,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      needsSetup: false,
      refetch: vi.fn(),
      setAuthUser: vi.fn(),
      user: null,
    });
  });

  it("shows the demo notice and pre-fills the shared credentials", () => {
    renderLogin();

    expect(
      screen.getByText(
        "This is a read-only public demo of Shisho. Sign in with the pre-filled credentials. Changes and downloads are disabled.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Username")).toHaveValue("demo");
    expect(screen.getByLabelText("Password")).toHaveValue("shishodemo");
  });

  it("does not show or pre-fill demo details outside Demo Mode", () => {
    vi.mocked(useAuth).mockReturnValue({
      ...vi.mocked(useAuth)(),
      demoMode: false,
    });

    renderLogin();

    expect(
      screen.queryByText(/read-only public demo/i),
    ).not.toBeInTheDocument();
    expect(screen.getByLabelText("Username")).toHaveValue("");
    expect(screen.getByLabelText("Password")).toHaveValue("");
  });
});
