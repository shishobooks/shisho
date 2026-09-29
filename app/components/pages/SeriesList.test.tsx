import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";

import type { SeriesResponse } from "@/types";

import { SeriesCard } from "./SeriesList";

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
