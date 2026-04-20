import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

import { PluginManifestDialog } from "./PluginManifestDialog";

const mockUsePluginManifest = vi.fn();

vi.mock("@/hooks/queries/plugins", () => ({
  usePluginManifest: (
    scope: string,
    id: string,
    opts: { enabled?: boolean },
  ) => {
    return mockUsePluginManifest(scope, id, opts);
  },
}));

const wrap = (ui: React.ReactNode) => (
  <QueryClientProvider
    client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
  >
    {ui}
  </QueryClientProvider>
);

describe("PluginManifestDialog", () => {
  it("renders the manifest JSON when data is provided", () => {
    const manifest = { name: "my-plugin", version: "1.0.0" };
    mockUsePluginManifest.mockReturnValue({
      data: manifest,
      isLoading: false,
      error: null,
    });

    render(
      wrap(
        <PluginManifestDialog
          id="my-plugin"
          onOpenChange={vi.fn()}
          open={true}
          scope="local"
        />,
      ),
    );

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText(/my-plugin/)).toBeInTheDocument();
    expect(screen.getByText(/1\.0\.0/)).toBeInTheDocument();
  });

  it("does not call usePluginManifest with enabled=true when open is false", () => {
    mockUsePluginManifest.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    });

    render(
      wrap(
        <PluginManifestDialog
          id="my-plugin"
          onOpenChange={vi.fn()}
          open={false}
          scope="local"
        />,
      ),
    );

    // When open=false, the hook should be called with enabled:false
    expect(mockUsePluginManifest).toHaveBeenCalledWith(
      "local",
      "my-plugin",
      expect.objectContaining({ enabled: false }),
    );
  });

  it("shows a loading state when isLoading is true", () => {
    mockUsePluginManifest.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    });

    render(
      wrap(
        <PluginManifestDialog
          id="my-plugin"
          onOpenChange={vi.fn()}
          open={true}
          scope="local"
        />,
      ),
    );

    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });

  it("shows an error message when error is set", () => {
    mockUsePluginManifest.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Failed to fetch manifest"),
    });

    render(
      wrap(
        <PluginManifestDialog
          id="my-plugin"
          onOpenChange={vi.fn()}
          open={true}
          scope="local"
        />,
      ),
    );

    expect(screen.getByText(/Failed to fetch manifest/)).toBeInTheDocument();
  });
});
