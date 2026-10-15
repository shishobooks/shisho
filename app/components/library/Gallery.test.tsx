import { render, screen, waitFor } from "@testing-library/react";
import {
  createMemoryRouter,
  MemoryRouter,
  RouterProvider,
} from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import Gallery from "./Gallery";

const query = {
  data: undefined as unknown,
  error: null as unknown,
  isFetching: false,
  isEnabled: true,
  isPlaceholderData: false,
  refetch: vi.fn(),
};

const renderGallery = (
  overrides: Partial<typeof query>,
  items: string[] = [],
) =>
  render(
    <MemoryRouter>
      <Gallery
        isLoading={false}
        itemLabel="books"
        items={items}
        query={{ ...query, ...overrides }}
        renderItem={(item) => <div key={item}>{item}</div>}
        total={items.length}
      />
    </MemoryRouter>,
  );

describe("Gallery", () => {
  it("replaces a page past the end with the last page", async () => {
    const router = createMemoryRouter(
      [
        {
          path: "/",
          element: (
            <Gallery
              isLoading={false}
              itemLabel="books"
              items={[]}
              query={{ ...query, data: { items: [], total: 45 } }}
              renderItem={(item: string) => <div key={item}>{item}</div>}
              total={45}
            />
          ),
        },
      ],
      { initialEntries: ["/?page=9"] },
    );
    render(<RouterProvider router={router} />);

    // 45 items at 20 per page is three pages.
    await waitFor(() => expect(router.state.location.search).toBe("?page=3"));
    expect(router.state.historyAction).toBe("REPLACE");
  });

  it("shows a spinner, not an error, while its query waits on its gate", () => {
    renderGallery({ isEnabled: false });

    expect(screen.getByText("Loading...")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("reports a failed query that has no data", () => {
    renderGallery({ error: new TypeError("Failed to fetch") });

    expect(screen.getByRole("alert")).toHaveTextContent(
      /^Failed to load books/,
    );
  });

  it("keeps the loaded items when a background refetch fails", () => {
    renderGallery(
      {
        data: { items: [], total: 1 },
        error: new TypeError("Failed to fetch"),
      },
      ["Dune"],
    );

    expect(screen.getByText("Dune")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("announces the count and the empty state in one region that stays mounted", () => {
    const gallery = (isLoading: boolean, items: string[]) => (
      <MemoryRouter>
        <Gallery
          isLoading={isLoading}
          itemLabel="books"
          items={items}
          query={{ ...query, data: { items: [], total: items.length } }}
          renderItem={(item) => <div key={item}>{item}</div>}
          total={items.length}
        />
      </MemoryRouter>
    );
    const { rerender } = render(gallery(true, []));
    const region = screen
      .getAllByRole("status")
      .find((element) => !element.textContent?.includes("Loading"));
    expect(region).toBeDefined();
    expect(region).toHaveTextContent(/^$/);

    rerender(gallery(false, ["Dune", "Emma"]));
    expect(region).toBeInTheDocument();
    expect(region).toHaveTextContent("Showing 1-2 of 2 books");

    rerender(gallery(true, []));
    expect(region).toHaveTextContent(/^$/);
    rerender(gallery(false, []));
    expect(region).toBeInTheDocument();
    expect(region).toHaveTextContent("No books found.");
  });
});
