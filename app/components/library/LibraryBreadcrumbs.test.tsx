import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";

import LibraryBreadcrumbs from "./LibraryBreadcrumbs";

const renderCrumbs = (libraryName?: string) =>
  render(
    <MemoryRouter>
      <LibraryBreadcrumbs
        items={[{ label: "Test Book" }]}
        libraryId="1"
        libraryName={libraryName}
      />
    </MemoryRouter>,
  );

describe("LibraryBreadcrumbs", () => {
  it("links the library by name", () => {
    renderCrumbs("Fiction");

    expect(screen.getByRole("link", { name: "Fiction" })).toHaveAttribute(
      "href",
      "/libraries/1",
    );
    expect(screen.getByText("Test Book")).toBeInTheDocument();
  });

  it("shows a placeholder while the name loads", () => {
    renderCrumbs();

    expect(screen.getByRole("link", { name: "Library" })).toBeInTheDocument();
  });
});
