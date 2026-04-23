import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ExtractSubtitleButton } from "./ExtractSubtitleButton";

describe("ExtractSubtitleButton", () => {
  it("renders nothing when title has no colon", () => {
    const onExtract = vi.fn();
    const { container } = render(
      <ExtractSubtitleButton onExtract={onExtract} title="Why We Sleep" />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when title has colon but split is empty on one side", () => {
    const onExtract = vi.fn();
    const { container } = render(
      <ExtractSubtitleButton onExtract={onExtract} title="Title:" />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("renders button when title has a splittable colon", () => {
    const onExtract = vi.fn();
    render(
      <ExtractSubtitleButton
        onExtract={onExtract}
        title="Why We Sleep: Unlocking the Power of Sleep and Dreams"
      />,
    );
    expect(
      screen.getByRole("button", { name: /extract subtitle/i }),
    ).toBeInTheDocument();
  });

  it("fires onExtract with the split values on click", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onExtract = vi.fn();
    render(
      <ExtractSubtitleButton
        onExtract={onExtract}
        title="Star Wars: Thrawn: Alliances"
      />,
    );
    await user.click(screen.getByRole("button", { name: /extract subtitle/i }));
    expect(onExtract).toHaveBeenCalledWith("Star Wars", "Thrawn: Alliances");
  });
});
