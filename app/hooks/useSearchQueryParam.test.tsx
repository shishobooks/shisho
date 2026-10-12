import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import { describe, expect, it } from "vitest";

import { SearchInput } from "@/components/library/SearchInput";

import { useSearchQueryParam } from "./useSearchQueryParam";

const Harness = () => {
  const { searchQuery, setSearchQuery } = useSearchQueryParam();
  const navigate = useNavigate();
  return (
    <>
      <SearchInput
        initialValue={searchQuery}
        label="Search books"
        onDebouncedChange={setSearchQuery}
      />
      <button onClick={() => navigate(-1)} type="button">
        Back
      </button>
      <output data-testid="location">{useLocation().search}</output>
    </>
  );
};

const renderAt = (entry: string) =>
  render(
    <MemoryRouter initialEntries={[entry]}>
      <Harness />
    </MemoryRouter>,
  );

const location = () => screen.getByTestId("location").textContent;

const back = () =>
  act(() => screen.getByRole("button", { name: "Back" }).click());

// Longer than the SearchInput debounce, so a stale report would have landed.
const settle = () => act(() => new Promise((r) => setTimeout(r, 400)));

describe("useSearchQueryParam with SearchInput", () => {
  it("returns to the page you were on with one back press", async () => {
    const user = userEvent.setup();
    renderAt("/?page=5");
    const input = screen.getByRole("searchbox", { name: "Search books" });

    await user.type(input, "fo");
    await waitFor(() => expect(location()).toBe("?page=1&search=fo"));
    await user.type(input, "o");
    await waitFor(() => expect(location()).toBe("?page=1&search=foo"));

    back();
    await settle();

    expect(location()).toBe("?page=5");
    expect(input).toHaveValue("");
  });

  it("keeps the earlier search when going back after clearing it", async () => {
    const user = userEvent.setup();
    renderAt("/?page=5");
    const input = screen.getByRole("searchbox", { name: "Search books" });

    await user.type(input, "foo");
    await waitFor(() => expect(location()).toBe("?page=1&search=foo"));
    await user.clear(input);
    await waitFor(() => expect(location()).toBe("?page=1"));

    back();
    await settle();

    expect(location()).toBe("?page=1&search=foo");
    expect(input).toHaveValue("foo");

    back();
    await settle();

    expect(location()).toBe("?page=5");
    expect(input).toHaveValue("");
  });
});
