import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useFillTimeZone } from "@/hooks/useFillTimeZone";
import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

let savedZone: string | null = null;

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient()}>
    {children}
  </QueryClientProvider>
);

const zonePuts = () =>
  vi
    .mocked(API.request)
    .mock.calls.filter(([method]) => method === "PUT")
    .map(([, endpoint, payload]) => ({ endpoint, payload }));

const browserZone = Intl.DateTimeFormat().resolvedOptions().timeZone;

describe("useFillTimeZone", () => {
  beforeEach(() => {
    setAuth();
    savedZone = null;
    vi.spyOn(API, "request").mockImplementation(((method: string) =>
      Promise.resolve(
        method === "GET"
          ? { gallery_size: "m", time_zone: savedZone }
          : { gallery_size: "m", time_zone: browserZone },
      )) as never);
  });

  it("fills an unset time zone from the browser", async () => {
    renderHook(() => useFillTimeZone(), { wrapper });
    await vi.waitFor(() =>
      expect(zonePuts()).toEqual([
        { endpoint: "/settings/user", payload: { time_zone: browserZone } },
      ]),
    );
  });

  it("leaves a chosen time zone alone", async () => {
    savedZone = "Asia/Tokyo";
    renderHook(() => useFillTimeZone(), { wrapper });
    await vi.waitFor(() =>
      expect(vi.mocked(API.request)).toHaveBeenCalledWith(
        "GET",
        "/settings/user",
        null,
        null,
        expect.anything(),
      ),
    );
    await Promise.resolve();
    expect(zonePuts()).toEqual([]);
  });

  it("saves nothing in Demo Mode or signed out", async () => {
    setAuth({ demoMode: true });
    renderHook(() => useFillTimeZone(), { wrapper });
    setAuth({ user: null });
    renderHook(() => useFillTimeZone(), { wrapper });
    await vi.advanceTimersByTimeAsync(100);
    expect(zonePuts()).toEqual([]);
  });
});
