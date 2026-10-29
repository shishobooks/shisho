import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { DateField } from "./date-field";

const user = () => userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

// A labelled field that keeps its own value, as a form would.
const Field = ({
  initial,
  max,
  onChange = () => {},
}: {
  initial: string;
  max?: Date;
  onChange?: (value: string) => void;
}) => {
  const [value, setValue] = useState(initial);
  return (
    <>
      <span id="field-label">Started</span>
      <DateField
        id="field"
        labelId="field-label"
        max={max}
        onChange={(v) => {
          setValue(v);
          onChange(v);
        }}
        placeholder="Unknown"
        value={value}
      />
    </>
  );
};

describe("DateField", () => {
  beforeEach(() => vi.setSystemTime(new Date(2026, 9, 29, 12)));

  it("is named by its label and the date it shows", () => {
    render(<Field initial="2026-03-01" />);
    expect(
      screen.getByRole("button", { name: "Started Mar 1, 2026" }),
    ).toBeInTheDocument();
  });

  it("names an empty or malformed value by its placeholder", () => {
    render(<Field initial="not-a-day" />);
    expect(
      screen.getByRole("button", { name: "Started Unknown" }),
    ).toBeInTheDocument();
  });

  it("picks today with Today", async () => {
    const onChange = vi.fn();
    render(<Field initial="" onChange={onChange} />);
    await user().click(screen.getByRole("button", { name: /^Started/ }));
    await user().click(await screen.findByRole("button", { name: "Today" }));
    expect(onChange).toHaveBeenCalledWith("2026-10-29");
    expect(
      screen.getByRole("button", { name: "Started Oct 29, 2026" }),
    ).toBeInTheDocument();
  });

  it("disables Today when today is past the latest allowed day", async () => {
    render(<Field initial="" max={new Date(2026, 9, 1)} />);
    await user().click(screen.getByRole("button", { name: /^Started/ }));
    expect(await screen.findByRole("button", { name: "Today" })).toBeDisabled();
  });

  it("keeps the day and closes when the selected day is picked again", async () => {
    const onChange = vi.fn();
    render(<Field initial="2026-03-01" onChange={onChange} />);
    await user().click(screen.getByRole("button", { name: /^Started/ }));
    await user().click(
      await screen.findByRole("button", { name: /March 1st, 2026/ }),
    );
    expect(onChange).toHaveBeenCalledWith("2026-03-01");
    expect(
      screen.queryByRole("button", { name: "Today" }),
    ).not.toBeInTheDocument();
  });
});
