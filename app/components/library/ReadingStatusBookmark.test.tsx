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

  it.each([undefined, null, 0, 0.004])(
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

  it("keeps a paused book's icon, whatever its progress", () => {
    const { container } = render(
      <ReadingStatusBookmark progress={0.62} status="paused" />,
    );

    expect(screen.getByRole("img", { name: "Paused" })).toHaveTextContent("");
    expect(container.querySelector("svg")).not.toBeNull();
  });
});
