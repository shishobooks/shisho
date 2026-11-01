import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import ReaderChapterSelect from "./ReaderChapterSelect";

const ENTRIES = [
  { value: "0", label: "Part Two", disabled: true },
  { value: "1", label: "Chapter 3", depth: 1 },
  { value: "2", label: "Afterword" },
];

const setup = () => {
  const onValueChange = vi.fn();
  render(
    <ReaderChapterSelect
      entries={ENTRIES}
      onValueChange={onValueChange}
      value="2"
    />,
  );
  return {
    onValueChange,
    trigger: screen.getByRole("combobox", { name: "Jump to chapter" }),
    user: userEvent.setup({ advanceTimers: vi.advanceTimersByTime }),
  };
};

describe("ReaderChapterSelect", () => {
  it("shows the current entry, disables headings, and indents nesting", async () => {
    const { trigger, user } = setup();
    expect(trigger).toHaveTextContent(/^Afterword$/);

    await user.click(trigger);
    const options = await screen.findAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual([
      "Part Two",
      "Chapter 3",
      "Afterword",
    ]);
    expect(options[0]).toHaveAttribute("aria-disabled", "true");
    expect(options[1].style.paddingLeft).toBe("1.5rem");
    expect(options[2].style.paddingLeft).toBe("0.5rem");
  });

  it("gives focus back to the book after a pick with the mouse, so the arrow keys turn pages", async () => {
    const { onValueChange, trigger, user } = setup();
    await user.click(trigger);
    await user.click(await screen.findByRole("option", { name: "Chapter 3" }));

    expect(onValueChange).toHaveBeenCalledWith("1");
    expect(trigger).not.toHaveFocus();
  });

  it("returns focus to the menu after a pick with the keyboard", async () => {
    const { onValueChange, trigger, user } = setup();
    trigger.focus();
    await user.keyboard("{Enter}");
    await screen.findByRole("listbox");
    await user.keyboard("{ArrowUp}{Enter}");

    expect(onValueChange).toHaveBeenCalledWith("1");
    await vi.waitFor(() => expect(trigger).toHaveFocus());
  });
});
