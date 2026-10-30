import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { toast } from "sonner";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { useReflowableBlob } from "@/hooks/queries/reflowable";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { POSITION_SAVE_DELAY_MS } from "@/hooks/useReadingProgress";
import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import { rejectingMutate, REJECTION_MESSAGE } from "@/testing/mutations";
import type { UpdatePositionPayload } from "@/types";

import ReflowableReader from "./ReflowableReader";

// Prevent jsdom from trying to execute the real foliate view.js (it uses
// browser-only module specifiers and dynamic imports that jsdom can't resolve).
vi.mock("@/libraries/foliate/view.js", () => ({}));

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/hooks/queries/reflowable", () => ({
  useReflowableBlob: vi.fn(),
}));

// The table of contents the mocked foliate view reports once a book opens.
let mockToc: unknown[] = [];
// Whether the mocked renderer is at the end of the book.
let mockAtEnd = false;
// The saved position GET /reading/files/:id/position returns.
let savedCfi: string | null = null;
const SAVED_AT = "2026-10-01T09:00:00.000Z";
// What POST /reading/files/:id/position reports it did.
let saveResult: object = {};
let positionLoadFails = false;

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
        init = vi.fn().mockResolvedValue(undefined);
        renderer = {
          get atEnd() {
            return mockAtEnd;
          },
          setStyles: vi.fn(),
          setAttribute: vi.fn(),
        };
        lastLocation = { cfi: "epubcfi(/6/2!/4/2)", fraction: 0 };
        get book() {
          return { toc: mockToc };
        }
      },
    );
  }
});

vi.mock("@/hooks/queries/settings", () => ({
  useUserSettings: vi.fn(() => ({ data: undefined, isLoading: true })),
  useUpdateUserSettings: vi.fn(() => ({ mutate: vi.fn() })),
}));

beforeEach(() => {
  setAuth({ permissions: ["books:read"] });
  mockAtEnd = false;
  savedCfi = null;
  saveResult = {};
  positionLoadFails = false;
  vi.spyOn(API, "request").mockImplementation(((
    method: string,
    endpoint: string,
  ) => {
    if (method === "GET" && endpoint === "/reading/files/7/position") {
      if (positionLoadFails) return Promise.reject(new Error("offline"));
      return Promise.resolve({
        position: savedCfi
          ? {
              file_id: 7,
              cfi: savedCfi,
              percentage: 0.4,
              occurred_at: SAVED_AT,
            }
          : null,
      });
    }
    return Promise.resolve({
      book_id: 3,
      accepted: true,
      started: false,
      reverted: false,
      finished_read_through_id: null,
      ...saveResult,
    });
  }) as never);
});

const renderReader = (
  fileType = "epub",
  name?: string,
  client = new QueryClient(),
) => {
  const file = {
    id: 7,
    book_id: 3,
    file_type: fileType,
    name,
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
  it("sets the tab title to the file's name", () => {
    vi.mocked(useReflowableBlob).mockReturnValue({
      isLoading: true,
      isError: false,
      refetch: vi.fn(),
    } as never);
    renderReader("epub", "File Name");
    expect(document.title).toBe("Reading: File Name - Shisho");
  });

  it.each([undefined, ""])(
    "sets the tab title to the book's title when the file's name is %j",
    (name) => {
      vi.mocked(useReflowableBlob).mockReturnValue({
        isLoading: true,
        isError: false,
        refetch: vi.fn(),
      } as never);
      renderReader("epub", name);
      expect(document.title).toBe("Reading: Test Book - Shisho");
    },
  );

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
    expect(view).not.toBeNull();
    // The book opens once the saved location has loaded.
    await vi.waitFor(() => expect(vi.mocked(view!.open)).toHaveBeenCalled());

    const opened = vi.mocked(view!.open).mock.calls[0][0] as globalThis.File;
    expect(opened.name).toBe("book.mobi");
    expect(opened.type).toBe("application/x-mobipocket-ebook");
  });

  // foliate throws if asked to turn a page before a book has opened.
  it("disables the page buttons until the book is ready", () => {
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    renderReader();
    expect(
      screen.getByRole("button", { name: "Previous page" }),
    ).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next page" })).toBeDisabled();
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

  describe("clicks inside the book", () => {
    // foliate reports each loaded section's document in a "load" event.
    const loadSection = (container: HTMLElement) => {
      const doc = document.implementation.createHTMLDocument("section");
      doc.body.innerHTML = '<p>Text <a href="#n">note</a></p>';
      act(() => {
        container
          .querySelector("foliate-view")!
          .dispatchEvent(new CustomEvent("load", { detail: { doc } }));
      });
      return doc;
    };
    const header = () => screen.getByRole("banner");

    const renderWithAutoHide = (flow: "paginated" | "scrolled") => {
      vi.mocked(useUserSettings).mockReturnValue({
        data: {
          viewer_reflowable_font_size: 100,
          viewer_reflowable_theme: "light",
          viewer_reflowable_flow: flow,
          viewer_hide_chrome: true,
        },
        isLoading: false,
      } as never);
      vi.mocked(useReflowableBlob).mockReturnValue({
        data: undefined,
        isLoading: true,
        isError: false,
        error: null,
        refetch: vi.fn(),
      } as never);
      return renderReader();
    };

    it("toggle the controls in scrolled flow", () => {
      const { container } = renderWithAutoHide("scrolled");
      const doc = loadSection(container);
      expect(header()).not.toHaveClass("-translate-y-full");

      act(() => doc.querySelector("p")!.click());
      expect(header()).toHaveClass("-translate-y-full");

      act(() => doc.querySelector("p")!.click());
      expect(header()).not.toHaveClass("-translate-y-full");
    });

    it("show hidden controls on a tap in scrolled flow", () => {
      vi.useFakeTimers();
      const { container } = renderWithAutoHide("scrolled");
      const doc = loadSection(container);
      act(() => vi.advanceTimersByTime(2000));
      expect(header()).toHaveClass("-translate-y-full");

      // A tap sends compatibility mouse events before its click.
      const p = doc.querySelector("p")!;
      act(() => {
        p.dispatchEvent(
          Object.assign(new Event("pointermove", { bubbles: true }), {
            pointerType: "touch",
          }),
        );
        p.dispatchEvent(new Event("mousemove", { bubbles: true }));
        p.click();
      });
      expect(header()).not.toHaveClass("-translate-y-full");
      vi.useRealTimers();
    });

    it("leave the controls alone when following a link", () => {
      const { container } = renderWithAutoHide("scrolled");
      const doc = loadSection(container);

      act(() => doc.querySelector("a")!.click());
      expect(header()).not.toHaveClass("-translate-y-full");
    });

    it("leave the controls to the tap zones in paginated flow", () => {
      const { container } = renderWithAutoHide("paginated");
      const doc = loadSection(container);

      act(() => doc.querySelector("p")!.click());
      expect(header()).not.toHaveClass("-translate-y-full");
    });
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

describe("ReflowableReader resuming", () => {
  const openBook = async (client?: QueryClient) => {
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);
    const { container } = renderReader("epub", undefined, client);
    const view = container.querySelector(
      "foliate-view",
    ) as unknown as HTMLElement & { init: ReturnType<typeof vi.fn> };
    await vi.waitFor(() => expect(view.init).toHaveBeenCalled());
    // Let init's promise settle so the reader leaves its restoring state.
    await act(async () => {});
    return view;
  };

  const relocate = (view: HTMLElement, detail: object) =>
    act(() => {
      view.dispatchEvent(new CustomEvent("relocate", { detail }));
    });

  const positionSaves = () =>
    vi
      .mocked(API.request)
      .mock.calls.filter(
        ([method, endpoint]) =>
          method === "POST" && endpoint === "/reading/files/7/position",
      )
      .map(([, , payload]) => payload as UpdatePositionPayload);

  it("opens at the saved location", async () => {
    savedCfi = "epubcfi(/6/14!/4/2/10)";
    const view = await openBook();
    expect(view.init).toHaveBeenCalledWith({
      lastLocation: "epubcfi(/6/14!/4/2/10)",
    });
  });

  it("opens at the start without a saved location", async () => {
    const view = await openBook();
    expect(view.init).toHaveBeenCalledWith({ showTextStart: false });
  });

  it("saves where the reader moves, a few seconds later", async () => {
    const view = await openBook();
    relocate(view, {
      cfi: "epubcfi(/6/4!/4/2)",
      fraction: 0.1,
      reason: "page",
    });
    relocate(view, {
      cfi: "epubcfi(/6/6!/4/2)",
      fraction: 0.2,
      reason: "page",
    });
    expect(positionSaves()).toEqual([]);

    await act(async () => {
      vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS);
    });
    expect(positionSaves()).toEqual([
      expect.objectContaining({ cfi: "epubcfi(/6/6!/4/2)", percentage: 0.2 }),
    ]);
  });

  it("doesn't save a re-layout that keeps the place", async () => {
    const view = await openBook();
    relocate(view, {
      cfi: "epubcfi(/6/2!/4/4)",
      fraction: 0.01,
      reason: "anchor",
    });
    await act(async () => {
      vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS * 2);
    });
    expect(positionSaves()).toEqual([]);
  });

  it("saves at once when it reaches the end", async () => {
    const view = await openBook();
    mockAtEnd = true;
    relocate(view, { cfi: "epubcfi(/6/40!/4/2)", fraction: 1, reason: "page" });
    expect(positionSaves()).toEqual([
      expect.objectContaining({ cfi: "epubcfi(/6/40!/4/2)", at_end: true }),
    ]);
  });

  it("saves at once when the page is hidden", async () => {
    const view = await openBook();
    relocate(view, {
      cfi: "epubcfi(/6/4!/4/2)",
      fraction: 0.1,
      reason: "page",
    });
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => "hidden",
    });
    try {
      act(() => {
        document.dispatchEvent(new Event("visibilitychange"));
      });
      expect(positionSaves()).toEqual([
        expect.objectContaining({ cfi: "epubcfi(/6/4!/4/2)" }),
      ]);
    } finally {
      Object.defineProperty(document, "visibilityState", {
        configurable: true,
        get: () => "visible",
      });
    }
  });

  it("saves nothing in Demo Mode", async () => {
    setAuth({ permissions: ["books:read"], demoMode: true });
    const view = await openBook();
    relocate(view, {
      cfi: "epubcfi(/6/4!/4/2)",
      fraction: 0.1,
      reason: "page",
    });
    await act(async () => {
      vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS);
    });
    expect(positionSaves()).toEqual([]);
  });

  it("opens at the place saved on the server, not one cached earlier", async () => {
    savedCfi = "epubcfi(/6/30!/4/2)";
    const client = new QueryClient();
    client.setQueryData(["FilePosition", 7], {
      position: { file_id: 7, cfi: "epubcfi(/6/4!/4/2)", percentage: 0.1 },
    });
    const view = await openBook(client);
    expect(view.init).toHaveBeenCalledWith({
      lastLocation: "epubcfi(/6/30!/4/2)",
    });
  });

  it("sends the saved place unchanged until the reader moves", async () => {
    savedCfi = "epubcfi(/6/14!/4/2/10)";
    // foliate reports the start of the page it laid out, which differs
    // from the saved location on another screen size.
    const view = await openBook();
    act(() => {
      vi.advanceTimersByTime(60_000);
      window.dispatchEvent(new Event("pagehide"));
    });
    expect(positionSaves()).toEqual([
      expect.objectContaining({
        cfi: "epubcfi(/6/14!/4/2/10)",
        percentage: 0.4,
        occurred_at: SAVED_AT,
        session: expect.anything(),
      }),
    ]);
    expect(view).toBeTruthy();
  });

  it("saves nothing when the saved place failed to load", async () => {
    positionLoadFails = true;
    const error = vi.spyOn(toast, "error");
    const view = await openBook();
    expect(view.init).toHaveBeenCalledWith({ showTextStart: false });
    expect(error).toHaveBeenCalledWith(
      "Failed to load your reading position",
      undefined,
    );
    relocate(view, {
      cfi: "epubcfi(/6/4!/4/2)",
      fraction: 0.1,
      reason: "page",
    });
    await act(async () => {
      vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS);
    });
    expect(positionSaves()).toEqual([]);
  });

  it("takes the finish toast away when going back undoes the finish", async () => {
    const dismiss = vi.spyOn(toast, "dismiss");
    const view = await openBook();
    saveResult = { reverted: true };
    relocate(view, {
      cfi: "epubcfi(/6/20!/4/2)",
      fraction: 0.5,
      reason: "page",
    });
    await act(async () => {
      vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS);
    });
    expect(dismiss).toHaveBeenCalledWith("reading-finished-3");
  });
});
