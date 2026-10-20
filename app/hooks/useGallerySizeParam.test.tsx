import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ContentWidth, ContentWidthProvider } from "@/contexts/ContentWidth";
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

const renderAt = (entry: string, onChange?: () => void, width?: number) => {
  if (width !== undefined) {
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(
      width,
    );
  }
  const wrapper = ({ children }: { children: ReactNode }) => (
    <MemoryRouter initialEntries={[entry]}>
      {width === undefined ? (
        children
      ) : (
        <ContentWidthProvider>
          <ContentWidth>{children}</ContentWidth>
        </ContentWidthProvider>
      )}
    </MemoryRouter>
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
    expect(result.current.gallery.pageSizeResolved).toBe(true);
    expect(result.current.gallery.currentPage).toBe(1);
    expect(result.current.gallery.offset).toBe(0);
  });

  it("prefers the URL size and reports it as dirty", () => {
    const { result } = renderAt("/?size=l");

    expect(result.current.gallery.effectiveSize).toBe("l");
    expect(result.current.gallery.isSizeDirty).toBe(true);
  });

  it("fills three rows of the columns the content width fits", () => {
    const { result } = renderAt("/", undefined, 1660);

    expect(result.current.gallery.columns).toBe(11);
    expect(result.current.gallery.itemsPerPage).toBe(33);
  });

  it("waits until the page's content has been measured", () => {
    const wrapper = ({ children }: { children: ReactNode }) => (
      <MemoryRouter>
        <ContentWidthProvider>{children}</ContentWidthProvider>
      </MemoryRouter>
    );
    const { result } = renderHook(() => useGallerySizeParam(), { wrapper });

    expect(result.current.pageSizeResolved).toBe(false);
  });

  it("shows twelve rows of two on a phone", () => {
    vi.stubGlobal(
      "matchMedia",
      vi.fn((query: string) => ({
        matches: query === "(max-width: 639px)",
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    );
    const { result } = renderAt("/");

    expect(result.current.gallery.columns).toBe(2);
    expect(result.current.gallery.itemsPerPage).toBe(24);
    vi.unstubAllGlobals();
  });

  it("opens the page holding the start item", () => {
    const { result } = renderAt("/?start=50");

    expect(result.current.gallery.itemsPerPage).toBe(24);
    expect(result.current.gallery.offset).toBe(48);
    expect(result.current.gallery.currentPage).toBe(3);
  });

  it("opens an old page-number link on the same page", () => {
    const { result } = renderAt("/?page=3");

    expect(result.current.gallery.offset).toBe(48);
    expect(result.current.gallery.currentPage).toBe(3);
  });

  it("replaces an old page number when moving to another page", () => {
    const { result } = renderAt("/?page=3");

    act(() => result.current.gallery.goToPage(1));
    expect(result.current.search).toBe("");

    act(() => result.current.gallery.goToPage(2));
    expect(result.current.search).toBe("?start=24");
  });

  it("keeps the start item on the page when the page size changes", () => {
    const { result } = renderAt("/?start=50&size=l");

    expect(result.current.gallery.itemsPerPage).toBe(18);
    expect(result.current.gallery.offset).toBe(36);
    expect(result.current.gallery.currentPage).toBe(3);
  });

  it("reports an unresolved page size until the settings query settles", () => {
    settings.saved = undefined;
    const { result } = renderAt("/");

    expect(result.current.gallery.pageSizeResolved).toBe(false);
  });

  it("names a page by the index of its first item", () => {
    const { result } = renderAt("/?size=l");

    act(() => result.current.gallery.goToPage(3));
    expect(new URLSearchParams(result.current.search).get("start")).toBe("36");

    act(() => result.current.gallery.goToPage(1));
    expect(result.current.search).toBe("?size=l");
  });

  it("keeps the start item when the size changes", () => {
    const onChange = vi.fn();
    const { result } = renderAt("/?start=50", onChange);

    act(() => result.current.gallery.applyGallerySize("s"));

    const params = new URLSearchParams(result.current.search);
    expect(params.get("size")).toBe("s");
    expect(params.get("start")).toBe("50");
    expect(onChange).toHaveBeenCalledWith("s");
  });

  it("drops the size param when it matches the saved size", () => {
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
