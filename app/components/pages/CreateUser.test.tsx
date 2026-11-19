import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import CreateUser from "./CreateUser";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

// useUnsavedChanges calls react-router's useBlocker, which needs a data router.
vi.mock("@/hooks/useUnsavedChanges", () => ({
  useUnsavedChanges: () => ({
    cancelNavigation: vi.fn(),
    proceedNavigation: vi.fn(),
    showBlockerDialog: false,
  }),
}));

const role = (id: number, name: string, isSystem = true) => ({
  id,
  name,
  is_system: isSystem,
  created_at: "",
  updated_at: "",
});

const mockApi = () =>
  vi.spyOn(API, "request").mockImplementation(async (method, path) => {
    if (method === "GET" && path === "/roles") {
      return {
        items: [role(1, "admin"), role(2, "editor"), role(3, "viewer")],
        total: 3,
      };
    }
    if (method === "GET" && path === "/libraries") {
      return {
        items: [
          { id: 7, name: "Comics" },
          { id: 8, name: "Novels" },
        ],
        total: 2,
      };
    }
    if (method === "POST" && path === "/users") {
      return { id: 42, username: "newuser" };
    }
    if (method === "POST" && path === "/invite-links") {
      return {
        id: 5,
        token: "tok123",
        role_id: 3,
        all_library_access: false,
        library_ids: [7],
        expires_at: "2026-11-25T12:00:00Z",
        created_by_username: "admin",
      };
    }
    return {};
  });

const renderPage = (entry = "/admin/users/create") => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[entry]}>
        <Routes>
          <Route element={<CreateUser />} path="/admin/users/create" />
          <Route element={<div>User page</div>} path="/admin/users/:id" />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const setupUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime, delay: null });

const noLibrariesWarning = /No libraries are selected/;

describe("CreateUser", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["users:read", "users:write"] });
  });

  it("offers an Invite link by default and Set a password as the other path", async () => {
    mockApi();
    renderPage();

    const methods = screen.getByRole("radiogroup", {
      name: "How do you want to add them?",
    });
    expect(
      within(methods).getByRole("radio", { name: "Invite link" }),
    ).toBeChecked();
    expect(
      within(methods).getByRole("radio", { name: "Set a password" }),
    ).not.toBeChecked();
    expect(
      screen.getByRole("button", { name: "Create invite link" }),
    ).toBeVisible();
    // The other path's form is kept, hidden, so switching keeps its input.
    expect(screen.getByLabelText("Password")).not.toBeVisible();
  });

  it("creates an invite with the chosen role, Libraries, and expiry, and shows the link to copy", async () => {
    const request = mockApi();
    const user = setupUser();
    renderPage();

    await user.click(await screen.findByRole("radio", { name: /viewer/ }));
    await user.click(
      screen.getByRole("checkbox", { name: "Access to all libraries" }),
    );
    await user.click(await screen.findByRole("checkbox", { name: "Comics" }));
    await user.click(
      screen.getByRole("button", { name: "Create invite link" }),
    );

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith("POST", "/invite-links", {
        role_id: 3,
        all_library_access: false,
        library_ids: [7],
        expires_in_days: 7,
      }),
    );
    expect(await screen.findByText("Invite link ready")).toBeVisible();
    expect(
      screen.getByLabelText("Invite link", { selector: "code" }),
    ).toHaveTextContent(`${window.location.origin}/invite/tok123`);
    expect(
      screen.getByRole("button", { name: "Copy invite link" }),
    ).toBeVisible();
  });

  it("warns on either path when no Library is selected", async () => {
    mockApi();
    const user = setupUser();
    renderPage();

    expect(screen.queryByText(noLibrariesWarning)).not.toBeInTheDocument();
    await user.click(
      screen.getByRole("checkbox", { name: "Access to all libraries" }),
    );
    expect(screen.getByText(noLibrariesWarning)).toBeVisible();
    await user.click(await screen.findByRole("checkbox", { name: "Novels" }));
    expect(screen.queryByText(noLibrariesWarning)).not.toBeInTheDocument();

    await user.click(screen.getByRole("radio", { name: "Set a password" }));
    await user.click(
      screen.getByRole("checkbox", { name: "Access to all libraries" }),
    );
    expect(screen.getByText(noLibrariesWarning)).toBeVisible();
    await user.click(await screen.findByRole("checkbox", { name: "Comics" }));
    expect(screen.queryByText(noLibrariesWarning)).not.toBeInTheDocument();
  });

  it("picks the role from a radio group named by its Select Role label and sends it on create", async () => {
    const request = mockApi();
    const user = setupUser();
    renderPage("/admin/users/create?method=password");

    const roleGroup = await screen.findByRole("radiogroup", {
      name: "Select Role",
    });
    const editor = await within(roleGroup).findByRole("radio", {
      name: /editor/,
    });
    const viewer = within(roleGroup).getByRole("radio", { name: /viewer/ });
    expect(editor).not.toBeChecked();

    await user.click(editor);
    expect(editor).toBeChecked();
    // A radio stays chosen when clicked again; only another role replaces it.
    await user.click(editor);
    expect(editor).toBeChecked();
    await user.click(viewer);
    expect(viewer).toBeChecked();
    expect(editor).not.toBeChecked();
    await user.click(editor);

    await user.type(screen.getByLabelText("Username"), "newuser");
    await user.type(screen.getByLabelText("Password"), "password123");
    await user.type(screen.getByLabelText("Confirm Password"), "password123");
    await user.click(screen.getByRole("button", { name: "Create User" }));

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "POST",
        "/users",
        expect.objectContaining({ role_id: 2, username: "newuser" }),
      ),
    );
  });

  it("requires a password reset by default and then shows what to send them", async () => {
    const request = mockApi();
    const user = setupUser();
    renderPage();

    await user.click(screen.getByRole("radio", { name: "Set a password" }));
    const reset = screen.getByRole("checkbox", {
      name: "Require password reset on first login",
    });
    expect(reset).toBeChecked();

    await user.type(screen.getByLabelText("Username"), "newuser");
    await user.type(screen.getByLabelText("Password"), "password123");
    await user.type(screen.getByLabelText("Confirm Password"), "password123");
    await user.click(
      within(screen.getByRole("radiogroup", { name: "Select Role" })).getByRole(
        "radio",
        { name: /viewer/ },
      ),
    );
    await user.click(screen.getByRole("button", { name: "Create User" }));

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "POST",
        "/users",
        expect.objectContaining({ require_password_reset: true }),
      ),
    );
    expect(await screen.findByText("Send these to them")).toBeVisible();
    expect(screen.getByLabelText("Server URL")).toHaveTextContent(
      window.location.origin,
    );
    expect(screen.getByLabelText("Username")).toHaveTextContent("newuser");
    expect(
      screen.getByRole("button", { name: "Copy server url" }),
    ).toBeVisible();
    expect(screen.getByRole("button", { name: "Copy username" })).toBeVisible();
    expect(screen.getByText(/choose a new password/)).toBeVisible();
    // The admin stays here to pass these on instead of landing on the user.
    expect(screen.queryByText("User page")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "View user" })).toHaveAttribute(
      "href",
      "/admin/users/42",
    );
  });

  it("names the library checkboxes with their heading", async () => {
    mockApi();
    const user = setupUser();
    renderPage();

    await user.click(
      await screen.findByRole("checkbox", { name: "Access to all libraries" }),
    );

    const group = await screen.findByRole("group", {
      name: "Select Libraries",
    });
    expect(
      await within(group).findByRole("checkbox", { name: "Comics" }),
    ).toBeVisible();
  });
});
