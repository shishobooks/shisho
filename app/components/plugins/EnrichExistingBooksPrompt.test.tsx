import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PluginDetail } from "@/components/pages/PluginDetail";
import type {
  AvailablePlugin,
  PluginCapabilities,
} from "@/hooks/queries/plugins";
import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import {
  PluginStatusActive,
  PluginStatusMalfunctioned,
  type PluginStatus,
} from "@/types";
import type { Permission } from "@/utils/permissions";

import { DiscoverTab } from "./DiscoverTab";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/hooks/useUnsavedChanges", () => ({
  useUnsavedChanges: () => ({
    cancelNavigation: vi.fn(),
    proceedNavigation: vi.fn(),
    showBlockerDialog: false,
  }),
}));

const ADMIN: Permission[] = [
  "config:read",
  "config:write",
  "books:read",
  "jobs:read",
  "jobs:write",
];

const enricher: PluginCapabilities = {
  identifierTypes: [],
  metadataEnricher: { description: "", fields: ["title"], fileTypes: [] },
};

const converter: PluginCapabilities = {
  identifierTypes: [],
  inputConverter: {
    description: "",
    mimeTypes: [],
    sourceTypes: ["mobi"],
    targetType: "epub",
  },
};

const plugin = (capabilities: PluginCapabilities): AvailablePlugin => ({
  compatible: true,
  description: "",
  homepage: "",
  id: "goodreads",
  imageUrl: "",
  is_official: true,
  name: "Goodreads",
  overview: "",
  scope: "shisho",
  versions: [
    {
      capabilities,
      changelog: "",
      compatible: true,
      downloadUrl: "",
      manifestVersion: 1,
      minShishoVersion: "0.1.0",
      releaseDate: "2024-01-01",
      sha256: "",
      version: "1.0.0",
    },
  ],
});

const flush = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 50));
  });

const stubServer = (
  available: AvailablePlugin,
  bookCount: number | (() => number),
  installedStatus: PluginStatus = PluginStatusActive,
) =>
  vi.spyOn(API, "request").mockImplementation(async (method, path) => {
    if (path === "/plugins/available") return [available];
    if (path === "/plugins/installed" && method === "GET") return [];
    if (path === "/plugins/installed" && method === "POST") {
      return {
        scope: available.scope,
        id: available.id,
        status: installedStatus,
      };
    }
    if (path === "/plugins/repositories") return [];
    if (path === "/books") {
      return {
        items: [],
        total: typeof bookCount === "function" ? bookCount() : bookCount,
      };
    }
    if (path === "/jobs" && method === "POST") {
      return { id: 1, type: "scan", status: "pending", data: {} };
    }
    return undefined;
  });

const renderDiscover = (
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  }),
) =>
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <DiscoverTab canWrite />
      </MemoryRouter>
    </QueryClientProvider>,
  );

const install = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.click(await screen.findByRole("button", { name: "Install" }));
  await user.click(
    await screen.findByRole("button", { name: "Install Plugin" }),
  );
  await flush();
};

const jobRequests = (request: ReturnType<typeof stubServer>) =>
  request.mock.calls.filter(
    ([method, path]) => method === "POST" && path === "/jobs",
  );

describe("Enrich existing books after installing an enricher", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ADMIN });
  });

  it("offers to enrich the existing books, with their count, and starts the library-wide scan", async () => {
    const request = stubServer(plugin(enricher), 1234);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDiscover();
    await install(user);

    const prompt = await screen.findByRole("dialog", {
      name: "Enrich your 1,234 existing books?",
    });
    expect(prompt).toHaveTextContent(/takes a while/i);
    expect(prompt).toHaveTextContent(/services for every book/i);
    expect(prompt).toHaveTextContent(
      /manual edits and values you chose in identify are kept/i,
    );

    await user.click(
      within(prompt).getByRole("button", { name: "Enrich books" }),
    );
    await flush();

    expect(request).toHaveBeenCalledWith(
      "POST",
      "/jobs",
      { type: "scan", data: { scan_for_new_metadata: true } },
      null,
    );
    expect(
      screen.queryByRole("dialog", { name: /enrich your/i }),
    ).not.toBeInTheDocument();
  });

  it("starts nothing when the admin declines", async () => {
    const request = stubServer(plugin(enricher), 12);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDiscover();
    await install(user);

    const prompt = await screen.findByRole("dialog", {
      name: "Enrich your 12 existing books?",
    });
    await user.click(within(prompt).getByRole("button", { name: "Not now" }));
    await flush();

    expect(jobRequests(request)).toHaveLength(0);
    expect(
      screen.queryByRole("dialog", { name: /enrich your/i }),
    ).not.toBeInTheDocument();
  });

  it("doesn't prompt when no library has books", async () => {
    stubServer(plugin(enricher), 0);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDiscover();
    await install(user);

    expect(
      screen.queryByRole("dialog", { name: /enrich your/i }),
    ).not.toBeInTheDocument();
  });

  it("doesn't prompt later when books arrive after installing into an empty server", async () => {
    let count = 0;
    stubServer(plugin(enricher), () => count);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDiscover(queryClient);
    await install(user);

    // A scan finishes and book listings refresh while the page is open.
    count = 40;
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: ["ListBooks"] });
    });
    await flush();

    expect(
      screen.queryByRole("dialog", { name: /enrich your/i }),
    ).not.toBeInTheDocument();
  });

  it("doesn't prompt when the enricher failed to load", async () => {
    stubServer(plugin(enricher), 50, PluginStatusMalfunctioned);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDiscover();
    await install(user);

    expect(
      screen.queryByRole("dialog", { name: /enrich your/i }),
    ).not.toBeInTheDocument();
  });

  it("asks for a fresh count each time", async () => {
    let count = 5;
    stubServer(plugin(enricher), () => count);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDiscover(queryClient);
    await install(user);
    const first = await screen.findByRole("dialog", {
      name: "Enrich your 5 existing books?",
    });
    await user.click(within(first).getByRole("button", { name: "Not now" }));
    await flush();

    // The same session installs another enricher after more books arrived.
    count = 9;
    await install(user);

    expect(
      await screen.findByRole("dialog", {
        name: "Enrich your 9 existing books?",
      }),
    ).toBeInTheDocument();
  });

  it("doesn't prompt for a plugin that isn't a metadata enricher", async () => {
    stubServer(plugin(converter), 50);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDiscover();
    await install(user);

    expect(
      screen.queryByRole("dialog", { name: /enrich your/i }),
    ).not.toBeInTheDocument();
  });

  it("doesn't prompt a role that can't start a scan", async () => {
    setAuth({ permissions: ["config:read", "config:write", "books:read"] });
    stubServer(plugin(enricher), 50);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderDiscover();
    await install(user);

    expect(
      screen.queryByRole("dialog", { name: /enrich your/i }),
    ).not.toBeInTheDocument();
  });

  it("also prompts after installing from the plugin's page", async () => {
    stubServer(plugin(enricher), 3);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <MemoryRouter initialEntries={["/admin/plugins/shisho/goodreads"]}>
          <Routes>
            <Route
              element={<PluginDetail />}
              path="/admin/plugins/:scope/:id"
            />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    await install(user);

    expect(
      await screen.findByRole("dialog", {
        name: "Enrich your 3 existing books?",
      }),
    ).toBeInTheDocument();
  });
});
