import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import Logo from "./Logo";

describe("Logo", () => {
  it("renders the shelf mark with per-instance gradient ids", () => {
    const { container } = render(
      <>
        <Logo />
        <Logo size="lg" />
      </>,
    );

    const gradients = Array.from(container.querySelectorAll("linearGradient"));
    const ids = gradients.map((gradient) => gradient.id);

    // Two logos, each with a contact-shadow and shelf-shade gradient.
    expect(ids).toHaveLength(4);
    expect(new Set(ids).size).toBe(4);
    for (const id of ids) {
      expect(id).toMatch(/^[a-zA-Z0-9_-]+$/);
    }

    // Every gradient fill points at a gradient defined in the document.
    const gradientFills = Array.from(container.querySelectorAll("rect"))
      .map((rect) => rect.getAttribute("fill"))
      .filter((fill): fill is string => fill?.startsWith("url(#") ?? false);
    expect(gradientFills.length).toBeGreaterThan(0);
    for (const fill of gradientFills) {
      expect(ids).toContain(fill.slice("url(#".length, -1));
    }
  });
});
