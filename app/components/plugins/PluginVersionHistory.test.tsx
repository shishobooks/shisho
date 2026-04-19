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

const makeAvailable = (
  versions: PluginVersion[],
  homepage = "",
): AvailablePlugin => ({
  author: "",
  compatible: true,
  description: "",
  homepage,
  id: "p",
  imageUrl: "",
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
  it("ignores non-github homepages for diff URL", () => {
    const available = makeAvailable(
      [makeVersion("1.0.0")],
      "https://github.com.evil.com/foo",
    );
    renderWithClient(<PluginVersionHistory available={available} />);
    expect(
      screen.queryByRole("link", { name: /view release on github/i }),
    ).toBeNull();
  });

  it("builds a valid release URL for a real github homepage", () => {
    const available = makeAvailable(
      [makeVersion("1.0.0")],
      "https://github.com/me/repo",
    );
    renderWithClient(<PluginVersionHistory available={available} />);
    const link = screen.getByRole("link", {
      name: /view release on github/i,
    });
    expect(link).toHaveAttribute(
      "href",
      "https://github.com/me/repo/releases/tag/v1.0.0",
    );
  });
});
