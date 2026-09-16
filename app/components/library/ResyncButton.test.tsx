import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { toast, Toaster } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ResyncButton } from "./ResyncButton";

afterEach(() => {
  toast.dismiss();
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("starting a library scan", () => {
  it.each([403, 500])(
    "shows a toast on HTTP %s and allows another attempt",
    async (status) => {
      vi.stubGlobal("__APP_VERSION__", "test");
      const message =
        status === 403
          ? "This action is unavailable in the demo."
          : "Could not start scan.";
      let attempts = 0;
      const fetch = vi
        .spyOn(globalThis, "fetch")
        .mockImplementation(async (_input, init) => {
          if (init?.method === "POST") {
            attempts++;
            if (attempts === 1)
              return Response.json(
                { error: { code: "rejected", message } },
                { status },
              );
            return Response.json({ id: 1, status: "pending", type: "scan" });
          }
          return Response.json({ items: [], total: 0 });
        });
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      });
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const view = render(
        <QueryClientProvider client={client}>
          <MemoryRouter>
            <ResyncButton libraryId={42} />
            <Toaster />
          </MemoryRouter>
        </QueryClientProvider>,
      );
      try {
        await waitFor(() => expect(screen.getByRole("button")).toBeEnabled());
        await user.click(screen.getByRole("button"));
        expect(await screen.findByText(message)).toBeInTheDocument();
        expect(screen.getAllByText(message)).toHaveLength(1);
        expect(attempts).toBe(1);
        await waitFor(() => expect(screen.getByRole("button")).toBeEnabled());
        await user.click(screen.getByRole("button"));
        await waitFor(() => expect(attempts).toBe(2));
        expect(fetch).toHaveBeenCalledWith(
          "/api/jobs",
          expect.objectContaining({
            method: "POST",
            body: JSON.stringify({ type: "scan", library_id: 42, data: {} }),
          }),
        );
      } finally {
        view.unmount();
        client.clear();
      }
    },
  );
});
