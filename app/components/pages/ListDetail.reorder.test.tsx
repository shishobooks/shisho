import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { toast, Toaster } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AuthProvider } from "@/components/contexts/Auth";
import { MobileNavProvider } from "@/contexts/MobileNav";

import ListDetail from "./ListDetail";

// Stand in for the drag interaction: one button that asks to reorder and,
// like the real list, consumes the rejection.
vi.mock("@/components/library/DraggableBookList", () => ({
  DraggableBookList: ({
    onReorder,
  }: {
    onReorder: (ids: number[]) => Promise<unknown>;
  }) => (
    <button onClick={() => onReorder([2, 1]).catch(() => undefined)}>
      Reorder
    </button>
  ),
}));

afterEach(() => {
  toast.dismiss();
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const list = {
  id: 5,
  name: "Queue",
  description: "",
  is_ordered: true,
  permission: "owner",
  book_count: 2,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
};

const renderWithReorderFailure = (failure: Response) => {
  vi.stubGlobal("__APP_VERSION__", "test");
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const path = String(input).split("?")[0];
    if (init?.method === "PATCH") return failure.clone();
    if (path === "/api/auth/status")
      return Response.json({ needs_setup: false, demo_mode: true });
    if (path === "/api/auth/me")
      return Response.json({
        id: 1,
        username: "reader",
        permissions: ["books:read"],
        role_name: "viewer",
      });
    if (path === "/api/lists/5") return Response.json(list);
    if (path.endsWith("/shares")) return Response.json([]);
    if (path === "/api/user/libraries") return Response.json([]);
    if (path === "/api/settings/user")
      return Response.json({ gallery_size: "m" });
    return Response.json({ items: [], total: 0 });
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AuthProvider>
        <MobileNavProvider>
          <MemoryRouter initialEntries={["/lists/5"]}>
            <Routes>
              <Route element={<ListDetail />} path="/lists/:id" />
            </Routes>
            <Toaster />
          </MemoryRouter>
        </MobileNavProvider>
      </AuthProvider>
    </QueryClientProvider>,
  );
  return userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
};

describe("ListDetail reorder failures", () => {
  it("reports an ordinary failure with a toast", async () => {
    const user = renderWithReorderFailure(
      Response.json(
        { error: { code: "internal", message: "Could not save order" } },
        { status: 500 },
      ),
    );

    await user.click(await screen.findByRole("button", { name: "Reorder" }));

    expect(await screen.findByText("Could not save order")).toBeInTheDocument();
  });

  it("shows only the Demo Mode toast for a demo rejection", async () => {
    const user = renderWithReorderFailure(
      Response.json(
        {
          error: {
            code: "demo_mode",
            message: "This action is unavailable in the demo.",
          },
        },
        { status: 403 },
      ),
    );

    await user.click(await screen.findByRole("button", { name: "Reorder" }));

    await waitFor(() =>
      expect(
        screen.getAllByText("This action is unavailable in the demo."),
      ).toHaveLength(1),
    );
  });
});
