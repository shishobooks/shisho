import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { FileTypeCBZ } from "@/types";
import type { PageSourceFile } from "@/utils/pageUrl";

import PagePicker from "./PagePicker";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const file: PageSourceFile = {
  id: 1,
  updated_at: "2024-01-01T00:00:00Z",
  file_type: FileTypeCBZ,
};

describe("PagePicker navigation", () => {
  it("offers one labeled button per direction", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(
      <PagePicker
        currentPage={1}
        file={file}
        onOpenChange={vi.fn()}
        onSelect={vi.fn()}
        open
        pageCount={3}
      />,
    );

    // The name belongs to the visible chevron button; the pointer-only tap
    // zones over it are hidden, so each name matches once.
    const previous = screen.getByRole("button", { name: "Previous page" });
    const next = screen.getByRole("button", { name: "Next page" });
    expect(previous.querySelector("svg")).not.toBeNull();
    expect(next.querySelector("svg")).not.toBeNull();
    expect(screen.getByText("Page 2 of 3")).toBeInTheDocument();

    await user.click(next);
    expect(screen.getByText("Page 3 of 3")).toBeInTheDocument();
    expect(next).toBeDisabled();

    await user.click(previous);
    expect(screen.getByText("Page 2 of 3")).toBeInTheDocument();
  });
});
