import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import ReadingStatusBookmark from "./ReadingStatusBookmark";

describe("ReadingStatusBookmark", () => {
  it("shows how far through a book being read, in place of its icon", () => {
    const { container } = render(
      <ReadingStatusBookmark progress={0.62} status="currently_reading" />,
    );

    expect(
      screen.getByRole("img", { name: "Currently Reading, 62% through" }),
    ).toHaveTextContent("62%");
    expect(container.querySelector("svg")).toBeNull();
  });

  it("shows 0% for a book just started", () => {
    render(<ReadingStatusBookmark progress={0} status="currently_reading" />);

    expect(
      screen.getByRole("img", { name: "Currently Reading, 0% through" }),
    ).toHaveTextContent("0%");
  });

  it.each([undefined, null])(
    "keeps the icon for progress of %s",
    (progress) => {
      const { container } = render(
        <ReadingStatusBookmark
          progress={progress}
          status="currently_reading"
        />,
      );

      expect(
        screen.getByRole("img", { name: "Currently Reading" }),
      ).toHaveTextContent("");
      expect(container.querySelector("svg")).not.toBeNull();
    },
  );

  it("keeps a read book's check mark, not a percentage", () => {
    const { container } = render(
      <ReadingStatusBookmark progress={1} status="read" />,
    );

    expect(screen.getByRole("img", { name: "Read" })).toHaveTextContent("");
    expect(container.querySelector("svg")).not.toBeNull();
  });

  it("keeps a paused book's icon, whatever its progress", () => {
    const { container } = render(
      <ReadingStatusBookmark progress={0.62} status="paused" />,
    );

    expect(screen.getByRole("img", { name: "Paused" })).toHaveTextContent("");
    expect(container.querySelector("svg")).not.toBeNull();
  });
});
