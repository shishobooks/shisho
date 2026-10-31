import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Label } from "./label";

describe("Label", () => {
  it("styles a field's label", () => {
    render(<Label htmlFor="name">Name</Label>);
    expect(screen.getByText("Name")).toHaveClass("text-sm", "font-medium");
  });

  it("leaves a row or card that wraps its control to the caller's styles", () => {
    render(
      <Label className="flex gap-2" variant="unstyled">
        <input type="checkbox" />
        Show hidden files
      </Label>,
    );
    const label = screen.getByText("Show hidden files");
    expect(label).toHaveClass("flex", "gap-2");
    expect(label).not.toHaveClass("font-medium");
    expect(
      screen.getByRole("checkbox", { name: "Show hidden files" }),
    ).toBeInTheDocument();
  });
});
