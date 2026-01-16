import { SortNameInput } from "./SortNameInput";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

describe("SortNameInput", () => {
  it("shows checkbox and input", () => {
    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={() => {}}
        sortValue=""
        source="manual"
        type="person"
      />,
    );

    expect(screen.getByRole("checkbox")).toBeInTheDocument();
    expect(screen.getByRole("textbox")).toBeInTheDocument();
  });

  it("checkbox is checked when source is not manual", () => {
    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={() => {}}
        sortValue="King, Stephen"
        source="filepath"
        type="person"
      />,
    );

    expect(screen.getByRole("checkbox")).toBeChecked();
  });

  it("checkbox is unchecked when source is manual", () => {
    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={() => {}}
        sortValue="King, S."
        source="manual"
        type="person"
      />,
    );

    expect(screen.getByRole("checkbox")).not.toBeChecked();
  });

  it("shows live preview when checkbox is checked", () => {
    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={() => {}}
        sortValue=""
        source="filepath"
        type="person"
      />,
    );

    expect(screen.getByRole("textbox")).toHaveValue("King, Stephen");
    expect(screen.getByRole("textbox")).toBeDisabled();
  });

  it("updates preview when nameValue changes", () => {
    const { rerender } = render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={() => {}}
        sortValue=""
        source="filepath"
        type="person"
      />,
    );

    expect(screen.getByRole("textbox")).toHaveValue("King, Stephen");

    rerender(
      <SortNameInput
        nameValue="J.R.R. Tolkien"
        onChange={() => {}}
        sortValue=""
        source="filepath"
        type="person"
      />,
    );

    expect(screen.getByRole("textbox")).toHaveValue("Tolkien, J.R.R.");
  });

  it("calls onChange with empty string when checkbox is checked", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={onChange}
        sortValue="King, S."
        source="manual"
        type="person"
      />,
    );

    await user.click(screen.getByRole("checkbox"));

    expect(onChange).toHaveBeenCalledWith("");
  });

  it("enables input and pre-fills with generated value when unchecked", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={onChange}
        sortValue=""
        source="filepath"
        type="person"
      />,
    );

    // Initially disabled
    expect(screen.getByRole("textbox")).toBeDisabled();

    // Uncheck the checkbox
    await user.click(screen.getByRole("checkbox"));

    // Now enabled with generated value
    expect(screen.getByRole("textbox")).not.toBeDisabled();
    expect(screen.getByRole("textbox")).toHaveValue("King, Stephen");
    expect(onChange).toHaveBeenCalledWith("King, Stephen");
  });

  it("uses forTitle for title type", () => {
    render(
      <SortNameInput
        nameValue="The Hobbit"
        onChange={() => {}}
        sortValue=""
        source="filepath"
        type="title"
      />,
    );

    expect(screen.getByRole("textbox")).toHaveValue("Hobbit, The");
  });

  it("uses forPerson for person type", () => {
    render(
      <SortNameInput
        nameValue="Ludwig van Beethoven"
        onChange={() => {}}
        sortValue=""
        source="filepath"
        type="person"
      />,
    );

    expect(screen.getByRole("textbox")).toHaveValue("Beethoven, Ludwig van");
  });

  it("shows correct label for title type", () => {
    render(
      <SortNameInput
        nameValue="The Hobbit"
        onChange={() => {}}
        sortValue=""
        source="filepath"
        type="title"
      />,
    );

    expect(screen.getByText("Autogenerate sort title")).toBeInTheDocument();
  });

  it("shows correct label for person type", () => {
    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={() => {}}
        sortValue=""
        source="filepath"
        type="person"
      />,
    );

    expect(screen.getByText("Autogenerate sort name")).toBeInTheDocument();
  });

  it("allows typing when checkbox is unchecked", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={onChange}
        sortValue="Custom Sort"
        source="manual"
        type="person"
      />,
    );

    const input = screen.getByRole("textbox");
    await user.clear(input);
    await user.type(input, "New Value");

    expect(onChange).toHaveBeenLastCalledWith("New Value");
  });
});
