import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import { PermissionKoboSync, type APIKey } from "@/types/generated/apikeys";

import SecuritySettings from "./SecuritySettings";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/components/library/TopNav", () => ({ default: () => null }));

const koboKey = (id: string): APIKey => ({
  id,
  userId: 1,
  name: `Kobo ${id}`,
  key: `key-${id}`,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
  permissions: [
    {
      id: `p-${id}`,
      apiKeyId: id,
      permission: PermissionKoboSync,
      createdAt: "2026-01-01T00:00:00Z",
    },
  ],
});

const stubRequests = () =>
  vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
    if (path === "/user/api-keys") return [koboKey("a"), koboKey("b")];
    if (path === "/lists") return { items: [], total: 0 };
    return [];
  });

const requestedPaths = (request: ReturnType<typeof stubRequests>) =>
  request.mock.calls.map((call) => call[1]);

const renderPage = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <SecuritySettings />
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

describe("SecuritySettings Kobo setup requests", () => {
  // The Kobo sync scope is open to every signed-in user, so this role holds
  // no permissions at all.
  beforeEach(() => setAuth());

  afterEach(() => vi.restoreAllMocks());

  it("requests no libraries or lists while every setup dialog is closed", async () => {
    const request = stubRequests();

    renderPage();
    expect(await screen.findByText("Kobo a")).toBeInTheDocument();
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(requestedPaths(request)).not.toContain("/user/libraries");
    expect(requestedPaths(request)).not.toContain("/lists");
  });

  it("requests the libraries and lists for the scope once a dialog opens", async () => {
    const request = stubRequests();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    renderPage();
    await user.click(
      (await screen.findAllByRole("button", { name: "Setup" }))[0],
    );

    await waitFor(() =>
      expect(requestedPaths(request)).toContain("/user/libraries"),
    );
    expect(requestedPaths(request)).toContain("/lists");
  });
});
