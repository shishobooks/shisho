import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import { mockCoverDimensions } from "@/testing/coverDimensions";
import type { Book } from "@/types";

import CurrentlyReadingRow from "./CurrentlyReadingRow";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const book = (id: number, title: string, status: Book["reading_status"]) =>
  ({
    id,
    title,
    library_id: 1,
    files: [],
    authors: [],
    reading_status: status,
  }) as unknown as Book;

const Location = () => {
  const location = useLocation();
  return <output aria-label="Location">{location.search}</output>;
};

const renderRow = (items: Book[], total = items.length) => {
  const request = vi
    .spyOn(API, "request")
    .mockResolvedValue({ items, total } as never);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/libraries/1"]}>
        <Routes>
          <Route
            element={
              <>
                <CurrentlyReadingRow
                  columns={3}
                  coverAspectRatio="book"
                  libraryId="1"
                />
                <Location />
              </>
            }
            path="/libraries/:libraryId"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return request;
};

beforeEach(() => {
  vi.restoreAllMocks();
  setAuth({ permissions: ["books:read"] });
  mockCoverDimensions();
});

describe("CurrentlyReadingRow", () => {
  it("asks for the library's books underway and keeps the server's order, which puts paused ones last", async () => {
    const request = renderRow([
      book(1, "Underway", "currently_reading"),
      book(2, "Also underway", "currently_reading"),
      book(3, "Set aside", "paused"),
    ]);

    const row = await screen.findByRole("region", {
      name: "Currently Reading",
    });
    expect(request).toHaveBeenCalledWith(
      "GET",
      "/books",
      null,
      { library_id: 1, currently_reading: true, limit: 3 },
      expect.anything(),
    );
    const bookLinks = within(row)
      .getAllByRole("link")
      .map((link) => link.getAttribute("href"))
      .filter((href) => /\/books\/\d+$/.test(href ?? ""));
    expect(bookLinks).toEqual([
      "/libraries/1/books/1",
      "/libraries/1/books/2",
      "/libraries/1/books/3",
    ]);
    expect(
      within(row).queryByRole("button", { name: /Show all/ }),
    ).not.toBeInTheDocument();
  });

  it("links to every book underway when they don't fit in one row", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderRow(
      [
        book(1, "One", "currently_reading"),
        book(2, "Two", "currently_reading"),
        book(3, "Three", "paused"),
      ],
      5,
    );

    await user.click(await screen.findByRole("button", { name: "Show all 5" }));
    expect(screen.getByRole("status", { name: "Location" })).toHaveTextContent(
      "?reading_status=currently_reading%2Cpaused",
    );
  });

  it("shows nothing when nothing is underway", async () => {
    const request = renderRow([]);
    await waitFor(() => expect(request).toHaveBeenCalled());
    expect(
      screen.queryByRole("region", { name: "Currently Reading" }),
    ).not.toBeInTheDocument();
  });
});
