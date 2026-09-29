import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { toast, Toaster } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ALL_PERMISSIONS, setAuth } from "@/testing/auth";

import AdminSharing from "./AdminSharing";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

afterEach(() => {
  toast.dismiss();
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("saving a sharing switch through the API", () => {
  it.each([
    [403, "demo_mode", "This action is unavailable in the demo."],
    [500, "internal", "Something went wrong."],
  ])(
    "shows one toast on HTTP %s and keeps the saved value",
    async (status, code, message) => {
      vi.stubGlobal("__APP_VERSION__", "test");
      setAuth({ permissions: ALL_PERMISSIONS });
      vi.spyOn(globalThis, "fetch").mockImplementation(async (_input, init) => {
        if (init?.method === "PUT") {
          return Response.json({ error: { code, message } }, { status });
        }
        return Response.json({ enabled: false, require_expiration: false });
      });
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      });
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const view = render(
        <QueryClientProvider client={client}>
          <MemoryRouter>
            <AdminSharing />
            <Toaster />
          </MemoryRouter>
        </QueryClientProvider>,
      );
      try {
        const toggle = await screen.findByRole("switch", {
          name: "Enable Share Links",
        });
        await user.click(toggle);
        expect(await screen.findByText(message)).toBeInTheDocument();
        await waitFor(() => expect(toggle).toBeEnabled());
        expect(screen.getAllByText(message)).toHaveLength(1);
        expect(toggle).not.toBeChecked();
      } finally {
        view.unmount();
        client.clear();
      }
    },
  );
});
