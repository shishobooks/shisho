import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { SearchInput } from "./SearchInput";

describe("SearchInput", () => {
  it("is named by its label, not its placeholder", () => {
    render(
      <SearchInput
        initialValue=""
        label="Search books"
        onDebouncedChange={vi.fn()}
        placeholder="Search..."
      />,
    );

    expect(
      screen.getByRole("searchbox", { name: "Search books" }),
    ).toBeInTheDocument();
  });
});
