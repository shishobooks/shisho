import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useAuth } from "@/hooks/useAuth";

import DemoBanner from "./DemoBanner";

vi.mock("@/hooks/useAuth", () => ({
  useAuth: vi.fn(),
}));

describe("DemoBanner", () => {
  beforeEach(() => {
    vi.mocked(useAuth).mockReturnValue({ demoMode: true } as never);
  });

  it("shows the Demo Mode restrictions and useful links", () => {
    render(<DemoBanner />);

    expect(screen.getByRole("complementary")).toHaveTextContent(
      "Read-only demo. Edits and downloads are disabled.",
    );
    expect(
      screen.getByRole("link", { name: "Install Shisho" }),
    ).toHaveAttribute(
      "href",
      "https://www.shishobooks.com/docs/getting-started",
    );
    expect(screen.getByRole("link", { name: "GitHub" })).toHaveAttribute(
      "href",
      "https://github.com/shishobooks/shisho",
    );
    expect(
      screen.getByRole("link", { name: "About this library" }),
    ).toHaveAttribute("href", "https://www.shishobooks.com/docs/demo");
  });

  it("does not hide any part of the notice at narrow widths", () => {
    render(<DemoBanner />);

    // jsdom applies no CSS, so check for the utility that hid the second
    // sentence below md. Real-width layout is covered by a browser check.
    let node: HTMLElement | null = screen.getByText(
      "Edits and downloads are disabled.",
    );
    const notice = node.closest("p");
    while (node && node !== notice?.parentElement) {
      expect(node.className).not.toMatch(/(^|\s)hidden(\s|$)/);
      node = node.parentElement;
    }
  });

  it("makes installing the only filled action", () => {
    render(<DemoBanner />);

    const filled = screen
      .getAllByRole("link")
      .filter((link) => link.classList.contains("bg-primary"))
      .map((link) => link.getAttribute("aria-label"));

    expect(filled).toEqual(["Install Shisho"]);
  });

  it("is hidden outside Demo Mode", () => {
    vi.mocked(useAuth).mockReturnValue({ demoMode: false } as never);

    const { container } = render(<DemoBanner />);

    expect(container).toBeEmptyDOMElement();
  });
});
