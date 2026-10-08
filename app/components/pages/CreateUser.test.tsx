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
      return { items: [{ id: 7, name: "Comics" }], total: 1 };
    }
    if (method === "POST" && path === "/users") return { id: 42 };
    return {};
  });

const renderPage = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/settings/users/create"]}>
        <Routes>
          <Route element={<CreateUser />} path="/settings/users/create" />
          <Route element={<div>User page</div>} path="/settings/users/:id" />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const setupUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime, delay: null });

describe("CreateUser", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["users:read", "users:write"] });
  });

  it("picks the role from a radio group named by its Select Role label and sends it on create", async () => {
    const request = mockApi();
    const user = setupUser();
    renderPage();

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
