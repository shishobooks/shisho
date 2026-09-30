import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";
import type { Info as CacheInfo } from "@/types/generated/cache";

import AdminCache from "./AdminCache";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const caches: CacheInfo[] = [
  {
    id: "pdf",
    name: "PDF Pages",
    description: "Rendered PDF pages",
    size_bytes: 2048,
    file_count: 3,
  },
  {
    id: "downloads",
    name: "Downloads",
    description: "Generated downloads",
    size_bytes: 0,
    file_count: 0,
  },
  {
    id: "covers",
    name: "Covers",
    description: "Resized covers",
    size_bytes: 4096,
    file_count: 5,
  },
];

// The id of the cache being cleared, or null when no clear is in flight.
let clearingId: string | null = null;

vi.mock("@/hooks/queries/cache", () => ({
  useCaches: () => ({ data: caches, isLoading: false, error: null }),
  useClearCache: () => ({
    mutateAsync: vi.fn(),
    isPending: clearingId !== null,
    variables: clearingId ?? undefined,
  }),
}));

const clearButton = (name: string) =>
  screen.getByRole("button", { name: `Clear ${name} cache` });

describe("AdminCache Clear button", () => {
  beforeEach(() => {
    setAuth({ permissions: ["config:read", "config:write"] });
    clearingId = null;
  });

  it("is disabled for an empty cache and enabled for one with files", () => {
    render(<AdminCache />);

    expect(clearButton("Downloads")).toBeDisabled();
    expect(clearButton("PDF Pages")).toBeEnabled();
    expect(clearButton("Covers")).toBeEnabled();
  });

  it("is disabled only for the cache being cleared", () => {
    const { rerender } = render(<AdminCache />);
    expect(clearButton("PDF Pages")).toBeEnabled();

    clearingId = "pdf";
    rerender(<AdminCache />);

    expect(clearButton("PDF Pages")).toBeDisabled();
    expect(clearButton("PDF Pages")).toHaveTextContent("Clearing...");
    expect(clearButton("Covers")).toBeEnabled();
  });
});
