import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";

import { PluginStatusActive, type Plugin } from "@/types/generated/models";

import { PluginHeroActions } from "./PluginHeroActions";

const mockReloadMutateAsync = vi.fn();

vi.mock("@/hooks/queries/plugins", () => ({
  usePluginManifest: () => ({
    data: undefined,
    error: null,
    isLoading: false,
  }),
  useReloadPlugin: () => ({
    isPending: false,
    mutateAsync: mockReloadMutateAsync,
  }),
}));

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

const makePlugin = (overrides: Partial<Plugin> = {}): Plugin => ({
  auto_update: false,
  id: "my-plugin",
  installed_at: "2024-01-01T00:00:00Z",
  name: "My Plugin",
  scope: "local",
  status: PluginStatusActive,
  version: "1.0.0",
  ...overrides,
});

const wrap = (ui: React.ReactNode) => (
  <QueryClientProvider
    client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
  >
    {ui}
  </QueryClientProvider>
);

describe("PluginHeroActions", () => {
  it("shows reload button only for local-scope plugins when canWrite", () => {
    render(
      wrap(
        <PluginHeroActions
          canWrite={true}
          plugin={makePlugin({ scope: "local" })}
        />,
      ),
    );
    expect(
      screen.getByRole("button", { name: /reload plugin from disk/i }),
    ).toBeInTheDocument();
  });

  it("does not show reload button for non-local plugins", () => {
    render(
      wrap(
        <PluginHeroActions
          canWrite={true}
          plugin={makePlugin({ scope: "shisho" })}
        />,
      ),
    );
    expect(
      screen.queryByRole("button", { name: /reload plugin from disk/i }),
    ).toBeNull();
  });

  it("does not show reload button when canWrite is false", () => {
    render(
      wrap(
        <PluginHeroActions
          canWrite={false}
          plugin={makePlugin({ scope: "local" })}
        />,
      ),
    );
    expect(
      screen.queryByRole("button", { name: /reload plugin from disk/i }),
    ).toBeNull();
  });

  it("always shows view manifest button", () => {
    render(
      wrap(
        <PluginHeroActions
          canWrite={true}
          plugin={makePlugin({ scope: "local" })}
        />,
      ),
    );
    expect(
      screen.getByRole("button", { name: /view manifest/i }),
    ).toBeInTheDocument();
  });

  it("shows view manifest button even when canWrite is false", () => {
    render(
      wrap(
        <PluginHeroActions
          canWrite={false}
          plugin={makePlugin({ scope: "local" })}
        />,
      ),
    );
    expect(
      screen.getByRole("button", { name: /view manifest/i }),
    ).toBeInTheDocument();
  });

  it("also shows view manifest for non-local plugins", () => {
    render(
      wrap(
        <PluginHeroActions
          canWrite={true}
          plugin={makePlugin({ scope: "shisho" })}
        />,
      ),
    );
    expect(
      screen.getByRole("button", { name: /view manifest/i }),
    ).toBeInTheDocument();
  });

  it("calls reload mutation when reload button is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mockReloadMutateAsync.mockResolvedValue({});

    render(
      wrap(
        <PluginHeroActions
          canWrite={true}
          plugin={makePlugin({ scope: "local" })}
        />,
      ),
    );

    await user.click(
      screen.getByRole("button", { name: /reload plugin from disk/i }),
    );

    await waitFor(() => {
      expect(mockReloadMutateAsync).toHaveBeenCalledWith({
        id: "my-plugin",
        scope: "local",
      });
    });
    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith(
        "My Plugin reloaded from disk",
      );
    });
  });

  it("opens the manifest dialog when view manifest is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    render(
      wrap(
        <PluginHeroActions
          canWrite={true}
          plugin={makePlugin({ scope: "local" })}
        />,
      ),
    );

    await user.click(screen.getByRole("button", { name: /view manifest/i }));

    await waitFor(() => {
      expect(
        screen.getByRole("dialog", { name: /plugin manifest/i }),
      ).toBeInTheDocument();
    });
  });
});
