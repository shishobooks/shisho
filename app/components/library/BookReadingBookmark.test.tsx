import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import BookReadingBookmark from "./BookReadingBookmark";

const reading = vi.hoisted(() => ({
  current: undefined as object | undefined,
}));

vi.mock("@/hooks/queries/reading", () => ({
  useBookReading: () => ({ data: reading.current }),
}));

describe("BookReadingBookmark", () => {
  beforeEach(() => {
    reading.current = undefined;
  });

  it("shows nothing without a status", () => {
    reading.current = { status: null, progress: null };
    render(<BookReadingBookmark bookId={1} />);

    expect(screen.queryByRole("img")).toBeNull();
  });

  it("puts the book's progress on the bookmark", () => {
    reading.current = { status: "currently_reading", progress: 0.62 };
    render(<BookReadingBookmark bookId={1} />);

    expect(
      screen.getByRole("img", { name: "Currently Reading, 62% through" }),
    ).toHaveTextContent("62%");
  });
});
