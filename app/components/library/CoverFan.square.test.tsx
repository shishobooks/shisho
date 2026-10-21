import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { mockCoverDimensions } from "@/testing/coverDimensions";

import CoverFan from "./CoverFan";

// A square cover's 56x56 box, which CoverImage reads to pick its thumbnail.
mockCoverDimensions(56, 56);

describe("CoverFan square covers", () => {
  it("requests a square thumbnail for a square cover", () => {
    const { container } = render(
      <CoverFan
        books={[
          {
            id: 1,
            title: "Audio",
            cover_cache_key: "1-1",
            cover_aspect: "square",
          },
        ]}
        libraryCoverAspectRatio="audiobook"
      />,
    );

    expect(container.querySelector("img")!.getAttribute("src")).toContain(
      "aspect=square",
    );
  });
});
