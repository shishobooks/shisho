import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect } from "react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import LibraryListPicker from "./LibraryListPicker";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const defaultLibraries = [
  { id: 1, name: "Fiction" },
  { id: 2, name: "Comics" },
];

const stubRequests = (libraries = defaultLibraries) =>
  vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
    if (path === "/user/libraries") return libraries;
    if (path === "/lists") {
      return {
        items: [
          { id: 10, name: "Favorites", book_count: 4 },
          { id: 11, name: "Queue", book_count: 2 },
        ],
        total: 2,
      };
    }
    if (path === "/books") return { items: [], total: 3 };
    return { items: [], total: 0 };
  });

// Records every history entry, so a double navigation shows up as a repeat.
const HistoryProbe = ({ visited }: { visited: string[] }) => {
  const location = useLocation();
  useEffect(() => {
    visited.push(location.pathname);
  }, [location.key, location.pathname, visited]);
  return null;
};

const renderPicker = ({
  visited,
  path = "/libraries/1/books/7",
}: { visited?: string[]; path?: string } = {}) =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route
            element={<LibraryListPicker />}
            path="/libraries/:libraryId/*"
          />
          <Route element={<LibraryListPicker />} path="/lists/:id" />
          <Route element={<LibraryListPicker />} path="/to-read" />
        </Routes>
        {visited && <HistoryProbe visited={visited} />}
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("LibraryListPicker", () => {
  beforeEach(() => setAuth({ permissions: ["books:read"] }));

  afterEach(() => vi.restoreAllMocks());

  it("shows the current library for a role with Books Read, without Libraries Read", async () => {
    stubRequests();
    renderPicker();

    expect(
      await screen.findByRole("button", { name: /Fiction/ }),
    ).toBeInTheDocument();
  });

  it("picks a library and a list with the keyboard and marks the current one", async () => {
    stubRequests();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const visited: string[] = [];
    renderPicker({ visited });
    await screen.findByRole("button", { name: /Fiction/ });

    // Open from the keyboard: Radix focuses the first item.
    await user.tab();
    await user.keyboard("{Enter}");
    const fiction = await screen.findByRole("menuitem", { name: "Fiction" });
    await waitFor(() => expect(fiction).toHaveFocus());
    expect(fiction).toHaveAttribute("aria-current", "true");
    expect(
      screen.getByRole("menuitem", { name: "Comics" }),
    ).not.toHaveAttribute("aria-current");
    expect(screen.getByRole("menuitem", { name: /Queue/ })).not.toHaveAttribute(
      "aria-current",
    );

    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitem", { name: "Comics" })).toHaveFocus();
    await user.keyboard("{Enter}");
    await waitFor(() =>
      expect(visited).toEqual(["/libraries/1/books/7", "/libraries/2"]),
    );
    await waitFor(() =>
      expect(screen.queryByRole("menu")).not.toBeInTheDocument(),
    );

    // Reopen and arrow past both libraries and To Read to the second list.
    screen.getByRole("button", { name: /Comics/ }).focus();
    await user.keyboard("{Enter}");
    await waitFor(() =>
      expect(screen.getByRole("menuitem", { name: "Fiction" })).toHaveFocus(),
    );
    await user.keyboard("{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}");
    expect(screen.getByRole("menuitem", { name: /Queue/ })).toHaveFocus();
    await user.keyboard("{Enter}");
    await waitFor(() =>
      expect(visited).toEqual([
        "/libraries/1/books/7",
        "/libraries/2",
        "/lists/11",
      ]),
    );

    // On a list page the list is the current page and no library is current.
    const trigger = await screen.findByRole("button", { name: /Queue/ });
    trigger.focus();
    await user.keyboard("{Enter}");
    expect(
      await screen.findByRole("menuitem", { name: /Queue/ }),
    ).toHaveAttribute("aria-current", "page");
    expect(
      screen.getByRole("menuitem", { name: /Favorites/ }),
    ).not.toHaveAttribute("aria-current");
    expect(
      screen.getByRole("menuitem", { name: "Comics" }),
    ).not.toHaveAttribute("aria-current");
  });

  it("keeps a typed space as typeahead instead of choosing an entry", async () => {
    stubRequests([
      { id: 1, name: "My Books" },
      { id: 2, name: "My Comics" },
    ]);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const visited: string[] = [];
    renderPicker({ visited });
    await screen.findByRole("button", { name: /My Books/ });

    await user.tab();
    await user.keyboard("{Enter}");
    await waitFor(() =>
      expect(screen.getByRole("menuitem", { name: "My Books" })).toHaveFocus(),
    );
    await user.keyboard("my c");

    expect(screen.getByRole("menuitem", { name: "My Comics" })).toHaveFocus();
    expect(visited).toEqual(["/libraries/1/books/7"]);
  });

  it("renders nothing and requests no libraries for a role without Books Read", async () => {
    setAuth({ permissions: ["shares:read"] });
    const request = stubRequests();
    const { container } = renderPicker();
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(container).toBeEmptyDOMElement();
    expect(request.mock.calls.map((call) => call[1])).not.toContain(
      "/user/libraries",
    );
  });

  it("leads the Lists section with To Read and its count", async () => {
    stubRequests();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPicker();

    await user.click(await screen.findByRole("button", { name: /Fiction/ }));
    const toRead = await screen.findByRole("menuitem", { name: /To Read/ });
    expect(toRead).toHaveAttribute("href", "/to-read");
    await waitFor(() => expect(toRead).toHaveTextContent("3"));
    const lists = screen
      .getAllByRole("menuitem")
      .filter((item) => item.getAttribute("href") !== null)
      .map((item) => item.getAttribute("href"))
      .filter((href) => !href!.startsWith("/libraries/"));
    expect(lists).toEqual(["/to-read", "/lists/10", "/lists/11"]);
  });

  it("names To Read as the current page on it", async () => {
    stubRequests();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPicker({ path: "/to-read" });

    const trigger = await screen.findByRole("button", { name: /To Read/ });
    await user.click(trigger);
    expect(
      await screen.findByRole("menuitem", { name: /To Read/ }),
    ).toHaveAttribute("aria-current", "page");
    expect(
      screen.getByRole("menuitem", { name: "Fiction" }),
    ).not.toHaveAttribute("aria-current");
  });
});
