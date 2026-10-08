import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import LogViewer, { type LogViewerEntry } from "./LogViewer";

const entry = (overrides: Partial<LogViewerEntry> = {}): LogViewerEntry => ({
  id: 1,
  level: "info",
  timestamp: "2024-01-01T00:00:00Z",
  message: "scan started",
  ...overrides,
});

describe("LogViewer", () => {
  it("expands a log entry from the keyboard and announces its state", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<LogViewer entries={[entry({ error: "disk full" })]} />);

    const toggle = screen.getByRole("button", { name: "Details" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText(/error:/)).toBeNull();

    await user.tab();
    expect(toggle).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText(/error:/)).toBeInTheDocument();

    await user.keyboard(" ");
    expect(toggle).toHaveAttribute("aria-expanded", "false");
  });

  it("keeps the message as text and still toggles on a row click", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<LogViewer entries={[entry({ error: "disk full" })]} />);

    // The message is not inside the button, so it can be selected.
    expect(screen.getByText("scan started").closest("button")).toBeNull();
    await user.click(screen.getByText("scan started"));
    expect(screen.getByText(/error:/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Details" }));
    expect(screen.queryByText(/error:/)).toBeNull();
  });

  it("leaves an entry with nothing to expand as plain text", () => {
    render(<LogViewer entries={[entry()]} />);

    expect(screen.getByText("scan started")).toBeInTheDocument();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
