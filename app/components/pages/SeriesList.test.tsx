import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeAll, describe, expect, it, vi } from "vitest";

import { API, ShishoAPIError } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { SeriesResponse } from "@/types";

import SeriesList, { SeriesCard } from "./SeriesList";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

beforeAll(() => {
  window.matchMedia = vi.fn(() => ({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })) as unknown as typeof window.matchMedia;
});

const series = {
  id: 3,
  name: "Saga",
  library_id: 1,
  book_count: 2,
  cover_cache_key: "12-1704067200",
} as SeriesResponse;

describe("SeriesCard", () => {
  it("keys the cover URL on the series' cover_cache_key", () => {
    render(
      <MemoryRouter>
        <SeriesCard
          aspectClass="aspect-[2/3]"
          isAudiobook={false}
          libraryId="1"
          seriesItem={series}
        />
      </MemoryRouter>,
    );

    expect(screen.getByAltText("Saga Cover")).toHaveAttribute(
      "src",
      "/api/series/3/cover?v=12-1704067200",
    );
  });
});

describe("SeriesList search failure", () => {
  it("reports a failed search after the unfiltered list loaded", async () => {
    setAuth({ permissions: ["books:read", "series:read"] });
    // The unfiltered list loads; any search fails.
    vi.spyOn(API, "request").mockImplementation(
      async (_method, path, _payload, query) => {
        if (path === "/settings/user") return { gallery_size: "m" };
        if (path === "/user/libraries") return [];
        if (path === "/series" && (query as { search?: string })?.search) {
          throw new ShishoAPIError(
            "Internal Server Error",
            "internal_server_error",
            500,
          );
        }
        return { items: [], total: 0 };
      },
    );
    const user = userEvent.setup();
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <MemoryRouter initialEntries={["/libraries/1/series"]}>
          <Routes>
            <Route
              element={<SeriesList />}
              path="/libraries/:libraryId/series"
            />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(
      await screen.findByText("No series in this library yet."),
    ).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText("Search series..."), "zzz");

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /^Failed to load series/,
    );
  });
});
