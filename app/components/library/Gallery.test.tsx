import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import Gallery from "./Gallery";

vi.mock("@/components/library/PaginationFooter", () => ({
  default: () => null,
}));

const query = {
  data: undefined as unknown,
  error: null as unknown,
  isFetching: false,
  isEnabled: true,
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
  it("shows a spinner, not an error, while its query waits on its gate", () => {
    renderGallery({ isEnabled: false });

    expect(screen.getByRole("status")).toBeInTheDocument();
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
});
