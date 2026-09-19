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

    expect(
      screen.getByText("Read-only demo. Edits and downloads are disabled."),
    ).toBeInTheDocument();
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

  it("is hidden outside Demo Mode", () => {
    vi.mocked(useAuth).mockReturnValue({ demoMode: false } as never);

    const { container } = render(<DemoBanner />);

    expect(container).toBeEmptyDOMElement();
  });
});
