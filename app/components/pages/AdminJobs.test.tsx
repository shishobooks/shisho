import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
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
    expect(
      screen.queryByRole("heading", { name: "No jobs yet" }),
    ).not.toBeInTheDocument();
  });
});

// A fresh server has no jobs until its first library exists: scheduled
// scans skip a server with no libraries.
describe("AdminJobs with no jobs", () => {
  afterEach(() => vi.restoreAllMocks());

  it("explains what jobs are and how they show up", async () => {
    setAuth({ permissions: ["jobs:read"] });
    vi.spyOn(API, "request").mockResolvedValue({ items: [], total: 0 });
    const router = createMemoryRouter([{ path: "/", element: <AdminJobs /> }]);
    render(
      <QueryClientProvider client={new QueryClient()}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );

    const heading = await screen.findByRole("heading", { name: "No jobs yet" });
    const emptyState = heading.parentElement!;
    expect(emptyState).toHaveTextContent(
      "Jobs are background tasks like library scans and bulk downloads.",
    );
    // The page header holds the only action, so the empty state has none.
    expect(within(emptyState).queryByRole("button")).toBeNull();
    expect(within(emptyState).queryByRole("link")).toBeNull();
    expect(screen.queryByText(/Showing/)).toBeNull();
  });
});
