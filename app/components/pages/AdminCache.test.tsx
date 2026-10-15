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
import type {
  Info as CacheInfo,
  SettingsResponse,
} from "@/types/generated/cache";

import AdminCache from "./AdminCache";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const caches: CacheInfo[] = [
  {
    id: "pdf_pages",
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
    file_count: 1200,
  },
];

// The id of the cache being cleared, or null when no clear is in flight.
let clearingId: string | null = null;
const settings: SettingsResponse = {
  cover_thumbnails: { max_size_gb: 1, unlimited: false },
  downloads: { max_size_gb: 5, unlimited: false },
  cbz_pages: { max_size_gb: 5, unlimited: true },
  pdf_pages: { max_size_gb: 5, unlimited: true },
};
const updateSettings = vi.fn(async () => settings);

vi.mock("@/hooks/queries/cache", () => ({
  useCaches: () => ({ data: caches, isLoading: false, error: null }),
  useClearCache: () => ({
    mutateAsync: vi.fn(),
    isPending: clearingId !== null,
    variables: clearingId ?? undefined,
  }),
  useCacheSettings: () => ({
    data: settings,
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

// The limit controls of one cache. Each control's name says which cache it
// belongs to, so a screen reader user moving between cards can tell them apart.
const card = (name: string) => ({
  size: () =>
    screen.getByRole("spinbutton", { name: `Maximum size (GiB) for ${name}` }),
  noLimit: () => screen.getByRole("checkbox", { name: `No limit for ${name}` }),
  save: () => screen.getByRole("button", { name: `Save limit for ${name}` }),
  querySave: () =>
    screen.queryByRole("button", { name: `Save limit for ${name}` }),
});

describe("AdminCache Clear button", () => {
  beforeEach(() => {
    setAuth({ permissions: ["config:read", "config:write"] });
    clearingId = null;
    updateSettings.mockReset();
    updateSettings.mockResolvedValue(settings);
    vi.mocked(toast.error).mockClear();
  });

  it("groups thousands in a cache's file count", () => {
    renderPage();

    expect(screen.getByText("1,200 files")).toBeInTheDocument();
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

    clearingId = "pdf_pages";
    await act(async () => {
      await router.navigate("/?clearing=pdf_pages");
    });

    expect(clearButton("PDF Pages")).toBeDisabled();
    expect(clearButton("PDF Pages")).toHaveTextContent("Clearing...");
    expect(clearButton("Covers")).toBeEnabled();
  });

  it("lets an administrator save a fractional limit for one cache", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPage();
    const covers = card("Covers");
    expect(covers.size()).toHaveValue(1);
    expect(card("Downloads").size()).toHaveValue(5);
    await user.clear(covers.size());
    await user.type(covers.size(), "2.5");
    expect(card("Downloads").save()).toBeDisabled();
    await user.click(covers.save());
    expect(updateSettings).toHaveBeenCalledWith({
      cover_thumbnails: { max_size_gb: 2.5, unlimited: false },
    });
    expect(toast.success).toHaveBeenCalledWith("Covers cache limit saved");
  });

  it("lifts a limit and keeps its size for later", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPage();
    const downloads = card("Downloads");
    expect(downloads.noLimit()).not.toBeChecked();
    await user.click(downloads.noLimit());
    expect(downloads.size()).toBeDisabled();
    expect(downloads.size()).toHaveValue(5);
    await user.click(downloads.save());
    expect(updateSettings).toHaveBeenCalledWith({
      downloads: { max_size_gb: 5, unlimited: true },
    });
  });

  it("sets a limit on a cache that had none", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPage();
    const pdf = card("PDF Pages");
    expect(pdf.noLimit()).toBeChecked();
    expect(pdf.size()).toBeDisabled();
    expect(pdf.save()).toBeDisabled();
    await user.click(pdf.noLimit());
    await user.clear(pdf.size());
    await user.type(pdf.size(), "3");
    await user.click(pdf.save());
    expect(updateSettings).toHaveBeenCalledWith({
      pdf_pages: { max_size_gb: 3, unlimited: false },
    });
  });

  it("shows the limits without allowing read-only users to change them", () => {
    setAuth({ permissions: ["config:read"] });
    renderPage();
    expect(screen.getByRole("note")).toHaveTextContent(
      "You can view these caches but not change their limits or clear them.",
    );
    for (const name of ["Covers", "Downloads", "PDF Pages"]) {
      expect(card(name).size()).toBeDisabled();
      expect(card(name).noLimit()).toBeDisabled();
      expect(card(name).querySave()).toBeNull();
    }
  });

  it("explains an empty or out-of-range limit and keeps Save disabled", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPage();
    const covers = card("Covers");
    expect(covers.size()).not.toHaveAttribute("aria-invalid");
    await user.clear(covers.size());
    expect(covers.save()).toBeDisabled();
    expect(covers.size()).toHaveAttribute("aria-invalid", "true");
    expect(covers.size()).toHaveAccessibleDescription(
      "Enter a size from 0 to 1024 GiB.",
    );
    await user.type(covers.size(), "1025");
    expect(covers.save()).toBeDisabled();
    expect(covers.size()).toHaveAccessibleDescription(
      "Enter a size from 0 to 1024 GiB.",
    );
    await user.clear(covers.size());
    await user.type(covers.size(), "3");
    expect(covers.size()).not.toHaveAttribute("aria-invalid");
    expect(covers.save()).toBeEnabled();
    expect(updateSettings).not.toHaveBeenCalled();
  });

  it("keeps the draft and reports a failed save", async () => {
    updateSettings.mockRejectedValueOnce(new Error("offline"));
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPage();
    const covers = card("Covers");
    await user.clear(covers.size());
    await user.type(covers.size(), "2.5");
    await user.click(covers.save());
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "Failed to save cache limit",
        undefined,
      ),
    );
    expect(covers.size()).toHaveValue(2.5);
  });

  it("protects an edited limit when leaving the page", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const router = renderPage();
    const downloads = card("Downloads");
    await user.click(downloads.noLimit());
    await act(async () => {
      await router.navigate("/other");
    });
    expect(
      screen.getByRole("dialog", { name: "Unsaved Changes" }),
    ).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Stay" }));
    expect(downloads.noLimit()).toBeChecked();
    await act(async () => {
      await router.navigate("/other");
    });
    await user.click(screen.getByRole("button", { name: "Discard Changes" }));
    expect(screen.getByText("Elsewhere")).toBeVisible();
  });
});
