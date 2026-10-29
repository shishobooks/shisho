import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import StarRating from "./StarRating";

// jsdom has no layout: give the stars a 120px box at the left edge.
const placeStars = () => {
  const stars = screen.getByTestId("star-rating");
  vi.spyOn(stars, "getBoundingClientRect").mockReturnValue({
    left: 0,
    width: 120,
    top: 0,
    height: 24,
    right: 120,
    bottom: 24,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  });
  return stars;
};

describe("StarRating", () => {
  it("shows the saved rating as a number", () => {
    render(<StarRating onChange={vi.fn()} value={4.5} />);
    expect(screen.getByText("4.5")).toBeInTheDocument();
  });

  it("shows no number when unrated", () => {
    render(<StarRating onChange={vi.fn()} value={null} />);
    expect(screen.queryByText(/^\d/)).not.toBeInTheDocument();
  });

  it("previews the quarter under the pointer before a click", () => {
    const onChange = vi.fn();
    render(<StarRating onChange={onChange} value={2} />);
    const stars = placeStars();

    // 4.5 stars of 5 is 108px across; 105px is nearer 4.5 than 4.25.
    fireEvent.pointerMove(stars, { clientX: 105 });
    expect(screen.getByText("4.5")).toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();

    fireEvent.pointerLeave(stars);
    expect(screen.getByText("2")).toBeInTheDocument();
  });

  it("previews at least a quarter star at the far left", () => {
    render(<StarRating onChange={vi.fn()} value={3} />);
    const stars = placeStars();
    fireEvent.pointerMove(stars, { clientX: 1 });
    expect(screen.getByText("0.25")).toBeInTheDocument();
  });

  // A click in the first 1/40 of an unrated row sets nothing (the slider is
  // already at 0), so it previews nothing either.
  it("previews nothing where a click on an unrated book sets nothing", () => {
    render(<StarRating onChange={vi.fn()} value={null} />);
    const stars = placeStars();
    fireEvent.pointerMove(stars, { clientX: 1 });
    expect(screen.queryByText(/^\d/)).not.toBeInTheDocument();
    fireEvent.pointerMove(stars, { clientX: 4 });
    expect(screen.getByText("0.25")).toBeInTheDocument();
  });

  it("shows an arrow key's change while the pointer rests on the stars", () => {
    render(<StarRating onChange={vi.fn()} value={3} />);
    const stars = placeStars();
    fireEvent.pointerMove(stars, { clientX: 105 });
    expect(screen.getByText("4.5")).toBeInTheDocument();
    fireEvent.keyDown(screen.getByRole("slider", { name: "Rating" }), {
      key: "ArrowRight",
    });
    expect(screen.getByText("3.25")).toBeInTheDocument();
  });

  it("doesn't preview when disabled", () => {
    render(<StarRating disabled onChange={vi.fn()} value={3} />);
    const stars = placeStars();
    fireEvent.pointerMove(stars, { clientX: 105 });
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("turns solid once the hovered rating is saved, before the pointer leaves", () => {
    const onChange = vi.fn();
    const { rerender } = render(<StarRating onChange={onChange} value={2} />);
    const stars = placeStars();
    const fill = () => screen.getByTestId("star-rating-fill");

    fireEvent.pointerMove(stars, { clientX: 105 });
    expect(fill()).toHaveAttribute("data-previewing", "true");

    // The click saved 4.5, and the pointer is still over 4.5.
    rerender(<StarRating onChange={onChange} value={4.5} />);
    expect(fill()).toHaveAttribute("data-previewing", "false");
    expect(screen.getByText("4.5")).toBeInTheDocument();

    // Moving to another quarter previews again.
    fireEvent.pointerMove(stars, { clientX: 60 });
    expect(fill()).toHaveAttribute("data-previewing", "true");
  });
});
