import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  createMemoryRouter,
  RouterProvider,
  useLocation,
} from "react-router-dom";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";
import type { Info as CacheInfo } from "@/types/generated/cache";

import AdminCache from "./AdminCache";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

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
    id: "cover_thumbnails",
    name: "Covers",
    description: "Resized covers",
    size_bytes: 4096,
    file_count: 5,
  },
];

// The id of the cache being cleared, or null when no clear is in flight.
let clearingId: string | null = null;
const updateSettings = vi.fn(async () => ({
  cover_thumbnail_max_size_gb: 2.5,
}));

vi.mock("@/hooks/queries/cache", () => ({
  useCaches: () => ({ data: caches, isLoading: false, error: null }),
  useClearCache: () => ({
    mutateAsync: vi.fn(),
    isPending: clearingId !== null,
    variables: clearingId ?? undefined,
  }),
  useCacheSettings: () => ({
    data: { cover_thumbnail_max_size_gb: 1 },
    isLoading: false,
    error: null,
  }),
  useUpdateCacheSettings: () => ({
    mutateAsync: updateSettings,
    isPending: false,
  }),
}));

const renderPage = () => {
  const CacheRoute = () => {
    useLocation();
    return <AdminCache />;
  };
  const router = createMemoryRouter([
    { path: "/", element: <CacheRoute /> },
    { path: "/other", element: <p>Elsewhere</p> },
  ]);
  render(<RouterProvider router={router} />);
  return router;
};

const clearButton = (name: string) =>
  screen.getByRole("button", { name: `Clear ${name} cache` });

describe("AdminCache Clear button", () => {
  beforeEach(() => {
    setAuth({ permissions: ["config:read", "config:write"] });
    clearingId = null;
    updateSettings.mockReset();
    updateSettings.mockResolvedValue({ cover_thumbnail_max_size_gb: 2.5 });
    vi.mocked(toast.error).mockClear();
  });

  it("is disabled for an empty cache and enabled for one with files", () => {
    renderPage();

    expect(clearButton("Downloads")).toBeDisabled();
    expect(clearButton("PDF Pages")).toBeEnabled();
    expect(clearButton("Covers")).toBeEnabled();
  });

  it("is disabled only for the cache being cleared", async () => {
    const router = renderPage();
    expect(clearButton("PDF Pages")).toBeEnabled();

    clearingId = "pdf";
    await act(async () => {
      await router.navigate("/?clearing=pdf");
    });

    expect(clearButton("PDF Pages")).toBeDisabled();
    expect(clearButton("PDF Pages")).toHaveTextContent("Clearing...");
    expect(clearButton("Covers")).toBeEnabled();
  });

  it("lets an administrator save a fractional cover-thumbnail limit", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPage();
    const limit = screen.getByRole("spinbutton", {
      name: "Maximum size (GiB)",
    });
    expect(limit).toHaveValue(1);
    await user.clear(limit);
    await user.type(limit, "2.5");
    await user.click(screen.getByRole("button", { name: "Save limit" }));
    expect(updateSettings).toHaveBeenCalledWith({
      cover_thumbnail_max_size_gb: 2.5,
    });
  });

  it("shows the limit without allowing read-only users to change it", () => {
    setAuth({ permissions: ["config:read"] });
    renderPage();
    expect(
      screen.getByRole("spinbutton", { name: "Maximum size (GiB)" }),
    ).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Save limit" })).toBeNull();
  });

  it("keeps Save disabled for an empty or out-of-range limit", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPage();
    const limit = screen.getByRole("spinbutton", {
      name: "Maximum size (GiB)",
    });
    await user.clear(limit);
    expect(screen.getByRole("button", { name: "Save limit" })).toBeDisabled();
    await user.type(limit, "1025");
    expect(screen.getByRole("button", { name: "Save limit" })).toBeDisabled();
    expect(updateSettings).not.toHaveBeenCalled();
  });

  it("keeps the draft and reports a failed save", async () => {
    updateSettings.mockRejectedValueOnce(new Error("offline"));
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPage();
    const limit = screen.getByRole("spinbutton", {
      name: "Maximum size (GiB)",
    });
    await user.clear(limit);
    await user.type(limit, "2.5");
    await user.click(screen.getByRole("button", { name: "Save limit" }));
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "Failed to save cache limit",
        undefined,
      ),
    );
    expect(limit).toHaveValue(2.5);
  });
  it("protects an edited limit when leaving the page", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const router = renderPage();
    const limit = screen.getByRole("spinbutton", {
      name: "Maximum size (GiB)",
    });
    await user.clear(limit);
    await user.type(limit, "2.5");
    await act(async () => {
      await router.navigate("/other");
    });
    expect(
      screen.getByRole("dialog", { name: "Unsaved Changes" }),
    ).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Stay" }));
    expect(limit).toHaveValue(2.5);
    await act(async () => {
      await router.navigate("/other");
    });
    await user.click(screen.getByRole("button", { name: "Discard Changes" }));
    expect(screen.getByText("Elsewhere")).toBeVisible();
  });
});
