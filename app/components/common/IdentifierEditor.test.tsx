import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { IdentifierEditor, type IdentifierRow } from "./IdentifierEditor";

const TYPES = [
  { id: "isbn_13", label: "ISBN-13" },
  { id: "asin", label: "ASIN" },
];

describe("IdentifierEditor", () => {
  it("excludes already-present types from the type dropdown", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    render(
      <IdentifierEditor
        identifierTypes={TYPES}
        onChange={vi.fn()}
        value={[{ type: "isbn_13", value: "9780306406157" }]}
      />,
    );

    await user.click(
      screen.getByRole("combobox", { name: /Identifier type/i }),
    );
    const isbn = screen.getByRole("option", { name: /ISBN-13/i });
    expect(isbn).toHaveAttribute("aria-disabled", "true");
  });

  it("blocks Add and shows inline error when validateIdentifier rejects the value", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();

    render(
      <IdentifierEditor
        identifierTypes={TYPES}
        onChange={onChange}
        value={[]}
      />,
    );

    await user.click(
      screen.getByRole("combobox", { name: /Identifier type/i }),
    );
    await user.click(screen.getByRole("option", { name: /ISBN-13/i }));
    await user.type(screen.getByPlaceholderText(/value/i), "not-an-isbn");
    await user.click(screen.getByRole("button", { name: /Add/i }));

    expect(onChange).not.toHaveBeenCalled();
    expect(
      screen.getByText(/Invalid ISBN-13|checksum|valid/i),
    ).toBeInTheDocument();
  });

  it("appends a valid identifier on Add", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();

    render(
      <IdentifierEditor
        identifierTypes={TYPES}
        onChange={onChange}
        value={[]}
      />,
    );

    await user.click(
      screen.getByRole("combobox", { name: /Identifier type/i }),
    );
    await user.click(screen.getByRole("option", { name: /ISBN-13/i }));
    await user.type(screen.getByPlaceholderText(/value/i), "9780306406157");
    await user.click(screen.getByRole("button", { name: /Add/i }));

    expect(onChange).toHaveBeenCalledWith([
      { type: "isbn_13", value: "9780306406157" },
    ]);
  });

  it("removes a row when its delete button is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();

    render(
      <IdentifierEditor
        identifierTypes={TYPES}
        onChange={onChange}
        value={[{ type: "isbn_13", value: "9780306406157" }]}
      />,
    );

    await user.click(screen.getByLabelText(/Remove ISBN-13/i));
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it("clears all rows when Clear all clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    const rows: IdentifierRow[] = [
      { type: "isbn_13", value: "9780306406157" },
      { type: "asin", value: "B0BHRJYNHV" },
    ];

    render(
      <IdentifierEditor
        identifierTypes={TYPES}
        onChange={onChange}
        value={rows}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Clear all/i }));
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it("renders chips without status badges", () => {
    render(
      <IdentifierEditor
        identifierTypes={TYPES}
        onChange={vi.fn()}
        value={[
          { type: "isbn_13", value: "9780306406157" },
          { type: "asin", value: "B0BHRJYNHV" },
        ]}
      />,
    );

    expect(screen.getByText("9780306406157")).toBeInTheDocument();
    expect(
      screen.queryByTestId("identifier-status-badge"),
    ).not.toBeInTheDocument();
  });
});
