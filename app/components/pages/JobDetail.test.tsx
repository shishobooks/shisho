import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import JobDetail from "./JobDetail";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const renderJobDetail = () => {
  const router = createMemoryRouter(
    [{ path: "/jobs/:id", element: <JobDetail /> }],
    { initialEntries: ["/jobs/14"] },
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

describe("JobDetail", () => {
  afterEach(() => vi.restoreAllMocks());

  // The server stores a job log line's plugin in its own field, not in its
  // data, so the page has to read it from there to name the plugin.
  it("names the plugin a log line came from and offers it as a filter", async () => {
    setAuth({ permissions: ["jobs:read"] });
    vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
      if (path === "/jobs/14/logs") {
        return {
          items: [
            {
              id: 1,
              created_at: "2026-11-15T15:07:08Z",
              job_id: 14,
              level: "warn",
              message:
                "plugin file parser not used for extensions Shisho parses itself",
              plugin: "m4a-parser",
              data: JSON.stringify({ extensions: ["m4a"], scope: "local" }),
            },
            {
              id: 2,
              created_at: "2026-11-15T15:07:09Z",
              job_id: 14,
              level: "info",
              message: "finished scan job",
            },
          ],
        };
      }
      return {
        id: 14,
        type: "scan",
        status: "completed",
        created_at: "2026-11-15T15:07:08Z",
        updated_at: "2026-11-15T15:07:12Z",
      };
    });

    renderJobDetail();

    expect(await screen.findByText(/plugin="m4a-parser"/)).toBeInTheDocument();
    expect(
      screen.getByRole("combobox", { name: "Plugin" }),
    ).toBeInTheDocument();
  });
});
