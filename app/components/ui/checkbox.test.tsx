import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Checkbox } from "./checkbox";

describe("Checkbox", () => {
  it("renders unchecked state", () => {
    render(<Checkbox aria-label="off" checked={false} />);
    expect(screen.getByRole("checkbox", { name: "off" })).toHaveAttribute(
      "data-state",
      "unchecked",
    );
  });

  it("renders checked state with check icon", () => {
    render(<Checkbox aria-label="on" checked={true} />);
    const checkbox = screen.getByRole("checkbox", { name: "on" });
    expect(checkbox).toHaveAttribute("data-state", "checked");
    expect(
      checkbox.querySelector('[data-slot="checkbox-indicator"]'),
    ).not.toBeNull();
  });

  it("renders indeterminate state with horizontal bar", () => {
    render(<Checkbox aria-label="mixed" checked="indeterminate" />);
    const checkbox = screen.getByRole("checkbox", { name: "mixed" });
    expect(checkbox).toHaveAttribute("data-state", "indeterminate");
    const indicator = checkbox.querySelector(
      '[data-slot="checkbox-indicator"]',
    );
    expect(indicator).not.toBeNull();
    expect(
      indicator?.querySelector('[data-slot="checkbox-indeterminate-bar"]'),
    ).not.toBeNull();
  });
});
