import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { toast } from "sonner";
import { beforeAll, describe, expect, it, vi } from "vitest";

import { useReflowableBlob } from "@/hooks/queries/reflowable";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { rejectingMutate, REJECTION_MESSAGE } from "@/testing/mutations";

import ReflowableReader from "./ReflowableReader";

// Prevent jsdom from trying to execute the real foliate view.js (it uses
// browser-only module specifiers and dynamic imports that jsdom can't resolve).
vi.mock("@/libraries/foliate/view.js", () => ({}));

vi.mock("@/hooks/queries/reflowable", () => ({
  useReflowableBlob: vi.fn(),
}));

// The table of contents the mocked foliate view reports once a book opens.
let mockToc: unknown[] = [];

beforeAll(() => {
  if (!customElements.get("foliate-view")) {
    customElements.define(
      "foliate-view",
      class extends HTMLElement {
        open = vi.fn().mockResolvedValue(undefined);
        goLeft = vi.fn();
        goRight = vi.fn();
        goTo = vi.fn();
        goToFraction = vi.fn();
        book = { toc: mockToc };
      },
    );
  }
});

vi.mock("@/hooks/queries/settings", () => ({
  useUserSettings: vi.fn(() => ({ data: undefined, isLoading: true })),
  useUpdateUserSettings: vi.fn(() => ({ mutate: vi.fn() })),
}));

const renderReader = (fileType = "epub") => {
  const client = new QueryClient();
  const file = {
    id: 7,
    book_id: 3,
    file_type: fileType,
  } as never;
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ReflowableReader bookTitle="Test Book" file={file} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

// The invisible page-turn tap zones, which assistive tech skips.
const pageTapZones = () =>
  screen
    .queryAllByRole("button", { hidden: true })
    .filter((button) => button.getAttribute("aria-hidden") === "true");

describe("ReflowableReader", () => {
  it("shows the heading as the current entry when the location is under it", async () => {
    mockToc = [
      {
        label: "Part Two",
        href: "",
        subitems: [{ label: "Chapter 3", href: "ch3.xhtml" }],
      },
    ];
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    try {
      const { container } = renderReader();
      const menu = (await screen.findByRole("combobox", {
        name: "Jump to chapter",
      })) as HTMLSelectElement;
      // foliate reports a heading as the current item with a null href.
      act(() => {
        container.querySelector("foliate-view")!.dispatchEvent(
          new CustomEvent("relocate", {
            detail: {
              fraction: 0.1,
              tocItem: { label: "Part Two", href: null },
            },
          }),
        );
      });
      expect(menu.selectedOptions[0]?.textContent).toBe("Part Two");
    } finally {
      mockToc = [];
    }
  });

  it("lists headings in the chapter menu without making them selectable", async () => {
    mockToc = [
      {
        label: "Part Two",
        href: "",
        subitems: [{ label: "Chapter 3", href: "ch3.xhtml" }],
      },
      { label: "Afterword", href: "after.xhtml" },
    ];
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    try {
      renderReader();
      const menu = await screen.findByRole("combobox", {
        name: "Jump to chapter",
      });
      // The first option is the placeholder shown before a location is known.
      const options = (
        Array.from(menu.querySelectorAll("option")) as HTMLOptionElement[]
      ).slice(1);
      const labels = options.map((o) => o.textContent?.trim());
      expect(labels).toEqual(["Part Two", "Chapter 3", "Afterword"]);
      expect(options[0].disabled).toBe(true);
      expect(options[1].disabled).toBe(false);
      // Nested chapters are indented under their heading.
      expect(options[1].textContent).toMatch(/^\u00a0+Chapter 3$/);
      expect(options[2].textContent).toBe("Afterword");
    } finally {
      mockToc = [];
    }
  });

  it("shows a loading indicator while fetching the book", () => {
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    renderReader();
    expect(screen.getByText(/preparing book/i)).toBeInTheDocument();
  });

  it("shows an error state with a retry button on fetch failure", () => {
    const refetch = vi.fn();
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error("boom"),
      isEnabled: true,
      refetch,
    } as never);

    renderReader();
    expect(screen.getByText(/couldn't load/i)).toBeInTheDocument();
    // A rejection that did not come from the API says nothing useful.
    expect(screen.getByText("Failed to load book")).toBeInTheDocument();
    expect(screen.queryByText("boom")).not.toBeInTheDocument();
    screen.getByRole("button", { name: /retry/i }).click();
    expect(refetch).toHaveBeenCalled();
  });

  it("keeps an open book on screen when a background refetch fails", () => {
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: true,
      error: new TypeError("Failed to fetch"),
      isEnabled: true,
      refetch: vi.fn(),
    } as never);

    renderReader();
    expect(screen.queryByText(/couldn't load/i)).not.toBeInTheDocument();
  });

  it("opens a MOBI file in foliate under its own type", async () => {
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/x-mobipocket-ebook" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    const { container } = renderReader("mobi");
    const view = container.querySelector("foliate-view");
    await act(async () => {});

    expect(view).not.toBeNull();
    const opened = vi.mocked(view!.open).mock.calls[0][0] as globalThis.File;
    expect(opened.name).toBe("book.mobi");
    expect(opened.type).toBe("application/x-mobipocket-ebook");
  });

  it("shows the extended-wait hint after 10 seconds of loading", () => {
    vi.useFakeTimers();
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    renderReader();
    expect(screen.queryByText(/may take a moment/i)).not.toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(10_000);
    });
    expect(screen.getByText(/may take a moment/i)).toBeInTheDocument();

    vi.useRealTimers();
  });

  it("hides the page tap zones in scrolled flow mode", () => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: {
        preload_count: 3,
        fit_mode: "fit-height",
        viewer_reflowable_font_size: 100,
        viewer_reflowable_theme: "light",
        viewer_reflowable_flow: "scrolled",
      },
      isLoading: false,
    } as never);

    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    renderReader();
    expect(pageTapZones()).toHaveLength(0);
  });

  it("shows the page tap zones in paginated flow mode", () => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: {
        preload_count: 3,
        fit_mode: "fit-height",
        viewer_reflowable_font_size: 100,
        viewer_reflowable_theme: "light",
        viewer_reflowable_flow: "paginated",
      },
      isLoading: false,
    } as never);

    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    renderReader();
    expect(pageTapZones()).toHaveLength(2);
    // A mouse press does not move focus onto a hidden tap zone.
    for (const zone of pageTapZones()) {
      expect(fireEvent.mouseDown(zone)).toBe(false);
    }
    // The tap zones are hidden from assistive tech, so each page button has
    // one named control: the footer button.
    expect(
      screen.getByRole("button", { name: "Previous page" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Next page" }),
    ).toBeInTheDocument();
  });

  it("hides chrome toggle button in scrolled flow even with auto-hide enabled", () => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: {
        preload_count: 3,
        fit_mode: "fit-height",
        viewer_reflowable_font_size: 100,
        viewer_reflowable_theme: "light",
        viewer_reflowable_flow: "scrolled",
        viewer_hide_chrome: true,
      },
      isLoading: false,
    } as never);

    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    renderReader();
    expect(
      screen.queryByRole("button", { name: /toggle controls/i }),
    ).not.toBeInTheDocument();
  });

  it("updates settings when the theme button is clicked", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const mutate = vi.fn();
    vi.mocked(useUserSettings).mockReturnValue({
      data: {
        preload_count: 3,
        fit_mode: "fit-height",
        viewer_reflowable_font_size: 100,
        viewer_reflowable_theme: "light",
        viewer_reflowable_flow: "paginated",
      },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({ mutate } as never);

    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    renderReader();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await user.click(await screen.findByRole("button", { name: /settings/i }));
    await user.click(screen.getByRole("button", { name: /dark/i }));
    expect(mutate).toHaveBeenCalledWith(
      expect.objectContaining({ viewer_reflowable_theme: "dark" }),
      expect.anything(),
    );
  });

  it("toasts when saving a setting fails", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const error = vi.spyOn(toast, "error");
    vi.mocked(useUserSettings).mockReturnValue({
      data: {
        preload_count: 3,
        fit_mode: "fit-height",
        viewer_reflowable_font_size: 100,
        viewer_reflowable_theme: "light",
        viewer_reflowable_flow: "paginated",
      },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateUserSettings).mockReturnValue({
      mutate: rejectingMutate(),
    } as never);
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    renderReader();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await user.click(await screen.findByRole("button", { name: /settings/i }));
    await user.click(screen.getByRole("button", { name: /dark/i }));

    expect(error).toHaveBeenCalledWith(REJECTION_MESSAGE, undefined);
  });

  it("seeks to the start and end from the keyboard on the progress bar", async () => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: {
        preload_count: 3,
        fit_mode: "fit-height",
        viewer_reflowable_font_size: 100,
        viewer_reflowable_theme: "light",
        viewer_reflowable_flow: "paginated",
      },
      isLoading: false,
    } as never);
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    renderReader();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const progress = screen.getByRole("slider", { name: "Reading progress" });
    expect(progress).toHaveAttribute("aria-valuenow", "0");
    const view = document.querySelector("foliate-view") as unknown as {
      goToFraction: ReturnType<typeof vi.fn>;
    };

    act(() => progress.focus());
    await user.keyboard("{End}");
    expect(view.goToFraction).toHaveBeenLastCalledWith(1);
    await user.keyboard("{Home}");
    expect(view.goToFraction).toHaveBeenLastCalledWith(0);
  });

  it("names the font size slider", async () => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: {
        preload_count: 3,
        fit_mode: "fit-height",
        viewer_reflowable_font_size: 100,
        viewer_reflowable_theme: "light",
        viewer_reflowable_flow: "paginated",
      },
      isLoading: false,
    } as never);
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    renderReader();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await user.click(screen.getByRole("button", { name: "Settings" }));

    expect(
      await screen.findByRole("slider", { name: "Font size" }),
    ).toHaveAttribute("aria-valuenow", "100");
  });
});
