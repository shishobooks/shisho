import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useIsTruncated } from "./useIsTruncated";

const Probe = () => {
  const [ref, truncated] = useIsTruncated<HTMLDivElement>();
  return <div ref={ref}>{truncated ? "truncated" : "fits"}</div>;
};

const measure = (box: {
  scroll: [number, number];
  client: [number, number];
}) => {
  vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(
    box.scroll[0],
  );
  vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(
    box.scroll[1],
  );
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(
    box.client[0],
  );
  vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(
    box.client[1],
  );
};

afterEach(() => vi.restoreAllMocks());

describe("useIsTruncated", () => {
  it("reports text clamped to fewer lines", () => {
    measure({ scroll: [100, 60], client: [100, 40] });
    render(<Probe />);
    expect(screen.getByText("truncated")).toBeInTheDocument();
  });

  it("reports a single line cut off with an ellipsis", () => {
    measure({ scroll: [180, 20], client: [100, 20] });
    render(<Probe />);
    expect(screen.getByText("truncated")).toBeInTheDocument();
  });

  it("reports text that fits", () => {
    measure({ scroll: [100, 20], client: [100, 20] });
    render(<Probe />);
    expect(screen.getByText("fits")).toBeInTheDocument();
  });
});
