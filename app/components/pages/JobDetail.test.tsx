import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

const pluginLine = {
  id: 1,
  created_at: "2026-11-15T15:07:08Z",
  job_id: 14,
  level: "warn",
  message: "plugin file parser not used for extensions Shisho parses itself",
  plugin: "m4a-parser",
  data: JSON.stringify({ extensions: ["m4a"], scope: "local" }),
};

const plainLine = {
  id: 2,
  created_at: "2026-11-15T15:07:09Z",
  job_id: 14,
  level: "info",
  message: "finished scan job",
};

// Serves job 14 and its log lines, filtered by the plugin and level params as
// the API filters them.
const mockJobAPI = () => {
  vi.spyOn(API, "request").mockImplementation(
    async (_method, path, _body, params) => {
      if (path === "/jobs/14/logs") {
        const { plugin, level } = (params ?? {}) as {
          plugin?: string;
          level?: string[];
        };
        return {
          items: [pluginLine, plainLine].filter(
            (line) =>
              (!plugin || ("plugin" in line && line.plugin === plugin)) &&
              (!level || level.includes(line.level)),
          ),
        };
      }
      return {
        id: 14,
        type: "scan",
        status: "completed",
        created_at: "2026-11-15T15:07:08Z",
        updated_at: "2026-11-15T15:07:12Z",
      };
    },
  );
};

describe("JobDetail", () => {
  afterEach(() => vi.restoreAllMocks());

  // The server stores a job log line's plugin in its own field, not in its
  // data, so the page has to read it from there to name the plugin.
  it("names the plugin a log line came from and offers it as a filter", async () => {
    setAuth({ permissions: ["jobs:read"] });
    mockJobAPI();

    renderJobDetail();

    expect(await screen.findByText(/plugin="m4a-parser"/)).toBeInTheDocument();
    expect(
      screen.getByRole("combobox", { name: "Plugin" }),
    ).toBeInTheDocument();
  });

  it("finds a log line by its plugin when searching", async () => {
    setAuth({ permissions: ["jobs:read"] });
    mockJobAPI();
    const user = userEvent.setup();

    renderJobDetail();
    await screen.findByText(/plugin="m4a-parser"/);
    await user.type(
      screen.getByRole("textbox", { name: "Search logs" }),
      "m4a-parser",
    );

    expect(screen.getByText(pluginLine.message)).toBeInTheDocument();
    expect(screen.queryByText("finished scan job")).not.toBeInTheDocument();
  });

  // A level with no lines from the chosen plugin empties the list; the Plugin
  // filter must stay so it can still be cleared.
  it("keeps the chosen plugin filter when other filters leave no lines", async () => {
    setAuth({ permissions: ["jobs:read"] });
    mockJobAPI();
    const user = userEvent.setup();

    renderJobDetail();
    await user.click(await screen.findByRole("combobox", { name: "Plugin" }));
    await user.click(screen.getByRole("option", { name: "m4a-parser" }));
    await user.click(screen.getByRole("button", { name: "info" }));

    expect(
      await screen.findByText("No matching log messages"),
    ).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Plugin" })).toHaveTextContent(
      "m4a-parser",
    );
  });
});
