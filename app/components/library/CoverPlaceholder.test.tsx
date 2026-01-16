import CoverPlaceholder from "./CoverPlaceholder";
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

describe("CoverPlaceholder", () => {
  it("renders book variant with correct viewBox", () => {
    render(<CoverPlaceholder variant="book" />);

    const svgs = document.querySelectorAll("svg");
    expect(svgs).toHaveLength(2); // light and dark mode

    svgs.forEach((svg) => {
      expect(svg.getAttribute("viewBox")).toBe("0 0 200 300");
    });
  });

  it("renders audiobook variant with correct viewBox", () => {
    render(<CoverPlaceholder variant="audiobook" />);

    const svgs = document.querySelectorAll("svg");
    expect(svgs).toHaveLength(2);

    svgs.forEach((svg) => {
      expect(svg.getAttribute("viewBox")).toBe("0 0 300 300");
    });
  });

  it("applies custom className", () => {
    const { container } = render(
      <CoverPlaceholder className="custom-class" variant="book" />,
    );

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper.classList.contains("custom-class")).toBe(true);
  });

  it("renders light mode SVG visible by default", () => {
    render(<CoverPlaceholder variant="book" />);

    const svgs = document.querySelectorAll("svg");
    // First SVG is light mode (no hidden class)
    expect(svgs[0].classList.contains("dark:hidden")).toBe(true);
    // Second SVG is dark mode (hidden by default)
    expect(svgs[1].classList.contains("hidden")).toBe(true);
  });
});
