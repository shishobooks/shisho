import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { MultiSelectCombobox } from "./MultiSelectCombobox";

describe("MultiSelectCombobox", () => {
  it("renders chip status badges when status prop provided", () => {
    render(
      <MultiSelectCombobox
        isLoading={false}
        label="Genre"
        onChange={vi.fn()}
        onSearch={vi.fn()}
        options={[]}
        searchValue=""
        status={(v) => (v === "Sci-Fi" ? "new" : "unchanged")}
        values={["Sci-Fi", "Fantasy"]}
      />,
    );

    const sci = screen.getByText("Sci-Fi").closest("[data-testid='ms-chip']");
    expect(
      sci?.querySelector("[data-testid='ms-status-badge']"),
    ).toHaveTextContent(/new/i);
  });

  it("renders no status badges when status prop omitted (existing behavior)", () => {
    render(
      <MultiSelectCombobox
        isLoading={false}
        label="Genre"
        onChange={vi.fn()}
        onSearch={vi.fn()}
        options={[]}
        searchValue=""
        values={["Sci-Fi"]}
      />,
    );

    expect(screen.queryByTestId("ms-status-badge")).not.toBeInTheDocument();
  });

  it("renders removed entries as strikethrough with undo, restoring on click", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();

    render(
      <MultiSelectCombobox
        isLoading={false}
        label="Genre"
        onChange={onChange}
        onSearch={vi.fn()}
        options={[]}
        removed={["Horror"]}
        searchValue=""
        values={["Sci-Fi"]}
      />,
    );

    const horror = screen.getByText("Horror");
    expect(horror.className).toMatch(/line-through/);
    await user.click(screen.getByLabelText(/Restore Horror/i));
    expect(onChange).toHaveBeenCalledWith(["Sci-Fi", "Horror"]);
  });
});
