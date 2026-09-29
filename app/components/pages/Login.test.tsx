import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";

import Login from "./Login";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const renderLogin = () =>
  render(
    <MemoryRouter>
      <Login />
    </MemoryRouter>,
  );

describe("Login Demo Mode", () => {
  beforeEach(() => {
    setAuth({ demoMode: true, user: null });
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
    setAuth({ user: null });

    renderLogin();

    expect(
      screen.queryByText(/read-only public demo/i),
    ).not.toBeInTheDocument();
    expect(screen.getByLabelText("Username")).toHaveValue("");
    expect(screen.getByLabelText("Password")).toHaveValue("");
  });
});
