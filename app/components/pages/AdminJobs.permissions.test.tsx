import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import AdminJobs from "./AdminJobs";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const jobRequests = () => {
  const request = vi
    .spyOn(API, "request")
    .mockResolvedValue({ items: [], total: 0 });
  return () => request.mock.calls.filter((call) => call[1] === "/jobs");
};

const renderPage = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <AdminJobs />
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const flush = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });

describe("AdminJobs refresh", () => {
  afterEach(() => vi.restoreAllMocks());

  it("sends nothing for a role without Jobs Read", async () => {
    setAuth({ permissions: ["jobs:write"] });
    const jobs = jobRequests();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    renderPage();
    await user.click(await screen.findByRole("button", { name: "Refresh" }));
    await flush();

    expect(jobs()).toEqual([]);
  });

  it("refetches the jobs for a role with Jobs Read", async () => {
    setAuth({ permissions: ["jobs:read"] });
    const jobs = jobRequests();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    renderPage();
    await waitFor(() => expect(jobs()).toHaveLength(1));
    await user.click(await screen.findByRole("button", { name: "Refresh" }));

    await waitFor(() => expect(jobs()).toHaveLength(2));
  });
});
