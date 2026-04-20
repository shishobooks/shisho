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
  it("renders the official badge when the available plugin is official", () => {
    render(
      <PluginDetailHero
        available={{ ...baseAvailable, is_official: true }}
        canWrite={false}
        id="p"
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
        scope="shisho"
      />,
    );
    expect(screen.queryByLabelText(/official plugin/i)).toBeNull();
  });
});
