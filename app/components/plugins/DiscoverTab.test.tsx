import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import type { AvailablePlugin } from "@/hooks/queries/plugins";
import { PluginStatusActive, type Plugin } from "@/types/generated/models";

import { filterPlugins } from "./discoverFilters";
import { DiscoverTab } from "./DiscoverTab";

// --- Mocks ---

const mockInstallMutate = vi.fn();
const mockUpdateMutate = vi.fn();

vi.mock("@/hooks/queries/plugins", () => ({
  useInstallPlugin: () => ({ isPending: false, mutate: mockInstallMutate }),
  usePluginRepositories: () => ({ data: mockRepos }),
  usePluginsAvailable: () => ({
    data: mockAvailable,
    isLoading: false,
    error: null,
  }),
  usePluginsInstalled: () => ({ data: mockInstalled }),
  useUpdatePluginVersion: () => ({
    isPending: false,
    mutate: mockUpdateMutate,
  }),
}));

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

// --- Test data ---

const makePlugin = (
  overrides: Partial<AvailablePlugin> = {},
): AvailablePlugin => ({
  compatible: true,
  description: "A helpful plugin",
  homepage: "",
  id: "my-plugin",
  imageUrl: "",
  is_official: false,
  name: "My Plugin",
  overview: "",
  scope: "shisho",
  versions: [
    {
      capabilities: { metadataEnricher: { fileTypes: ["epub"] } },
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
  ...overrides,
});

const toInstalled = (
  p: AvailablePlugin,
  overrides: Partial<Plugin> = {},
): Plugin => ({
  auto_update: true,
  id: p.id,
  installed_at: "2024-01-01T00:00:00Z",
  name: p.name,
  scope: p.scope,
  status: PluginStatusActive,
  version: p.versions[0]?.version ?? "1.0.0",
  ...overrides,
});

let mockAvailable: AvailablePlugin[] = [];
let mockInstalled: Plugin[] = [];
const mockRepos: unknown[] = [];

const wrap = (ui: React.ReactNode) => (
  <QueryClientProvider
    client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
  >
    <MemoryRouter>{ui}</MemoryRouter>
  </QueryClientProvider>
);

describe("DiscoverTab", () => {
  it("renders Install button for compatible, uninstalled plugin when canWrite=true", () => {
    mockAvailable = [makePlugin()];
    mockInstalled = [];
    render(wrap(<DiscoverTab canWrite />));
    expect(
      screen.getByRole("button", { name: /install/i }),
    ).toBeInTheDocument();
  });

  it("renders disabled Installed button for already-installed plugin", () => {
    const p = makePlugin();
    mockAvailable = [p];
    mockInstalled = [toInstalled(p)];
    render(wrap(<DiscoverTab canWrite />));
    const btn = screen.getByRole("button", { name: /installed/i });
    expect(btn).toBeDisabled();
  });

  it("renders Update button and installed version when an update is available", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const p = makePlugin({
      versions: [
        {
          capabilities: { metadataEnricher: { fileTypes: ["epub"] } },
          changelog: "",
          compatible: true,
          downloadUrl: "",
          manifestVersion: 1,
          minShishoVersion: "0.1.0",
          releaseDate: "2024-01-01",
          sha256: "",
          version: "2.0.0",
        },
      ],
    });
    mockAvailable = [p];
    mockInstalled = [
      toInstalled(p, { update_available_version: "2.0.0", version: "1.0.0" }),
    ];
    render(wrap(<DiscoverTab canWrite />));

    // Row should show the installed version, not the repo's latest
    expect(screen.getByText("v1.0.0")).toBeInTheDocument();
    expect(screen.getByText(/Update 2\.0\.0/i)).toBeInTheDocument();

    // Update button should trigger the update mutation
    const btn = screen.getByRole("button", { name: /^update$/i });
    await user.click(btn);
    expect(mockUpdateMutate).toHaveBeenCalledWith(
      { id: p.id, scope: p.scope },
      expect.any(Object),
    );
  });

  it("renders disabled Incompatible button for incompatible plugin", () => {
    mockAvailable = [makePlugin({ compatible: false })];
    mockInstalled = [];
    render(wrap(<DiscoverTab canWrite />));
    const btn = screen.getByRole("button", { name: /incompatible/i });
    expect(btn).toBeDisabled();
  });

  it("shows no action buttons when canWrite=false", () => {
    mockAvailable = [makePlugin()];
    mockInstalled = [];
    render(wrap(<DiscoverTab canWrite={false} />));
    expect(screen.queryByRole("button", { name: /install/i })).toBeNull();
  });

  it("filters plugins by search query (name match)", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mockAvailable = [
      makePlugin({ id: "alpha", name: "Alpha Plugin" }),
      makePlugin({ id: "beta", name: "Beta Plugin" }),
    ];
    mockInstalled = [];
    render(wrap(<DiscoverTab canWrite />));
    await user.type(screen.getByPlaceholderText(/search/i), "alpha");
    expect(screen.getByText("Alpha Plugin")).toBeInTheDocument();
    expect(screen.queryByText("Beta Plugin")).toBeNull();
  });

  it("filters plugins by search query (description match)", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mockAvailable = [
      makePlugin({
        id: "alpha",
        name: "Plugin A",
        description: "converts things",
      }),
      makePlugin({
        id: "beta",
        name: "Plugin B",
        description: "enriches metadata",
      }),
    ];
    mockInstalled = [];
    render(wrap(<DiscoverTab canWrite />));
    await user.type(screen.getByPlaceholderText(/search/i), "converts");
    expect(screen.getByText("Plugin A")).toBeInTheDocument();
    expect(screen.queryByText("Plugin B")).toBeNull();
  });

  it("filterPlugins: handles null/undefined description without crashing", () => {
    const plugins = [
      makePlugin({
        description: undefined as unknown as string,
        id: "a",
        name: "Plugin A",
      }),
      makePlugin({
        description: "matches search term",
        id: "b",
        name: "Plugin B",
      }),
    ];
    const result = filterPlugins(plugins, "matches", "all", "all");
    expect(result.map((p) => p.name)).toEqual(["Plugin B"]);
  });

  it("filterPlugins: filters by source/scope", () => {
    const plugins = [
      makePlugin({ id: "a", name: "Plugin A", scope: "shisho" }),
      makePlugin({ id: "b", name: "Plugin B", scope: "community" }),
    ];
    const result = filterPlugins(plugins, "", "all", "community");
    expect(result.map((p) => p.name)).toEqual(["Plugin B"]);
  });

  it("filterPlugins: shows all plugins when source is 'all'", () => {
    const plugins = [
      makePlugin({ id: "a", name: "Plugin A", scope: "shisho" }),
      makePlugin({ id: "b", name: "Plugin B", scope: "community" }),
    ];
    const result = filterPlugins(plugins, "", "all", "all");
    expect(result).toHaveLength(2);
  });

  it("filterPlugins: filters by capability (metadataEnricher)", () => {
    const enricher = makePlugin({
      id: "enricher",
      name: "Enricher Plugin",
      versions: [
        {
          capabilities: { metadataEnricher: { fileTypes: ["epub"] } },
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
    const converter = makePlugin({
      id: "converter",
      name: "Converter Plugin",
      versions: [
        {
          capabilities: {
            inputConverter: { sourceTypes: ["epub"], targetType: "pdf" },
          },
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
    const result = filterPlugins(
      [enricher, converter],
      "",
      "metadataEnricher",
      "all",
    );
    expect(result.map((p) => p.name)).toEqual(["Enricher Plugin"]);
  });

  it("filterPlugins: filters by capability (inputConverter)", () => {
    const enricher = makePlugin({
      id: "enricher",
      name: "Enricher Plugin",
    });
    const converter = makePlugin({
      id: "converter",
      name: "Converter Plugin",
      versions: [
        {
          capabilities: {
            inputConverter: { sourceTypes: ["epub"], targetType: "pdf" },
          },
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
    const result = filterPlugins(
      [enricher, converter],
      "",
      "inputConverter",
      "all",
    );
    expect(result.map((p) => p.name)).toEqual(["Converter Plugin"]);
  });

  it("opens the CapabilitiesWarning dialog when Install is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mockAvailable = [makePlugin()];
    mockInstalled = [];
    render(wrap(<DiscoverTab canWrite />));
    await user.click(screen.getByRole("button", { name: /install/i }));
    // CapabilitiesWarning dialog should appear with the plugin name in a heading
    expect(
      screen.getByRole("heading", { name: /install my plugin/i }),
    ).toBeInTheDocument();
  });
});
