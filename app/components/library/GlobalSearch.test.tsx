import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import GlobalSearch from "./GlobalSearch";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const renderSearch = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/libraries/1"]}>
        <Routes>
          <Route element={<GlobalSearch />} path="/libraries/:libraryId" />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const searchRequests = (request: ReturnType<typeof vi.spyOn>) =>
  request.mock.calls.filter((call: unknown[]) => call[1] === "/search");

const flush = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 350));
  });

describe("GlobalSearch", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("offers no search to a role without Books Read", async () => {
    setAuth({ permissions: ["shares:write"] });
    const request = vi.spyOn(API, "request").mockResolvedValue([]);

    renderSearch();
    await flush();

    expect(screen.queryByPlaceholderText("Search library...")).toBeNull();
    expect(searchRequests(request)).toHaveLength(0);
  });

  it("searches the library for a role with Books Read", async () => {
    setAuth({ permissions: ["books:read"] });
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(async (_method, path) =>
        path === "/search" ? { books: [], series: [], people: [] } : [],
      );
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    renderSearch();
    await user.type(screen.getByPlaceholderText("Search library..."), "dune");
    await flush();

    expect(searchRequests(request)).toHaveLength(1);
  });

  it("clears the query with a labeled button", async () => {
    setAuth({ permissions: ["books:read"] });
    vi.spyOn(API, "request").mockImplementation(async (_method, path) =>
      path === "/search" ? { books: [], series: [], people: [] } : [],
    );
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    renderSearch();
    const input = screen.getByPlaceholderText("Search library...");
    await user.type(input, "dune");
    await user.click(screen.getByRole("button", { name: "Clear search" }));

    expect(input).toHaveValue("");
    expect(input).toHaveFocus();
  });

  it("is a combobox whose arrow keys move the active result while focus stays in the input", async () => {
    setAuth({ permissions: ["books:read"] });
    vi.spyOn(API, "request").mockImplementation(async (_method, path) =>
      path === "/search"
        ? {
            books: [
              {
                id: 1,
                title: "Dune",
                authors: "Frank Herbert",
                file_types: ["epub"],
                library_id: 1,
              },
            ],
            series: [
              { id: 2, name: "Dune Saga", book_count: 3, library_id: 1 },
            ],
            people: [
              { id: 3, name: "Brian Herbert", sort_name: "", library_id: 1 },
            ],
          }
        : [],
    );
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    renderSearch();
    const input = screen.getByRole("combobox", { name: "Search library" });
    expect(input).toHaveAttribute("aria-expanded", "false");
    expect(input).toHaveAttribute("aria-autocomplete", "list");

    await user.type(input, "dune");
    await flush();

    const listbox = await screen.findByRole("listbox", {
      name: "Search results",
    });
    expect(input).toHaveAttribute("aria-expanded", "true");
    expect(input).toHaveAttribute("aria-controls", listbox.id);
    expect(input).not.toHaveAttribute("aria-activedescendant");
    for (const name of ["Books", "Series", "People"]) {
      expect(within(listbox).getByRole("group", { name })).toBeVisible();
    }
    const options = within(listbox).getAllByRole("option");
    expect(options).toHaveLength(3);
    expect(
      options.map((option) => option.getAttribute("aria-selected")),
    ).toEqual(["false", "false", "false"]);

    await user.keyboard("{ArrowDown}");
    expect(input).toHaveAttribute("aria-activedescendant", options[0].id);
    expect(options[0]).toHaveAttribute("aria-selected", "true");

    await user.keyboard("{ArrowDown}{ArrowDown}");
    expect(input).toHaveAttribute("aria-activedescendant", options[2].id);
    expect(options[2]).toHaveAttribute("aria-selected", "true");
    expect(options[0]).toHaveAttribute("aria-selected", "false");

    await user.keyboard("{ArrowUp}");
    expect(input).toHaveAttribute("aria-activedescendant", options[1].id);
    expect(within(options[1]).getByText("Dune Saga")).toBeVisible();
    expect(input).toHaveFocus();

    // A click inside the list that is not a result keeps it open and keeps
    // focus in the input.
    await user.click(within(listbox).getByText("Series"));
    expect(input).toHaveFocus();
    expect(input).toHaveAttribute("aria-expanded", "true");

    // Tab leaves the combobox rather than walking the results, and closes
    // the list so aria-expanded does not point at a popup left behind.
    await user.tab();
    expect(input).not.toHaveFocus();
    expect(input).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("announces that it is searching and when nothing matched", async () => {
    setAuth({ permissions: ["books:read"] });
    let resolveSearch: (value: unknown) => void = () => {};
    vi.spyOn(API, "request").mockImplementation((_method, path) =>
      path === "/search"
        ? new Promise((resolve) => {
            resolveSearch = resolve;
          })
        : Promise.resolve([]),
    );
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    renderSearch();
    // The region is mounted before there is anything to say, so each new
    // message is announced.
    const status = screen.getByRole("status");
    expect(status).toHaveTextContent(/^$/);
    await user.type(screen.getByPlaceholderText("Search library..."), "zzz");
    await flush();

    expect(status).toHaveTextContent("Searching...");
    await act(async () => {
      resolveSearch({ books: [], series: [], people: [] });
    });
    await flush();
    expect(status).toHaveTextContent('No results found for "zzz"');
    expect(screen.getByRole("status")).toBe(status);
  });
});
