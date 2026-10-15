import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import PaginationFooter from "./PaginationFooter";

const setup = () => userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

const renderFooter = (currentPage: number, totalPages: number) => {
  const onPageChange = vi.fn();
  render(
    <PaginationFooter
      currentPage={currentPage}
      onPageChange={onPageChange}
      totalPages={totalPages}
    />,
  );
  return onPageChange;
};

describe("PaginationFooter", () => {
  it("names the current page and the page count with thousands grouped", () => {
    renderFooter(1200, 18307);

    expect(
      screen.getByRole("button", { name: /Page 1,200 of 18,307/ }),
    ).toBeInTheDocument();
  });

  it("disables the backward buttons on the first page", async () => {
    const onPageChange = renderFooter(1, 367);

    expect(
      screen.getByRole("button", { name: "Go to first page" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Go to previous page" }),
    ).toBeDisabled();

    await setup().click(
      screen.getByRole("button", { name: "Go to last page" }),
    );
    expect(onPageChange).toHaveBeenCalledWith(367);
  });

  it("disables the forward buttons on the last page", async () => {
    const onPageChange = renderFooter(367, 367);

    expect(
      screen.getByRole("button", { name: "Go to last page" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Go to next page" }),
    ).toBeDisabled();

    await setup().click(
      screen.getByRole("button", { name: "Go to first page" }),
    );
    expect(onPageChange).toHaveBeenCalledWith(1);
  });

  it("lists the pages around the current one and marks it", async () => {
    renderFooter(120, 367);

    await setup().click(
      screen.getByRole("button", { name: /Page 120 of 367/ }),
    );

    const options = screen.getAllByRole("option");
    expect(options[0]).toHaveTextContent("Page 70");
    expect(options[options.length - 1]).toHaveTextContent("Page 170");
    expect(screen.getByRole("option", { name: /Page 120/ })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("narrows to pages starting with the typed digits, in order", async () => {
    const user = setup();
    const onPageChange = renderFooter(120, 367);

    await user.click(screen.getByRole("button", { name: /Page 120 of 367/ }));
    await user.keyboard("34");

    const listbox = screen.getByRole("listbox");
    expect(
      within(listbox)
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual([
      "Page 34",
      "Page 340",
      "Page 341",
      "Page 342",
      "Page 343",
      "Page 344",
      "Page 345",
      "Page 346",
      "Page 347",
      "Page 348",
      "Page 349",
    ]);
    // Only options in the list: no separators between them.
    expect(listbox.querySelectorAll("[cmdk-item]")).toHaveLength(11);

    await user.keyboard("{Enter}");
    expect(onPageChange).toHaveBeenCalledWith(34);
  });

  it("moves to the last page, replacing history, when the page is past the end", () => {
    const onPageChange = renderFooter(10, 3);

    expect(onPageChange).toHaveBeenCalledWith(3, { replace: true });
  });

  it("moves to page 1 when a single page of results is all there is", () => {
    const onPageChange = renderFooter(4, 1);

    expect(onPageChange).toHaveBeenCalledWith(1, { replace: true });
  });

  it("leaves the page alone while there are no results", () => {
    const onPageChange = renderFooter(4, 0);

    expect(onPageChange).not.toHaveBeenCalled();
  });
});
