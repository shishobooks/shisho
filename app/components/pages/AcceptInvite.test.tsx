import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API, DEMO_MODE_MESSAGE, ShishoAPIError } from "@/libraries/api";
import { auth, setAuth } from "@/testing/auth";

import AcceptInvite from "./AcceptInvite";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

// useUnsavedChanges calls react-router's useBlocker, which needs a data router.
vi.mock("@/hooks/useUnsavedChanges", () => ({
  useUnsavedChanges: () => ({
    cancelNavigation: vi.fn(),
    proceedNavigation: vi.fn(),
    showBlockerDialog: false,
  }),
}));

const TOKEN = "invite-token";

const me = {
  id: 9,
  username: "newreader",
  role_id: 3,
  role_name: "viewer",
  permissions: ["books:read"],
  library_access: [7],
  must_change_password: false,
};

type Lookup = { valid: boolean; invited_by: string } | Error;

const mockApi = (lookup: Lookup, accept: () => unknown = () => me) =>
  vi.spyOn(API, "request").mockImplementation(async (method, path) => {
    if (method === "GET" && path === `/invites/${TOKEN}`) {
      if (lookup instanceof Error) throw lookup;
      return lookup;
    }
    if (method === "POST" && path === `/invites/${TOKEN}/accept`) {
      return accept();
    }
    return undefined;
  });

const renderPage = (entry = `/invite/${TOKEN}`) =>
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter initialEntries={[entry]}>
        <Routes>
          <Route element={<AcceptInvite />} path="/invite/:token" />
          <Route element={<AcceptInvite />} path="/invite/*" />
          <Route element={<div>The app</div>} path="/" />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );

const setupUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime, delay: null });

const fillForm = async (
  user: ReturnType<typeof setupUser>,
  username: string,
) => {
  await user.type(await screen.findByLabelText("Username"), username);
  await user.type(screen.getByLabelText("Password"), "password123");
  await user.type(screen.getByLabelText("Confirm Password"), "password123");
  await user.type(screen.getByLabelText(/Email/), "me@example.com");
};

const unavailableHeading = "This invite link is no longer valid";

describe("AcceptInvite", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ user: null });
  });

  it("welcomes the person by naming who invited them, then signs them in and takes them into the app", async () => {
    const request = mockApi({ valid: true, invited_by: "robin" });
    const user = setupUser();
    renderPage();

    expect(
      await screen.findByRole("heading", {
        name: "robin invited you to their Shisho library",
      }),
    ).toBeVisible();
    expect(screen.getByText(/Shisho is where they keep/)).toBeVisible();

    await fillForm(user, "newreader");
    await user.click(screen.getByRole("button", { name: "Create account" }));

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith("POST", `/invites/${TOKEN}/accept`, {
        username: "newreader",
        email: "me@example.com",
        password: "password123",
      }),
    );
    await waitFor(() => expect(auth.setAuthUser).toHaveBeenCalledWith(me));
    expect(await screen.findByText("The app")).toBeVisible();
  });

  it("flags a taken username on the form and keeps what was typed", async () => {
    mockApi({ valid: true, invited_by: "robin" }, () => {
      throw new ShishoAPIError(
        "That username is already taken.",
        "username_taken",
        422,
      );
    });
    const user = setupUser();
    renderPage();

    await fillForm(user, "viewer");
    await user.click(screen.getByRole("button", { name: "Create account" }));

    const username = screen.getByLabelText("Username");
    await waitFor(() =>
      expect(username).toHaveAttribute("aria-invalid", "true"),
    );
    expect(username).toHaveAccessibleDescription(
      "That username is taken. Pick another one.",
    );
    expect(username).toHaveValue("viewer");
    expect(screen.getByLabelText("Password")).toHaveValue("password123");
    expect(screen.getByLabelText(/Email/)).toHaveValue("me@example.com");
    expect(auth.setAuthUser).not.toHaveBeenCalled();

    // Editing the username clears the flag.
    await user.type(username, "2");
    expect(username).not.toHaveAttribute("aria-invalid");
  });

  it("says an invite that can't be accepted is no longer valid, naming whom to ask", async () => {
    mockApi({ valid: false, invited_by: "robin" });
    renderPage();

    expect(
      await screen.findByRole("heading", { name: unavailableHeading }),
    ).toBeVisible();
    expect(screen.getByText("robin")).toBeVisible();
    expect(screen.queryByLabelText("Username")).not.toBeInTheDocument();
  });

  it("shows the same page for an unknown or truncated link", async () => {
    mockApi(new ShishoAPIError("Invite Link not found.", "not_found", 404));
    renderPage();
    expect(
      await screen.findByRole("heading", { name: unavailableHeading }),
    ).toBeVisible();
    expect(screen.getByText(/the person who invited you/)).toBeVisible();
  });

  it("switches to the no-longer-valid page when the invite is used while the form is open", async () => {
    let valid = true;
    vi.spyOn(API, "request").mockImplementation(async (method, path) => {
      if (method === "GET" && path === `/invites/${TOKEN}`) {
        return { valid, invited_by: "robin" };
      }
      valid = false;
      throw new ShishoAPIError("Invite Link not found.", "not_found", 404);
    });
    const user = setupUser();
    renderPage();

    await fillForm(user, "newreader");
    await user.click(screen.getByRole("button", { name: "Create account" }));
    expect(
      await screen.findByRole("heading", { name: unavailableHeading }),
    ).toBeVisible();
  });

  it("flags a taken email address on the form", async () => {
    mockApi({ valid: true, invited_by: "robin" }, () => {
      throw new ShishoAPIError(
        "That email address is already in use.",
        "email_taken",
        422,
      );
    });
    const user = setupUser();
    renderPage();

    await fillForm(user, "newreader");
    await user.click(screen.getByRole("button", { name: "Create account" }));

    const email = screen.getByLabelText(/Email/);
    await waitFor(() => expect(email).toHaveAttribute("aria-invalid", "true"));
    expect(email).toHaveAccessibleDescription(
      "That email address is already in use.",
    );
    expect(screen.getByLabelText("Username")).toHaveValue("newreader");
  });

  it("offers a signed-in visitor to sign out, then shows the form", async () => {
    setAuth({ user: { username: "admin" } });
    auth.logout.mockImplementation(async () => {
      auth.user = null;
    });
    mockApi({ valid: true, invited_by: "robin" });
    const user = setupUser();
    renderPage();

    expect(
      await screen.findByText(/Sign out to accept this invite/),
    ).toBeVisible();
    expect(screen.getByText("admin")).toBeVisible();
    expect(screen.queryByLabelText("Username")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Sign out" }));
    expect(auth.logout).toHaveBeenCalled();
    expect(await screen.findByLabelText("Username")).toBeVisible();
  });

  it("tells a Demo Mode visitor invites aren't available, without asking the server", async () => {
    setAuth({ user: null, demoMode: true });
    // The real API, so a refused lookup would reach checkStatus and toast.
    const request = vi.spyOn(API, "request");
    renderPage();
    expect(
      await screen.findByRole("heading", {
        name: "Invites aren't available in the demo",
      }),
    ).toBeVisible();
    expect(request).not.toHaveBeenCalled();
    expect(screen.queryByText(DEMO_MODE_MESSAGE)).not.toBeInTheDocument();
  });
});
