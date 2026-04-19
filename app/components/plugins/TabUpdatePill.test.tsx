import { TabUpdatePill } from "./TabUpdatePill";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

describe("TabUpdatePill", () => {
  it("renders nothing when count is 0", () => {
    const { container } = render(<TabUpdatePill count={0} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders the count when > 0", () => {
    render(<TabUpdatePill count={3} />);
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.getByText("3")).toHaveAttribute("role", "img");
  });

  it("sets the plural tooltip for count > 1", () => {
    render(<TabUpdatePill count={3} />);
    expect(
      screen.getByLabelText(/3 plugins have an update available/i),
    ).toBeInTheDocument();
  });

  it("sets the singular tooltip for count === 1", () => {
    render(<TabUpdatePill count={1} />);
    expect(
      screen.getByLabelText(/1 plugin has an update available/i),
    ).toBeInTheDocument();
  });
});
