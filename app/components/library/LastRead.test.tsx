import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { formatDateTime } from "@/utils/format";

import LastRead from "./LastRead";

const hoverFor = async (text: string) => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  await user.hover(screen.getByText(text));
  act(() => vi.advanceTimersByTime(800));
  return screen.findByRole("tooltip");
};

describe("LastRead", () => {
  beforeEach(() => vi.setSystemTime(new Date(2026, 10, 20, 12)));

  it("shows the full time in a tooltip within the first day", async () => {
    const instant = new Date(2026, 10, 20, 8).toISOString();
    render(<LastRead lastReadAt={instant} />);

    expect(await hoverFor("Last read 4 hours ago")).toHaveTextContent(
      formatDateTime(instant),
    );
  });

  it("shows the full time in a tooltip after that", async () => {
    const instant = new Date(2026, 10, 15, 12).toISOString();
    render(<LastRead lastReadAt={instant} />);

    expect(await hoverFor("Last read 5 days ago")).toHaveTextContent(
      formatDateTime(instant),
    );
  });

  it("gives the full time to screen readers and machines too", () => {
    const instant = new Date(2026, 10, 15, 12).toISOString();
    render(<LastRead lastReadAt={instant} />);

    expect(
      screen.getByText("Last read 5 days ago").closest("time"),
    ).toHaveAttribute("datetime", instant);
    expect(screen.getByText(formatDateTime(instant))).toHaveClass("sr-only");
  });

  it("uses the app's tooltip, not the browser's", () => {
    render(<LastRead lastReadAt={new Date(2026, 10, 15, 12).toISOString()} />);

    expect(screen.getByText("Last read 5 days ago")).not.toHaveAttribute(
      "title",
    );
  });
});
