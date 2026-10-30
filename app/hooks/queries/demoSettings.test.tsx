import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type {
  BookReadingResponse,
  LibrarySettingsResponse,
  UserSettingsResponse,
} from "@/types";

import {
  getDemoLibrarySettingsKey,
  QueryKey as LibrarySettingsQueryKey,
  useLibrarySettings,
  useUpdateLibrarySettings,
} from "./librarySettings";
import {
  getDemoPageFlowKey,
  QueryKey as ReadingQueryKey,
  useBookReading,
  useSetPageFlow,
} from "./reading";
import {
  DEMO_USER_SETTINGS_KEY,
  QueryKey as UserSettingsQueryKey,
  useUpdateUserSettings,
  useUserSettings,
} from "./settings";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

setAuth({ demoMode: true });

const userDefaults: UserSettingsResponse = {
  fit_mode: "fit-width",
  gallery_size: "m",
  preload_count: 2,
  viewer_reflowable_flow: "paginated",
  viewer_reflowable_font_size: 100,
  viewer_reflowable_theme: "light",
  viewer_hide_chrome: false,
  viewer_playback_speed: 1,
};

const createWrapper = (queryClient: QueryClient) =>
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };

describe("Demo Mode settings", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    localStorage.clear();
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    vi.restoreAllMocks();
  });

  it("seeds user settings from the server, then updates local storage and cache", async () => {
    const request = vi.spyOn(API, "request").mockResolvedValue(userDefaults);
    const wrapper = createWrapper(queryClient);
    const settings = renderHook(() => useUserSettings(), { wrapper });

    await waitFor(() => expect(settings.result.current.isSuccess).toBe(true));
    expect(settings.result.current.data).toEqual(userDefaults);
    expect(
      JSON.parse(localStorage.getItem(DEMO_USER_SETTINGS_KEY) ?? ""),
    ).toEqual(userDefaults);

    const update = renderHook(() => useUpdateUserSettings(), { wrapper });
    await act(async () => {
      await update.result.current.mutateAsync({ gallery_size: "xl" });
    });

    const expected = { ...userDefaults, gallery_size: "xl" };
    expect(request).toHaveBeenCalledTimes(1);
    expect(
      JSON.parse(localStorage.getItem(DEMO_USER_SETTINGS_KEY) ?? ""),
    ).toEqual(expected);
    expect(
      queryClient.getQueryData([UserSettingsQueryKey.UserSettings]),
    ).toEqual(expected);
  });

  it("uses saved per-library settings and writes updates without a PUT", async () => {
    const libraryId = 42;
    const serverDefaults: LibrarySettingsResponse = {
      sort_spec: "title:asc",
    };
    const savedSettings: LibrarySettingsResponse = {
      sort_spec: "created_at:desc",
    };
    localStorage.setItem(
      getDemoLibrarySettingsKey(libraryId),
      JSON.stringify(savedSettings),
    );
    const request = vi.spyOn(API, "request").mockResolvedValue(serverDefaults);
    const wrapper = createWrapper(queryClient);
    const settings = renderHook(() => useLibrarySettings(libraryId), {
      wrapper,
    });

    await waitFor(() => expect(settings.result.current.isSuccess).toBe(true));
    expect(settings.result.current.data).toEqual(savedSettings);

    const update = renderHook(() => useUpdateLibrarySettings(libraryId), {
      wrapper,
    });
    await act(async () => {
      await update.result.current.mutateAsync({ sort_spec: null });
    });

    expect(request).toHaveBeenCalledTimes(1);
    expect(
      JSON.parse(
        localStorage.getItem(getDemoLibrarySettingsKey(libraryId)) ?? "",
      ),
    ).toEqual({ sort_spec: null });
    expect(
      queryClient.getQueryData([
        LibrarySettingsQueryKey.LibrarySettings,
        libraryId,
      ]),
    ).toEqual({ sort_spec: null });
  });

  it("keeps a book's page flow in local storage and saves it without a PUT", async () => {
    setAuth({ demoMode: true, permissions: ["books:read"] });
    const bookId = 7;
    const serverReading: BookReadingResponse = {
      book_id: bookId,
      status: null,
      rating: null,
      on_to_read: false,
      to_read_place: null,
      read_throughs: [],
      page_flow: null,
    };
    const request = vi.spyOn(API, "request").mockResolvedValue(serverReading);
    const wrapper = createWrapper(queryClient);
    const reading = renderHook(() => useBookReading(bookId), { wrapper });
    await waitFor(() => expect(reading.result.current.isSuccess).toBe(true));
    expect(reading.result.current.data?.page_flow).toBeNull();

    const update = renderHook(() => useSetPageFlow(), { wrapper });
    await act(async () => {
      await update.result.current.mutateAsync({
        bookId,
        payload: { page_flow: "scrolled" },
      });
    });

    expect(request).toHaveBeenCalledTimes(1);
    expect(
      JSON.parse(localStorage.getItem(getDemoPageFlowKey(bookId)) ?? ""),
    ).toEqual({ page_flow: "scrolled" });
    expect(
      queryClient.getQueryData<BookReadingResponse>([
        ReadingQueryKey.BookReading,
        bookId,
      ])?.page_flow,
    ).toBe("scrolled");

    // A later visit reads the saved flow over the server's.
    const fresh = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const reread = renderHook(() => useBookReading(bookId), {
      wrapper: createWrapper(fresh),
    });
    await waitFor(() => expect(reread.result.current.isSuccess).toBe(true));
    expect(reread.result.current.data).toEqual({
      ...serverReading,
      page_flow: "scrolled",
    });
  });

  it("leaves no stored page flow for a book whose flow was never chosen, and ignores a value that isn't a flow", async () => {
    setAuth({ demoMode: true, permissions: ["books:read"] });
    const reading = (bookId: number): BookReadingResponse => ({
      book_id: bookId,
      status: null,
      rating: null,
      on_to_read: false,
      to_read_place: null,
      read_throughs: [],
      page_flow: null,
    });
    vi.spyOn(API, "request").mockImplementation(async (_method, path) =>
      reading(Number(String(path).split("/").pop())),
    );
    localStorage.setItem(
      getDemoPageFlowKey(9),
      JSON.stringify({ page_flow: "sideways" }),
    );
    const wrapper = createWrapper(queryClient);

    const untouched = renderHook(() => useBookReading(8), { wrapper });
    await waitFor(() => expect(untouched.result.current.isSuccess).toBe(true));
    expect(localStorage.getItem(getDemoPageFlowKey(8))).toBeNull();

    const corrupt = renderHook(() => useBookReading(9), { wrapper });
    await waitFor(() => expect(corrupt.result.current.isSuccess).toBe(true));
    expect(corrupt.result.current.data?.page_flow).toBeNull();
  });
});
