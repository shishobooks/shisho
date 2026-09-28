import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import PagePreview from "./PagePreview";

describe("PagePreview", () => {
  it("loads the page thumbnail with the file's cache key", () => {
    render(
      <PagePreview
        file={{ id: 100, updated_at: "2024-06-01T00:00:00Z" }}
        page={4}
      />,
    );
    expect(screen.getByAltText("Page 5")).toHaveAttribute(
      "src",
      "/api/books/files/100/page/4?v=1717200000000",
    );
  });

  it("retries the image when the page URL changes after an error", () => {
    const { rerender } = render(
      <PagePreview
        file={{ id: 100, updated_at: "2024-01-01T00:00:00Z" }}
        page={4}
      />,
    );
    fireEvent.error(screen.getByAltText("Page 5"));
    expect(screen.queryByAltText("Page 5")).not.toBeInTheDocument();

    rerender(
      <PagePreview
        file={{ id: 100, updated_at: "2024-06-01T00:00:00Z" }}
        page={4}
      />,
    );
    expect(screen.getByAltText("Page 5")).toHaveAttribute(
      "src",
      "/api/books/files/100/page/4?v=1717200000000",
    );
  });
});
