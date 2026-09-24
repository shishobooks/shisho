import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import type { LibrarySettingsResponse, UserSettingsResponse } from "@/types";

import {
  getDemoLibrarySettingsKey,
  QueryKey as LibrarySettingsQueryKey,
  useLibrarySettings,
  useUpdateLibrarySettings,
} from "./librarySettings";
import {
  DEMO_USER_SETTINGS_KEY,
  QueryKey as UserSettingsQueryKey,
  useUpdateUserSettings,
  useUserSettings,
} from "./settings";

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({ demoMode: true }),
}));

const userDefaults: UserSettingsResponse = {
  fit_mode: "fit-width",
  gallery_size: "m",
  preload_count: 2,
  viewer_epub_flow: "paginated",
  viewer_epub_font_size: 100,
  viewer_epub_theme: "light",
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
});
