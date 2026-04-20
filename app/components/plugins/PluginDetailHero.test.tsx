import { PluginDetailHero } from "./PluginDetailHero";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { AvailablePlugin } from "@/hooks/queries/plugins";

const baseAvailable: AvailablePlugin = {
  author: "Official Shisho Plugins",
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
});
