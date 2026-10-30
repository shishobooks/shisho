import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import {
  MemoryRouter,
  useLocation,
  useNavigate,
  useSearchParams,
} from "react-router-dom";
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

  const renderWithOthers = () =>
    renderHook(
      () => ({
        update: useUpdateSearchParams(),
        location: useLocation(),
        navigate: useNavigate(),
        setSearchParams: useSearchParams()[1],
      }),
      { wrapper },
    );

  // Toggling one value on and off fast leaves the URL as it was. A change
  // made elsewhere afterwards (a search, a sort) must survive the next
  // update.
  it("builds on a change made elsewhere after its own updates", () => {
    const { result } = renderWithOthers();
    const update = result.current.update;
    act(() => {
      update(toggle("types", "epub"));
      update(toggle("types", "epub"));
    });
    expect(result.current.location.search).toBe("?q=x");

    act(() => result.current.setSearchParams({ q: "y" }));
    act(() => result.current.update(toggle("types", "cbz")));
    expect(result.current.location.search).toBe("?q=y&types=cbz");
  });

  it("builds on the URL Back returns to", () => {
    const { result } = renderWithOthers();
    act(() => result.current.update(toggle("types", "epub")));
    expect(result.current.location.search).toBe("?q=x&types=epub");

    act(() => result.current.navigate(-1));
    expect(result.current.location.search).toBe("?q=x");
    act(() => result.current.update(toggle("types", "cbz")));
    expect(result.current.location.search).toBe("?q=x&types=cbz");
  });

  it("builds on a change made elsewhere after an update that changed nothing", () => {
    const { result } = renderWithOthers();
    act(() => result.current.update(() => {}));
    act(() => result.current.setSearchParams({ q: "x", sort: "title:asc" }));
    act(() => result.current.update(toggle("types", "epub")));
    expect(result.current.location.search).toBe(
      "?q=x&sort=title%3Aasc&types=epub",
    );
  });

  // A save that finishes later calls the update it captured when clicked.
  // It must build on the URL as it is by then.
  it("builds on the latest URL when called from an earlier render", () => {
    const { result } = renderWithOthers();
    const captured = result.current.update;
    act(() => result.current.update(toggle("types", "epub")));
    act(() => captured((params) => params.set("page", "2")));
    expect(result.current.location.search).toBe("?q=x&types=epub&page=2");
  });

  it("adds no history entry for an update that changes nothing", () => {
    const { result } = renderWithOthers();
    const key = result.current.location.key;
    act(() => result.current.update(() => {}));
    expect(result.current.location.key).toBe(key);
  });
});
