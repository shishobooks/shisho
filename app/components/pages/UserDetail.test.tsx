import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import UserDetail from "./UserDetail";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

// useUnsavedChanges calls react-router's useBlocker, which needs a data router.
vi.mock("@/hooks/useUnsavedChanges", () => ({
  useUnsavedChanges: () => ({
    cancelNavigation: vi.fn(),
    proceedNavigation: vi.fn(),
    showBlockerDialog: false,
  }),
}));

const role = (id: number, name: string) => ({
  id,
  name,
  is_system: true,
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
    if (method === "GET" && path === "/users/5") {
      return {
        id: 5,
        username: "reader",
        role_id: 3,
        is_active: true,
        must_change_password: false,
        created_at: "",
        updated_at: "",
        library_access: [{ id: 1, user_id: 5, library_id: 7 }],
      };
    }
    if (method === "POST" && path === "/users/5") return { id: 5 };
    return {};
  });

const renderPage = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/settings/users/5"]}>
        <Routes>
          <Route element={<UserDetail />} path="/settings/users/:id" />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const setupUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime, delay: null });

describe("UserDetail", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["users:read", "users:write"] });
  });

  it("shows the user's role in a radio group named by its Select Role label and saves a new one", async () => {
    const request = mockApi();
    const user = setupUser();
    renderPage();

    const roleGroup = await screen.findByRole("radiogroup", {
      name: "Select Role",
    });
    const viewer = await within(roleGroup).findByRole("radio", {
      name: /viewer/,
    });
    await waitFor(() => expect(viewer).toBeChecked());
    const editor = within(roleGroup).getByRole("radio", { name: /editor/ });

    await user.click(editor);
    expect(editor).toBeChecked();
    expect(viewer).not.toBeChecked();

    await user.click(screen.getByRole("button", { name: "Save Changes" }));

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "POST",
        "/users/5",
        expect.objectContaining({ role_id: 2 }),
      ),
    );
  });

  it("sends no role_id when a save changes only the email", async () => {
    const request = mockApi();
    const user = setupUser();
    renderPage();

    const roleGroup = await screen.findByRole("radiogroup", {
      name: "Select Role",
    });
    await waitFor(() =>
      expect(
        within(roleGroup).getByRole("radio", { name: /viewer/ }),
      ).toBeChecked(),
    );
    await user.type(screen.getByLabelText("Email"), "reader@example.com");
    await user.click(screen.getByRole("button", { name: "Save Changes" }));

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "POST",
        "/users/5",
        expect.objectContaining({ email: "reader@example.com" }),
      ),
    );
    const save = request.mock.calls.find(
      (call) => call[0] === "POST" && call[1] === "/users/5",
    );
    expect((save?.[2] as { role_id?: number }).role_id).toBeUndefined();
  });

  it("names the library checkboxes with their heading", async () => {
    mockApi();
    renderPage();

    const group = await screen.findByRole("group", {
      name: "Select Libraries",
    });
    const comics = await within(group).findByRole("checkbox", {
      name: "Comics",
    });
    expect(comics).toBeChecked();
  });

  it("disables the role picker for a role that cannot write users", async () => {
    setAuth({ permissions: ["users:read"] });
    mockApi();
    renderPage();

    const roleGroup = await screen.findByRole("radiogroup", {
      name: "Select Role",
    });
    for (const radio of await within(roleGroup).findAllByRole("radio")) {
      expect(radio).toBeDisabled();
    }
  });
});
