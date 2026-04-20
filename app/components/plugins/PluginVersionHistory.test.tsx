import { PluginVersionHistory } from "./PluginVersionHistory";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import React from "react";
import { describe, expect, it } from "vitest";

import type { AvailablePlugin, PluginVersion } from "@/hooks/queries/plugins";

const makeVersion = (
  v: string,
  overrides: Partial<PluginVersion> = {},
): PluginVersion => ({
  capabilities: undefined,
  changelog: "",
  compatible: true,
  downloadUrl: "https://example.com/plugin.zip",
  manifestVersion: 1,
  minShishoVersion: "0.0.0",
  releaseDate: "",
  sha256: "deadbeef",
  version: v,
  ...overrides,
});

const makeAvailable = (versions: PluginVersion[]): AvailablePlugin => ({
  compatible: true,
  description: "",
  homepage: "",
  id: "p",
  imageUrl: "",
  is_official: false,
  name: "Plugin",
  overview: "",
  scope: "shisho",
  versions,
});

const renderWithClient = (ui: React.ReactElement) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
};

describe("PluginVersionHistory", () => {
  it("renders a View release link pointing at the version's releaseUrl", () => {
    const available = makeAvailable([
      makeVersion("1.0.0", {
        releaseUrl: "https://github.com/me/repo/releases/tag/v1.0.0",
      }),
    ]);
    renderWithClient(<PluginVersionHistory available={available} />);
    const link = screen.getByRole("link", { name: /view release/i });
    expect(link).toHaveAttribute(
      "href",
      "https://github.com/me/repo/releases/tag/v1.0.0",
    );
  });

  it("renders releaseUrl verbatim regardless of host", () => {
    const available = makeAvailable([
      makeVersion("1.0.0", {
        releaseUrl: "https://gitlab.example.com/me/repo/-/tags/v1.0.0",
      }),
    ]);
    renderWithClient(<PluginVersionHistory available={available} />);
    const link = screen.getByRole("link", { name: /view release/i });
    expect(link).toHaveAttribute(
      "href",
      "https://gitlab.example.com/me/repo/-/tags/v1.0.0",
    );
  });

  it("omits the release link when releaseUrl is absent", () => {
    const available = makeAvailable([makeVersion("1.0.0")]);
    renderWithClient(<PluginVersionHistory available={available} />);
    expect(screen.queryByRole("link", { name: /view release/i })).toBeNull();
  });
});
