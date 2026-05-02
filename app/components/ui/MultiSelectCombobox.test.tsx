import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { MultiSelectCombobox } from "./MultiSelectCombobox";

describe("MultiSelectCombobox", () => {
  it("renders chips without status badges", () => {
    render(
      <MultiSelectCombobox
        isLoading={false}
        label="Genre"
        onChange={vi.fn()}
        onSearch={vi.fn()}
        options={[]}
        searchValue=""
        values={["Sci-Fi", "Fantasy"]}
      />,
    );

    expect(screen.getByText("Sci-Fi")).toBeInTheDocument();
    expect(screen.getByText("Fantasy")).toBeInTheDocument();
    expect(screen.queryByTestId("ms-status-badge")).not.toBeInTheDocument();
  });

  it("removes a chip when X is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();

    render(
      <MultiSelectCombobox
        isLoading={false}
        label="Genre"
        onChange={onChange}
        onSearch={vi.fn()}
        options={[]}
        searchValue=""
        values={["Sci-Fi", "Fantasy"]}
      />,
    );

    await user.click(screen.getByLabelText(/Remove Sci-Fi/i));
    expect(onChange).toHaveBeenCalledWith(["Fantasy"]);
  });
});
