import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { MultiSelectCombobox } from "./MultiSelectCombobox";

function makeHook(items: string[], isLoading = false) {
  return () => ({ data: items, isLoading });
}

describe("MultiSelectCombobox", () => {
  it("renders chips for selected values", () => {
    render(
      <MultiSelectCombobox
        getOptionLabel={(s) => s}
        hook={makeHook([])}
        label="Genre"
        onChange={vi.fn()}
        values={["Sci-Fi", "Fantasy"]}
      />,
    );

    expect(screen.getByText("Sci-Fi")).toBeInTheDocument();
    expect(screen.getByText("Fantasy")).toBeInTheDocument();
  });

  it("removes a chip when X is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();

    render(
      <MultiSelectCombobox
        getOptionLabel={(s) => s}
        hook={makeHook([])}
        label="Genre"
        onChange={onChange}
        values={["Sci-Fi", "Fantasy"]}
      />,
    );

    await user.click(screen.getByLabelText(/Remove Sci-Fi/i));
    expect(onChange).toHaveBeenCalledWith(["Fantasy"]);
  });

  it("shows 'In your library' heading when options exist", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    render(
      <MultiSelectCombobox
        getOptionLabel={(s) => s}
        hook={makeHook(["Fantasy", "Horror"])}
        label="Genre"
        onChange={vi.fn()}
        values={[]}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    expect(screen.getByText("In your library")).toBeInTheDocument();
  });

  it("selects an option and calls onChange", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();

    render(
      <MultiSelectCombobox
        getOptionLabel={(s) => s}
        hook={makeHook(["Fantasy", "Horror"])}
        label="Genre"
        onChange={onChange}
        values={[]}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByText("Fantasy"));
    expect(onChange).toHaveBeenCalledWith(["Fantasy"]);
  });

  it("excludes already-selected values from dropdown", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    render(
      <MultiSelectCombobox
        getOptionLabel={(s) => s}
        hook={makeHook(["Fantasy", "Horror"])}
        label="Genre"
        onChange={vi.fn()}
        values={["Fantasy"]}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    expect(
      screen.queryByRole("option", { name: "Fantasy" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("Horror")).toBeInTheDocument();
  });

  it("shows Create option for unmatched search text", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    render(
      <MultiSelectCombobox
        getOptionLabel={(s) => s}
        hook={makeHook([])}
        label="Genre"
        onChange={vi.fn()}
        values={[]}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.type(screen.getByPlaceholderText(/Search genre/i), "Thriller");
    expect(screen.getByText(/Create new genre "Thriller"/)).toBeInTheDocument();
  });
});
