import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { describe, expect, it } from "vitest";

import { useUpdateSearchParams } from "./useUpdateSearchParams";

const wrapper = ({ children }: { children: ReactNode }) => (
  <MemoryRouter initialEntries={["/books?q=x"]}>{children}</MemoryRouter>
);

const toggle = (key: string, value: string) => (params: URLSearchParams) => {
  const values = (params.get(key) ?? "").split(",").filter(Boolean);
  const next = values.includes(value)
    ? values.filter((v) => v !== value)
    : [...values, value];
  if (next.length > 0) params.set(key, next.join(","));
  else params.delete(key);
};

describe("useUpdateSearchParams", () => {
  it("applies an update to the URL", () => {
    const { result } = renderHook(
      () => ({ update: useUpdateSearchParams(), location: useLocation() }),
      { wrapper },
    );
    act(() => result.current.update(toggle("types", "epub")));
    expect(result.current.location.search).toBe("?q=x&types=epub");
  });

  // Two clicks can land before the first navigation renders. The second
  // builds on the first rather than on the URL the page last rendered.
  it("keeps every update made before the page renders again", () => {
    const { result } = renderHook(
      () => ({ update: useUpdateSearchParams(), location: useLocation() }),
      { wrapper },
    );
    const update = result.current.update;
    act(() => {
      update(toggle("types", "epub"));
      update(toggle("types", "cbz"));
    });
    expect(result.current.location.search).toBe("?q=x&types=epub%2Ccbz");

    act(() => result.current.update(toggle("types", "epub")));
    expect(result.current.location.search).toBe("?q=x&types=cbz");
  });
});
