import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import AdminJobs from "./AdminJobs";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

describe("AdminJobs pagination", () => {
  afterEach(() => vi.restoreAllMocks());

  it("replaces a page past the end with the last page", async () => {
    setAuth({ permissions: ["jobs:read"] });
    // 25 jobs is two pages; page 9 has none of them.
    vi.spyOn(API, "request").mockResolvedValue({ items: [], total: 25 });
    const router = createMemoryRouter([{ path: "/", element: <AdminJobs /> }], {
      initialEntries: ["/?page=9"],
    });
    render(
      <QueryClientProvider client={new QueryClient()}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );

    await waitFor(() => expect(router.state.location.search).toBe("?page=2"));
    expect(router.state.historyAction).toBe("REPLACE");
    expect(screen.queryByText("No jobs found.")).not.toBeInTheDocument();
  });
});
