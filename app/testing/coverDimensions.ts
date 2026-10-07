import { afterEach, beforeEach, vi } from "vitest";

/** Gives cover components a measurable image box in jsdom, which has no layout. */
export const mockCoverDimensions = (width = 200, height = 300) => {
  let restore = () => {};
  beforeEach(() => {
    const measurement = vi
      .spyOn(HTMLImageElement.prototype, "getBoundingClientRect")
      .mockReturnValue(new DOMRect(0, 0, width, height));
    restore = () => measurement.mockRestore();
  });
  afterEach(() => restore());
};
