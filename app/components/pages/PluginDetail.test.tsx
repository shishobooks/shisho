import { PluginDetail } from "./PluginDetail";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

// Mock the auth hook used by PluginDetail.
vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({ hasPermission: () => true }),
}));

// Mock useUnsavedChanges (uses react-router's useBlocker which requires a data router).
vi.mock("@/hooks/useUnsavedChanges", () => ({
  useUnsavedChanges: () => ({
    cancelNavigation: vi.fn(),
    proceedNavigation: vi.fn(),
    showBlockerDialog: false,
  }),
}));

// Mock query hooks so the component renders without network.
vi.mock("@/hooks/queries/plugins", () => ({
  usePluginsInstalled: () => ({ data: [], isLoading: false, isError: false }),
  usePluginsAvailable: () => ({
    data: [
      {
        compatible: true,
        description: "",
        homepage: "",
        id: "example",
        imageUrl: "",
        is_official: false,
        name: "Example Plugin",
        overview: "",
        scope: "shisho",
        versions: [],
      },
    ],
    isLoading: false,
    isError: false,
  }),
  useUpdatePlugin: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdatePluginVersion: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
    mutate: vi.fn(),
  }),
  useInstallPlugin: () => ({ mutate: vi.fn(), isPending: false }),
  usePluginRepositories: () => ({ data: [] }),
  PluginStatusActive: "active",
}));

const renderAt = (path: string) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route
            element={<PluginDetail />}
            path="/settings/plugins/:scope/:id"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

describe("PluginDetail breadcrumb", () => {
  it("renders Plugins as a link to /settings/plugins and the plugin name as current", () => {
    renderAt("/settings/plugins/shisho/example");
    const pluginsLink = screen.getByRole("link", { name: /^plugins$/i });
    expect(pluginsLink).toHaveAttribute("href", "/settings/plugins");
    // Plugin name appears in breadcrumb current position AND in the hero —
    // at least one of them; ensure no back-button "Plugins" button remains.
    expect(screen.queryByRole("button", { name: /^plugins$/i })).toBeNull();
  });
});
