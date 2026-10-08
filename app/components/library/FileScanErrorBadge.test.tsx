import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import FileScanErrorBadge from "./FileScanErrorBadge";

const file = { scan_error: "zip: not a valid zip file" };

describe("FileScanErrorBadge", () => {
  it("opens the scan error tooltip from the keyboard", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<FileScanErrorBadge file={file} />);

    await user.tab();
    expect(screen.getByRole("button", { name: "Unreadable" })).toHaveFocus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "zip: not a valid zip file",
    );
  });

  it("takes no focus inside a link, and gives screen readers the error as text", () => {
    render(<FileScanErrorBadge file={file} interactive={false} />);

    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(
      screen.getByText(/zip: not a valid zip file/, { selector: ".sr-only" }),
    ).toBeInTheDocument();
  });
});
