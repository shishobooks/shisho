import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";

import type { AvailablePlugin } from "@/hooks/queries/plugins";
import {
  PluginStatusActive,
  PluginStatusMalfunctioned,
  PluginStatusNotSupported,
  type Plugin,
} from "@/types/generated/models";

import { PluginDetailHero } from "./PluginDetailHero";

const wrap = (ui: ReactNode) => (
  <QueryClientProvider
    client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
  >
    {ui}
  </QueryClientProvider>
);

const baseAvailable: AvailablePlugin = {
  compatible: true,
  description: "",
  homepage: "",
  id: "p",
  imageUrl: "",
  is_official: false,
  name: "Plugin",
  overview: "",
  scope: "shisho",
  versions: [],
};

const baseInstalled: Plugin = {
  auto_update: false,
  id: "p",
  installed_at: "2026-01-01T00:00:00Z",
  name: "Plugin",
  scope: "shisho",
  status: PluginStatusActive,
  version: "1.0.0",
};

describe("PluginDetailHero", () => {
  it("renders the official badge next to the repo name", () => {
    render(
      <PluginDetailHero
        available={{ ...baseAvailable, is_official: true }}
        canWrite={false}
        id="p"
        repoName="Official Shisho Plugins"
        scope="shisho"
      />,
    );
    expect(screen.getByLabelText(/official plugin/i)).toBeInTheDocument();
  });

  it("does not render the official badge for community plugins", () => {
    render(
      <PluginDetailHero
        available={baseAvailable}
        canWrite={false}
        id="p"
        repoName="Community Plugins"
        scope="shisho"
      />,
    );
    expect(screen.queryByLabelText(/official plugin/i)).toBeNull();
  });

  it("renders the load-error alert when status is Malfunctioned", () => {
    render(
      wrap(
        <PluginDetailHero
          canWrite={false}
          id="p"
          installed={{
            ...baseInstalled,
            load_error: "failed to load plugin: invalid field",
            status: PluginStatusMalfunctioned,
          }}
          scope="shisho"
        />,
      ),
    );
    expect(screen.getByText(/plugin failed to load/i)).toBeInTheDocument();
    expect(
      screen.getByText(/failed to load plugin: invalid field/i),
    ).toBeInTheDocument();
  });

  it("renders the Incompatible alert when status is NotSupported without a load_error", () => {
    render(
      wrap(
        <PluginDetailHero
          canWrite={false}
          id="p"
          installed={{
            ...baseInstalled,
            status: PluginStatusNotSupported,
          }}
          scope="shisho"
        />,
      ),
    );
    expect(
      screen.getByText(/not compatible with this shisho version/i),
    ).toBeInTheDocument();
  });

  it("does not render the alert for an Active plugin", () => {
    render(
      wrap(
        <PluginDetailHero
          canWrite={false}
          id="p"
          installed={baseInstalled}
          scope="shisho"
        />,
      ),
    );
    expect(screen.queryByText(/failed to load/i)).toBeNull();
    expect(screen.queryByText(/not compatible/i)).toBeNull();
  });
});
