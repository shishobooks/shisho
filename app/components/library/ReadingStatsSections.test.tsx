import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { PaceList } from "./ReadingStatsSections";

describe("PaceList", () => {
  it("lists only the kinds with enough timed reading", () => {
    render(
      <PaceList
        pace={{
          words_per_minute: 312.6,
          comic_pages_per_hour: null,
          pdf_pages_per_hour: null,
          listening_speed: 1,
        }}
      />,
    );
    expect(screen.getByText("Ebooks")).toBeInTheDocument();
    expect(screen.getByText("313 words per minute")).toBeInTheDocument();
    expect(screen.queryByText("Comics")).toBeNull();
    expect(screen.queryByText("PDFs")).toBeNull();
    expect(screen.getByText("1× real time")).toBeInTheDocument();
    expect(screen.getByText(/timed reading/)).toBeInTheDocument();
  });

  it("says when there isn't enough timed reading", () => {
    render(
      <PaceList
        pace={{
          words_per_minute: null,
          comic_pages_per_hour: null,
          pdf_pages_per_hour: null,
          listening_speed: null,
        }}
      />,
    );
    expect(
      screen.getByText("Not enough timed reading yet."),
    ).toBeInTheDocument();
  });
});
