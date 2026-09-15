import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { toast, Toaster } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AuthProvider } from "@/components/contexts/Auth";
import ListDetail from "@/components/pages/ListDetail";
import ListsIndex from "@/components/pages/ListsIndex";
import { MobileNavProvider } from "@/contexts/MobileNav";

import AddToListPopover from "./AddToListPopover";

afterEach(() => {
  toast.dismiss();
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("list dialog save failures through its callers", () => {
  it.each(["lists page", "book popover", "list edit"])(
    "preserves inputs after rejection from %s, then closes on success",
    async (flow) => {
      vi.stubGlobal("__APP_VERSION__", "test");
      let fail = true;
      const writes: string[] = [];
      const list = {
        id: 5,
        name: "Existing list",
        description: "",
        is_ordered: false,
        permission: "owner",
        book_count: 0,
        created_at: "2024-01-01T00:00:00Z",
        updated_at: "2024-01-01T00:00:00Z",
      };
      vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
        const path = String(input).split("?")[0];
        if (init?.method !== "GET") {
          writes.push(path);
          if (fail)
            return Response.json(
              {
                error: {
                  code: "demo_mode",
                  message: "This action is unavailable in the demo.",
                },
              },
              { status: 403 },
            );
          if (path.endsWith("/books"))
            return new Response(null, { status: 204 });
          return Response.json({ ...list, name: "Reading queue" });
        }
        if (path === "/api/auth/status")
          return Response.json({ needs_setup: false, demo_mode: true });
        if (path === "/api/auth/me")
          return Response.json({
            id: 1,
            username: "reader",
            permissions: [],
            role_name: "viewer",
          });
        if (path === "/api/lists/5") return Response.json(list);
        if (
          path === "/api/lists/templates" ||
          path === "/api/books/42/lists" ||
          path.endsWith("/shares")
        )
          return Response.json([]);
        if (path === "/api/settings/user")
          return Response.json({ gallery_size: "medium" });
        return Response.json({ items: [], total: 0 });
      });
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      });
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const view = render(
        <QueryClientProvider client={client}>
          <AuthProvider>
            <MobileNavProvider>
              <MemoryRouter
                initialEntries={[flow === "list edit" ? "/lists/5" : "/lists"]}
              >
                <Routes>
                  <Route
                    element={
                      flow === "book popover" ? (
                        <AddToListPopover bookId={42} />
                      ) : (
                        <ListsIndex />
                      )
                    }
                    path="/lists"
                  />
                  <Route element={<ListDetail />} path="/lists/:id" />
                </Routes>
                <Toaster />
              </MemoryRouter>
            </MobileNavProvider>
          </AuthProvider>
        </QueryClientProvider>,
      );
      try {
        if (flow === "book popover") {
          await user.click(screen.getByTitle("Add to list"));
          await user.click(
            await screen.findByRole("button", { name: "Create New List" }),
          );
        } else {
          await user.click(
            await screen.findByRole("button", {
              name: flow === "list edit" ? "Edit" : "Create List",
            }),
          );
        }
        const name = screen.getByLabelText("Name");
        await user.clear(name);
        await user.type(name, "Reading queue");
        await user.type(screen.getByLabelText("Description"), "Keep my draft");
        await user.click(
          screen.getByRole("checkbox", { name: "Ordered list" }),
        );
        const saveLabel = flow === "list edit" ? "Save" : "Create";
        await user.click(screen.getByRole("button", { name: saveLabel }));
        await waitFor(() => expect(writes).toHaveLength(1));
        await waitFor(() =>
          expect(screen.getByRole("button", { name: saveLabel })).toBeEnabled(),
        );
        expect(screen.getByRole("dialog")).toBeInTheDocument();
        expect(screen.getByLabelText("Name")).toHaveValue("Reading queue");
        expect(screen.getByLabelText("Description")).toHaveValue(
          "Keep my draft",
        );
        expect(
          screen.getByRole("checkbox", { name: "Ordered list" }),
        ).toBeChecked();
        // Modal dialogs hide the toaster from accessibility queries, but the message remains visible.
        expect(
          await screen.findByText("This action is unavailable in the demo."),
        ).toBeInTheDocument();

        await user.keyboard("{Escape}");
        const warning = await screen.findByRole("dialog", {
          name: "Unsaved Changes",
        });
        await user.click(within(warning).getByRole("button", { name: "Stay" }));
        fail = false;
        await user.click(screen.getByRole("button", { name: saveLabel }));
        await waitFor(() =>
          expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
        );
      } finally {
        view.unmount();
        client.clear();
      }
    },
  );
});
