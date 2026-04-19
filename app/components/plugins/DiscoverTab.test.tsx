import { filterPlugins } from "./discoverFilters";
import { DiscoverTab } from "./DiscoverTab";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import type { AvailablePlugin } from "@/hooks/queries/plugins";

// --- Mocks ---

const mockInstallMutate = vi.fn();

vi.mock("@/hooks/queries/plugins", () => ({
  useInstallPlugin: () => ({ isPending: false, mutate: mockInstallMutate }),
  usePluginsAvailable: () => ({
    data: mockAvailable,
    isLoading: false,
    error: null,
  }),
  usePluginsInstalled: () => ({ data: mockInstalled }),
}));

vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

// --- Test data ---

const makePlugin = (
  overrides: Partial<AvailablePlugin> = {},
): AvailablePlugin => ({
  author: "Test Author",
  compatible: true,
  description: "A helpful plugin",
  homepage: "",
  id: "my-plugin",
  imageUrl: "",
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

let mockAvailable: AvailablePlugin[] = [];
let mockInstalled: AvailablePlugin[] = [];

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
    mockInstalled = [p];
    render(wrap(<DiscoverTab canWrite />));
    const btn = screen.getByRole("button", { name: /installed/i });
    expect(btn).toBeDisabled();
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
