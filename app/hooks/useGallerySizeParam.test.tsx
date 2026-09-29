import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { rejectingMutate, REJECTION_MESSAGE } from "@/testing/mutations";

import { useGallerySizeParam } from "./useGallerySizeParam";

const settings = vi.hoisted(() => ({
  saved: "m" as string | undefined,
  mutate: undefined as undefined | ((...args: never[]) => void),
}));

vi.mock("@/hooks/queries/settings", () => ({
  useUserSettings: () => ({
    data: settings.saved ? { gallery_size: settings.saved } : undefined,
    isSuccess: settings.saved !== undefined,
    isError: false,
  }),
  useUpdateUserSettings: () => ({
    mutate: settings.mutate ?? vi.fn(),
    isPending: false,
  }),
}));

afterEach(() => {
  settings.saved = "m";
  settings.mutate = undefined;
  vi.restoreAllMocks();
});

const renderAt = (entry: string, onChange?: () => void) => {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <MemoryRouter initialEntries={[entry]}>{children}</MemoryRouter>
  );
  return renderHook(
    () => ({
      gallery: useGallerySizeParam({ onChange }),
      search: useLocation().search,
    }),
    { wrapper },
  );
};

describe("useGallerySizeParam", () => {
  it("uses the saved size when the URL has none", () => {
    const { result } = renderAt("/");

    expect(result.current.gallery.effectiveSize).toBe("m");
    expect(result.current.gallery.isSizeDirty).toBe(false);
    expect(result.current.gallery.settingsResolved).toBe(true);
    expect(result.current.gallery.currentPage).toBe(1);
    expect(result.current.gallery.offset).toBe(0);
  });

  it("prefers the URL size and reports it as dirty", () => {
    const { result } = renderAt("/?size=l&page=3");

    expect(result.current.gallery.effectiveSize).toBe("l");
    expect(result.current.gallery.isSizeDirty).toBe(true);
    expect(result.current.gallery.currentPage).toBe(3);
    expect(result.current.gallery.offset).toBe(
      2 * result.current.gallery.itemsPerPage,
    );
  });

  it("reports unresolved settings until the settings query settles", () => {
    settings.saved = undefined;
    const { result } = renderAt("/");

    expect(result.current.gallery.settingsResolved).toBe(false);
  });

  it("keeps the first visible item in view when the size changes", () => {
    const onChange = vi.fn();
    const { result } = renderAt("/?page=3", onChange);
    const { offset } = result.current.gallery;

    act(() => result.current.gallery.applyGallerySize("s"));

    const params = new URLSearchParams(result.current.search);
    expect(params.get("size")).toBe("s");
    const page = Number(params.get("page") ?? "1");
    expect(page).toBeGreaterThan(1);
    expect(onChange).toHaveBeenCalledWith("s", page);
    // The first item that was in view is on the new page.
    expect(offset).toBeGreaterThanOrEqual(
      (page - 1) * result.current.gallery.itemsPerPage,
    );
  });

  it("drops the size and page params when they match the defaults", () => {
    const { result } = renderAt("/?size=l");

    act(() => result.current.gallery.applyGallerySize("m"));

    expect(result.current.search).toBe("");
  });

  it("removes the size param once the size is saved", () => {
    settings.mutate = vi.fn(
      (_vars: unknown, options?: { onSuccess?: () => void }) =>
        options?.onSuccess?.(),
    );
    const { result } = renderAt("/?size=l");

    act(() => result.current.gallery.saveSizeAsDefault());

    expect(settings.mutate).toHaveBeenCalledWith(
      { gallery_size: "l" },
      expect.anything(),
    );
    expect(result.current.search).toBe("");
  });

  it("toasts and keeps the size param when saving fails", () => {
    const error = vi.spyOn(toast, "error");
    settings.mutate = rejectingMutate();
    const { result } = renderAt("/?size=l");

    act(() => result.current.gallery.saveSizeAsDefault());

    expect(error).toHaveBeenCalledWith(REJECTION_MESSAGE, undefined);
    expect(result.current.search).toBe("?size=l");
  });
});
