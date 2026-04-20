import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import { PluginRow } from "./PluginRow";

const base = {
  scope: "shisho",
  id: "test",
  name: "Test",
  version: "1.0.0",
  description: "A test plugin.",
  imageUrl: undefined,
  capabilities: [],
  href: "/settings/plugins/shisho/test",
};

const wrap = (ui: React.ReactNode) => <MemoryRouter>{ui}</MemoryRouter>;

describe("PluginRow", () => {
  it("renders name and version on meta line", () => {
    render(wrap(<PluginRow {...base} />));
    expect(screen.getByText("Test")).toBeInTheDocument();
    expect(screen.getByText(/v1\.0\.0/)).toBeInTheDocument();
  });

  it("renders the repo name on the meta line when provided", () => {
    render(wrap(<PluginRow {...base} repoName="Official Shisho Plugins" />));
    expect(screen.getByText("Official Shisho Plugins")).toBeInTheDocument();
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

  it("stops action clicks from triggering Link navigation", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const mockAction = vi.fn();

    const LocationDisplay = () => {
      const location = useLocation();
      return <div data-testid="location">{location.pathname}</div>;
    };

    render(
      <MemoryRouter initialEntries={["/start"]}>
        <Routes>
          <Route
            element={
              <>
                <PluginRow
                  {...base}
                  actions={
                    <button onClick={mockAction} type="button">
                      Do
                    </button>
                  }
                />
                <LocationDisplay />
              </>
            }
            path="/start"
          />
          <Route element={<LocationDisplay />} path="*" />
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByTestId("location")).toHaveTextContent("/start");

    await user.click(screen.getByRole("button", { name: "Do" }));

    expect(mockAction).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("location")).toHaveTextContent("/start");
  });
});
