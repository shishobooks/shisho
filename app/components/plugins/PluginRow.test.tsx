import { PluginRow } from "./PluginRow";
import { render, screen } from "@testing-library/react";
import React from "react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";

const base = {
  scope: "shisho",
  id: "test",
  name: "Test",
  version: "1.0.0",
  author: "Me",
  description: "A test plugin.",
  imageUrl: undefined,
  capabilities: [],
  href: "/settings/plugins/shisho/test",
};

const wrap = (ui: React.ReactNode) => <MemoryRouter>{ui}</MemoryRouter>;

describe("PluginRow", () => {
  it("renders name, version, and author on meta line", () => {
    render(wrap(<PluginRow {...base} />));
    expect(screen.getByText("Test")).toBeInTheDocument();
    expect(screen.getByText(/v1\.0\.0/)).toBeInTheDocument();
    expect(screen.getByText(/by Me/)).toBeInTheDocument();
  });

  it("renders the repo name with a 'from' prefix when provided", () => {
    render(wrap(<PluginRow {...base} repoName="Official Shisho Plugins" />));
    expect(
      screen.getByText(/from Official Shisho Plugins/),
    ).toBeInTheDocument();
  });

  it("renders the Disabled badge when disabled=true", () => {
    render(wrap(<PluginRow {...base} disabled />));
    expect(screen.getByText(/disabled/i)).toBeInTheDocument();
  });

  it("renders capability badges on meta line", () => {
    render(
      wrap(
        <PluginRow
          {...base}
          capabilities={["Metadata enricher", "File parser"]}
        />,
      ),
    );
    expect(screen.getByText("Metadata enricher")).toBeInTheDocument();
    expect(screen.getByText("File parser")).toBeInTheDocument();
  });

  it("renders the Update badge when updateAvailable is set", () => {
    render(wrap(<PluginRow {...base} updateAvailable="1.5.0" />));
    expect(screen.getByText(/update 1\.5\.0/i)).toBeInTheDocument();
  });

  it("links the whole row to href", () => {
    render(wrap(<PluginRow {...base} />));
    expect(screen.getByRole("link")).toHaveAttribute(
      "href",
      "/settings/plugins/shisho/test",
    );
  });

  it("renders the official badge next to the repo name when isOfficial is true", () => {
    render(
      wrap(
        <PluginRow {...base} isOfficial repoName="Official Shisho Plugins" />,
      ),
    );
    // BadgeCheck from lucide renders with this aria-label when we label it.
    expect(screen.getByLabelText(/official plugin/i)).toBeInTheDocument();
  });

  it("does not render the official badge by default", () => {
    render(wrap(<PluginRow {...base} />));
    expect(screen.queryByLabelText(/official plugin/i)).toBeNull();
  });
});
