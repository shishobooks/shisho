import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import FadedText from "./FadedText";

// jsdom lays nothing out, so each test says how tall the text is.
const textHeight = (px: number) =>
  vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(px);

afterEach(() => {
  vi.restoreAllMocks();
});

describe("FadedText", () => {
  it("shows short text whole, with no button", () => {
    textHeight(80);
    render(<FadedText collapsedPx={96} text="A short description." />);

    expect(screen.getByText("A short description.")).not.toHaveStyle({
      maxHeight: "96px",
    });
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("shows text just past the limit whole rather than behind a button", () => {
    textHeight(110);
    render(<FadedText collapsedPx={96} text="Nearly fits." />);

    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("fades long text out without adding an ellipsis, and expands it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    textHeight(400);
    render(<FadedText collapsedPx={96} text="A long description" />);

    const text = screen.getByText("A long description");
    expect(text).toHaveStyle({ maxHeight: "96px" });
    // The cut is a fade, so text that ends in "..." in the data still reads
    // as the data's own.
    expect(text.textContent).toBe("A long description");

    const more = screen.getByRole("button", { name: "Show more" });
    expect(more).toHaveAttribute("aria-expanded", "false");
    await user.click(more);

    expect(text).not.toHaveStyle({ maxHeight: "96px" });
    expect(screen.getByRole("button", { name: "Show less" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });
});
