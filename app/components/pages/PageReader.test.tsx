import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router-dom";
import { toast } from "sonner";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import { POSITION_SAVE_DELAY_MS } from "@/hooks/useReadingProgress";
import { setAuth } from "@/testing/auth";
import { rejectingMutate, REJECTION_MESSAGE } from "@/testing/mutations";
import type { PageFlow } from "@/types";

import PageReader from "./PageReader";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const settings = vi.hoisted(() => ({
  mutate: undefined as unknown as (...args: never[]) => void,
  hideChrome: false,
}));

vi.mock("@/hooks/queries/settings", () => ({
  useUserSettings: () => ({
    data: {
      preload_count: 3,
      fit_mode: "fit-height",
      viewer_hide_chrome: settings.hideChrome,
    },
    isLoading: false,
  }),
  useUpdateUserSettings: () => ({ mutate: settings.mutate }),
}));

const chapters = vi.hoisted(() => ({ data: [] as unknown[] }));

vi.mock("@/hooks/queries/chapters", () => ({
  useFileChapters: () => ({ data: chapters.data }),
}));

// The saved flow, the flow the page shapes suggest, and the save mutation.
const flow = vi.hoisted(() => ({
  saved: null as PageFlow | null,
  detected: "paginated" as PageFlow | undefined,
  detectedFrom: undefined as string[] | null | undefined,
  readingLoading: false,
  mutate: undefined as unknown as (...args: never[]) => void,
}));

// The saved position, whether it has loaded, and what saves were sent.
const position = vi.hoisted(() => ({
  savedPage: null as number | null,
  loading: false,
  loadFails: false,
  saves: [] as Array<Record<string, unknown>>,
}));

vi.mock("@/hooks/queries/reading", () => ({
  QueryKey: { FilePosition: "FilePosition" },
  invalidateAfterPositionUpdate: () => {},
  markProgressStale: () => {},
  useUndoAutoFinish: () => ({ mutate: () => {} }),
  useFilePosition: () => ({
    data:
      position.loading || position.loadFails
        ? undefined
        : {
            position:
              position.savedPage === null
                ? null
                : { page: position.savedPage, percentage: 0.5 },
          },
    error: position.loadFails ? new Error("offline") : null,
    isEnabled: true,
    isFetching: position.loading,
    isFetchedAfterMount: !position.loading,
  }),
  saveFilePosition: (_fileId: number, payload: Record<string, unknown>) => {
    position.saves.push(payload);
    return Promise.resolve({
      book_id: 7,
      accepted: true,
      started: false,
      resumed: false,
      reverted: false,
      finished_read_through_id: null,
    });
  },
  useBookReading: (bookId?: number) => ({
    data:
      bookId && !flow.readingLoading
        ? { book_id: bookId, page_flow: flow.saved }
        : undefined,
    isLoading: Boolean(bookId) && flow.readingLoading,
  }),
  useSetPageFlow: () => ({ mutate: flow.mutate }),
}));

vi.mock("@/hooks/useDetectedPageFlow", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/useDetectedPageFlow")>()),
  useDetectedPageFlow: (urls: string[] | null) => {
    flow.detectedFrom = urls;
    return urls ? flow.detected : undefined;
  },
}));

const scrollIntoView = vi.fn();

beforeAll(() => {
  // jsdom has no layout, so it leaves scrollTo unimplemented.
  Element.prototype.scrollTo = () => {};
  HTMLElement.prototype.scrollIntoView = scrollIntoView;
});

afterEach(() => {
  vi.restoreAllMocks();
  scrollIntoView.mockClear();
  flow.saved = null;
  flow.detected = "paginated";
  flow.detectedFrom = undefined;
  flow.readingLoading = false;
  settings.hideChrome = false;
  chapters.data = [];
  position.savedPage = null;
  position.loading = false;
  position.loadFails = false;
});

beforeEach(() => {
  setAuth({ permissions: ["books:read"] });
  // The last test's reader closes after its afterEach, and saves its session.
  position.saves = [];
  settings.mutate = vi.fn();
});

// Shows the reader's URL query, which it updates as the page changes.
function LocationProbe() {
  const location = useLocation();
  return (
    <output data-state={JSON.stringify(location.state)} data-testid="location">
      {location.search}
    </output>
  );
}

const queryClient = new QueryClient();

const getPageUrl = (page: number, size?: number) =>
  size ? `/page-${page}?size=${size}` : `/page-${page}`;

const renderReader = (
  totalPages = 10,
  title?: string,
  initialPath: string | { search: string; state: unknown } = "/",
) =>
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialPath]}>
        <PageReader
          bookId={7}
          fileId={42}
          getPageUrl={getPageUrl}
          libraryId="1"
          title={title}
          totalPages={totalPages}
        />
        <LocationProbe />
      </MemoryRouter>
    </QueryClientProvider>,
  );

const openSettings = async (user: ReturnType<typeof userEvent.setup>) =>
  user.click(screen.getByRole("button", { name: "Reader settings" }));

describe("PageReader title", () => {
  it("sets the tab title to the title it is given", () => {
    settings.mutate = vi.fn();
    renderReader(10, "File Name");
    expect(document.title).toBe("Reading: File Name - Shisho");
  });
});

describe("PageReader settings", () => {
  it("toasts when saving a reader setting fails", async () => {
    const error = vi.spyOn(toast, "error");
    settings.mutate = rejectingMutate();
    const user = userEvent.setup();
    renderReader();

    await user.click(screen.getByRole("button", { name: "Reader settings" }));
    await user.click(await screen.findByRole("button", { name: "Fit Width" }));

    expect(settings.mutate).toHaveBeenCalled();
    expect(error).toHaveBeenCalledWith(REJECTION_MESSAGE, {
      id: "reader-settings-error",
    });
  });
});

describe("PageReader fit mode", () => {
  it("groups the fit mode buttons under their label and marks the chosen one pressed", async () => {
    settings.mutate = vi.fn();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader();

    await user.click(screen.getByRole("button", { name: "Reader settings" }));
    const group = await screen.findByRole("group", { name: "Fit Mode" });
    expect(
      within(group).getByRole("button", { name: "Fit Height" }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(
      within(group).getByRole("button", { name: "Fit Width" }),
    ).toHaveAttribute("aria-pressed", "false");
  });
});

describe("PageReader controls", () => {
  it("groups thousands in the page count", () => {
    settings.mutate = vi.fn();
    renderReader(1200);

    expect(screen.getByText("Page 1 of 1,200")).toBeInTheDocument();
    expect(
      screen.getByRole("slider", { name: "Reading progress" }),
    ).toHaveAttribute("aria-valuetext", "Page 1 of 1,200");
  });

  it("seeks to the first and last page from the keyboard on the progress bar", async () => {
    settings.mutate = vi.fn();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader();

    const progress = screen.getByRole("slider", { name: "Reading progress" });
    expect(progress).toHaveAttribute("aria-valuetext", "Page 1 of 10");

    act(() => progress.focus());
    await user.keyboard("{End}");
    expect(screen.getByText("Page 10 of 10")).toBeInTheDocument();
    expect(progress).toHaveAttribute("aria-valuenow", "10");
    await user.keyboard("{Home}");
    expect(screen.getByText("Page 1 of 10")).toBeInTheDocument();
  });

  it("names the page buttons and the preload slider", async () => {
    settings.mutate = vi.fn();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader();

    await user.click(screen.getByRole("button", { name: "Next page" }));
    expect(screen.getByText("Page 2 of 10")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Previous page" }));
    expect(screen.getByText("Page 1 of 10")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Reader settings" }));
    expect(
      await screen.findByRole("slider", { name: "Preload count" }),
    ).toHaveAttribute("aria-valuenow", "3");
  });
});

describe("PageReader flow", () => {
  it("shows every page in one column when the book's saved flow is scrolled", async () => {
    flow.saved = "scrolled";
    settings.mutate = vi.fn();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader(5);

    expect(
      screen.getAllByRole("img").map((img) => img.getAttribute("src")),
    ).toEqual(["/page-0", "/page-1", "/page-2", "/page-3", "/page-4"]);
    // A saved flow needs no probing.
    expect(flow.detectedFrom).toBeNull();

    await openSettings(user);
    const group = await screen.findByRole("group", { name: "Flow" });
    expect(
      within(group).getByRole("button", { name: "Scrolled" }),
    ).toHaveAttribute("aria-pressed", "true");
    // Fit modes size a single page, so scrolling hides them.
    expect(screen.queryByRole("group", { name: "Fit Mode" })).toBeNull();
  });

  it("probes thumbnails of the pages after the cover when the book has no saved flow", () => {
    flow.detected = "scrolled";
    settings.mutate = vi.fn();
    renderReader(5);

    expect(flow.detectedFrom).toEqual(["/page-1?size=256", "/page-2?size=256"]);
    expect(screen.getAllByRole("img")).toHaveLength(5);
  });

  it("shows a single page when the probed pages are printed pages", () => {
    flow.detected = "paginated";
    settings.mutate = vi.fn();
    renderReader(5);

    expect(screen.getAllByRole("img")).toHaveLength(1);
    expect(screen.getByRole("img")).toHaveAttribute("src", "/page-0");
  });

  it("shows no page until it knows the flow", () => {
    flow.readingLoading = true;
    flow.detected = undefined;
    settings.mutate = vi.fn();
    const { rerender } = renderReader(5);

    expect(screen.queryAllByRole("img")).toHaveLength(0);
    expect(flow.detectedFrom).toBeNull();

    flow.readingLoading = false;
    rerender(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <PageReader
            bookId={7}
            fileId={42}
            getPageUrl={getPageUrl}
            libraryId="1"
            totalPages={5}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    // Now probing, still nothing shown.
    expect(flow.detectedFrom).toEqual(["/page-1?size=256", "/page-2?size=256"]);
    expect(screen.queryAllByRole("img")).toHaveLength(0);
  });

  it("saves the flow chosen for the book and switches to it", async () => {
    flow.detected = "scrolled";
    settings.mutate = vi.fn();
    flow.mutate = vi.fn();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader(5);

    await openSettings(user);
    await user.click(await screen.findByRole("button", { name: "Paginated" }));

    expect(flow.mutate).toHaveBeenCalledWith(
      { bookId: 7, payload: { page_flow: "paginated" } },
      expect.anything(),
    );
    expect(screen.getAllByRole("img")).toHaveLength(1);
    expect(settings.mutate).not.toHaveBeenCalled();
  });

  it("goes back to the stored flow and toasts when saving the flow fails", async () => {
    const error = vi.spyOn(toast, "error");
    flow.saved = "scrolled";
    settings.mutate = vi.fn();
    flow.mutate = rejectingMutate();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader(5);

    await openSettings(user);
    await user.click(await screen.findByRole("button", { name: "Paginated" }));

    expect(error).toHaveBeenCalledWith(REJECTION_MESSAGE, {
      id: "reader-settings-error",
    });
    expect(screen.getAllByRole("img")).toHaveLength(5);
  });

  it("scrolls to the page in the URL and to each page the controls go to", async () => {
    flow.saved = "scrolled";
    settings.mutate = vi.fn();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader(5, undefined, "/?page=2");

    const scrolledTo = () =>
      within(
        scrollIntoView.mock.contexts[
          scrollIntoView.mock.contexts.length - 1
        ] as HTMLElement,
      ).getByRole("img");
    expect(scrolledTo()).toHaveAttribute("alt", "Page 3");
    expect(screen.getByText("Page 3 of 5")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Next page" }));
    expect(screen.getByText("Page 4 of 5")).toBeInTheDocument();
    expect(scrolledTo()).toHaveAttribute("alt", "Page 4");
  });
});

describe("PageReader page in the URL", () => {
  it.each([
    ["/?page=99", "Page 5 of 5"],
    ["/?page=-3", "Page 1 of 5"],
  ])("keeps %s within the file's pages", (path, label) => {
    settings.mutate = vi.fn();
    renderReader(5, undefined, path);
    expect(screen.getByText(label)).toBeInTheDocument();
  });

  it("writes the page to the URL once scrolling settles in the scrolled flow", async () => {
    flow.saved = "scrolled";
    settings.mutate = vi.fn();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader(5);

    await user.click(screen.getByRole("button", { name: "Next page" }));
    await user.click(screen.getByRole("button", { name: "Next page" }));
    expect(screen.getByText("Page 3 of 5")).toBeInTheDocument();
    // Browsers cap how often a page may rewrite its URL, and scrolling
    // changes the page every frame.
    expect(screen.getByTestId("location")).toHaveTextContent(/^$/);

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(screen.getByTestId("location")).toHaveTextContent("?page=2");
  });
});

describe("PageReader preloading", () => {
  // Stands in for Image, reporting a load only when a test says so.
  const stubImages = () => {
    const created: Array<{ src: string; onload: (() => void) | null }> = [];
    vi.stubGlobal(
      "Image",
      class {
        src = "";
        onload: (() => void) | null = null;
        onerror: (() => void) | null = null;
        constructor() {
          created.push(this);
        }
      },
    );
    return created;
  };
  afterEach(() => vi.unstubAllGlobals());

  // Safari ignores rel="prefetch", and a prefetch is only a low-priority hint
  // anywhere else. Queuing them all at once would also queue PDF renders on the
  // server, which do one page at a time.
  it("loads the pages past the neighbors one at a time, nearest first", () => {
    settings.mutate = vi.fn();
    const created = stubImages();
    renderReader(10, undefined, "/?page=5");

    // The neighbors load as the reader's own images, so preload_count 3 leaves
    // two pages on each side.
    expect(created.map((image) => image.src)).toEqual(["/page-7"]);
    act(() => created[0].onload?.());
    expect(created.map((image) => image.src)).toEqual(["/page-7", "/page-3"]);
    act(() => created[1].onload?.());
    act(() => created[2].onload?.());
    act(() => created[3].onload?.());
    expect(created.map((image) => image.src)).toEqual([
      "/page-7",
      "/page-3",
      "/page-8",
      "/page-2",
    ]);
  });

  it("preloads nothing in the scrolled flow", () => {
    settings.mutate = vi.fn();
    flow.saved = "scrolled";
    const created = stubImages();
    renderReader(5);
    expect(created).toHaveLength(0);
  });
});

describe("PageReader auto-hide in the scrolled flow", () => {
  it("offers a control to toggle the hidden controls from the keyboard", () => {
    flow.saved = "scrolled";
    settings.hideChrome = true;
    settings.mutate = vi.fn();
    renderReader(5);
    expect(
      screen.getByRole("button", { name: "Toggle controls" }),
    ).toBeInTheDocument();
  });
});

describe("PageReader chapter menu", () => {
  it("shows the current chapter and jumps to the one chosen", async () => {
    chapters.data = [
      { id: 1, title: "Opening", start_page: 0 },
      { id: 2, title: "Middle", start_page: 4 },
    ];
    settings.mutate = vi.fn();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader(10);

    const menu = screen.getByRole("combobox", { name: "Jump to chapter" });
    expect(menu).toHaveTextContent(/^Opening$/);

    await user.click(menu);
    await user.click(await screen.findByRole("option", { name: "Middle" }));
    expect(screen.getByText("Page 5 of 10")).toBeInTheDocument();
    expect(menu).toHaveTextContent(/^Middle$/);
  });
});

describe("PageReader keyboard", () => {
  it("turns pages with the arrow keys, but not while a control has them", async () => {
    chapters.data = [{ id: 1, title: "Opening", start_page: 0 }];
    settings.mutate = vi.fn();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader(10);

    fireEvent.keyDown(document.body, { key: "ArrowRight" });
    expect(screen.getByText("Page 2 of 10")).toBeInTheDocument();

    const menu = screen.getByRole("combobox", { name: "Jump to chapter" });
    fireEvent.keyDown(menu, { key: "ArrowRight" });
    fireEvent.keyDown(menu, { key: "d" });
    expect(screen.getByText("Page 2 of 10")).toBeInTheDocument();

    await openSettings(user);
    fireEvent.keyDown(
      await screen.findByRole("slider", { name: "Preload count" }),
      { key: "ArrowRight" },
    );
    expect(screen.getByText("Page 2 of 10")).toBeInTheDocument();
  });
});

describe("PageReader swiping", () => {
  const touch = (screenX: number, screenY: number) => ({ screenX, screenY });
  const swipe = (
    target: Element,
    from: [number, number],
    to: [number, number],
  ) => {
    fireEvent.touchStart(target, {
      touches: [touch(...from)],
      changedTouches: [touch(...from)],
    });
    fireEvent.touchEnd(target, {
      touches: [],
      changedTouches: [touch(...to)],
    });
    // A turn finishes once the page has slid into place.
    act(() => vi.advanceTimersByTime(500));
  };
  const openOnPage = (page: number) => {
    settings.mutate = vi.fn();
    renderReader(10, undefined, `/?page=${page}`);
    return screen.getByRole("main");
  };

  it("turns to the next page on a swipe to the left", () => {
    swipe(openOnPage(3), [300, 400], [100, 410]);
    expect(screen.getByTestId("location")).toHaveTextContent("page=4");
  });

  it("turns to the previous page on a swipe to the right", () => {
    swipe(openOnPage(3), [100, 400], [300, 410]);
    expect(screen.getByTestId("location")).toHaveTextContent("page=2");
  });

  it("turns the page when the swipe starts on a tap zone", () => {
    const main = openOnPage(3);
    const zone = main.querySelector('button[aria-hidden="true"]')!;
    swipe(zone, [100, 400], [300, 400]);
    expect(screen.getByTestId("location")).toHaveTextContent("page=2");
  });

  it("ignores a short drag and a mostly vertical one", () => {
    const main = openOnPage(3);
    swipe(main, [300, 400], [270, 400]);
    swipe(main, [300, 400], [200, 700]);
    expect(screen.getByTestId("location")).toHaveTextContent("page=3");
  });

  it("ignores a gesture with a second finger, such as a pinch", () => {
    const main = openOnPage(3);
    fireEvent.touchStart(main, {
      touches: [touch(300, 400)],
      changedTouches: [touch(300, 400)],
    });
    fireEvent.touchStart(main, {
      touches: [touch(300, 400), touch(100, 400)],
      changedTouches: [touch(100, 400)],
    });
    fireEvent.touchEnd(main, {
      touches: [touch(100, 400)],
      changedTouches: [touch(100, 400)],
    });
    fireEvent.touchEnd(main, { touches: [], changedTouches: [touch(90, 400)] });
    expect(screen.getByTestId("location")).toHaveTextContent("page=3");
  });

  describe("sliding", () => {
    const drag = (target: Element, from: number, to: number, y = 400) => {
      fireEvent.touchStart(target, {
        touches: [touch(from, y)],
        changedTouches: [touch(from, y)],
      });
      fireEvent.touchMove(target, {
        touches: [touch(to, y)],
        changedTouches: [touch(to, y)],
      });
    };
    const release = (target: Element, x: number, y = 400) =>
      fireEvent.touchEnd(target, {
        touches: [],
        changedTouches: [touch(x, y)],
      });
    const track = () => screen.getByTestId("page-track");
    const pageImage = (page: number) =>
      document.querySelector(`main img[src="/page-${page}"]`);

    it("has both neighbors on the page, hidden from assistive tech", () => {
      openOnPage(3);
      expect(pageImage(2)).not.toBeNull();
      expect(pageImage(4)).not.toBeNull();
      // Only the current page is announced.
      expect(screen.getAllByRole("img")).toHaveLength(1);
      expect(screen.getByRole("img")).toHaveAttribute("src", "/page-3");
    });

    it("follows the finger", () => {
      const main = openOnPage(3);
      drag(main, 300, 220);
      expect(track().style.transform).toBe("translateX(-80px)");
    });

    it("doesn't follow a drag that starts out vertical", () => {
      const main = openOnPage(3);
      fireEvent.touchStart(main, {
        touches: [touch(300, 400)],
        changedTouches: [touch(300, 400)],
      });
      fireEvent.touchMove(main, {
        touches: [touch(298, 450)],
        changedTouches: [touch(298, 450)],
      });
      fireEvent.touchMove(main, {
        touches: [touch(200, 460)],
        changedTouches: [touch(200, 460)],
      });
      expect(track().style.transform).toBe("");
    });

    it("resists at the first page, where there's nothing to show", () => {
      const main = openOnPage(0);
      drag(main, 100, 200);
      expect(track().style.transform).toBe("translateX(25px)");
    });

    it("springs back from a drag too short to turn", () => {
      const main = openOnPage(3);
      drag(main, 300, 270);
      release(main, 270);
      act(() => vi.advanceTimersByTime(500));
      expect(track().style.transform).toBe("translateX(0px)");
      expect(screen.getByTestId("location")).toHaveTextContent("page=3");
    });

    it("waits for the slide, then keeps the neighbor's own image", () => {
      const main = openOnPage(3);
      const next = pageImage(4);
      drag(main, 300, 150);
      release(main, 150);
      expect(screen.getByTestId("location")).toHaveTextContent("page=3");

      act(() => vi.advanceTimersByTime(500));
      expect(screen.getByTestId("location")).toHaveTextContent("page=4");
      expect(track().style.transform).toBe("");
      // The same element, so the browser has nothing to load again.
      expect(pageImage(4)).toBe(next);
      expect(screen.getByRole("img")).toBe(next);
    });

    it("takes a new swipe that starts while the last one is landing", () => {
      const main = openOnPage(3);
      drag(main, 300, 150);
      release(main, 150);
      act(() => vi.advanceTimersByTime(50));

      drag(main, 300, 150);
      expect(track().style.transform).toBe("translateX(-150px)");
      release(main, 150);
      act(() => vi.advanceTimersByTime(500));
      expect(screen.getByTestId("location")).toHaveTextContent("page=5");
    });

    it("clears the offsets it used, so they can't add blank scroll", () => {
      const main = openOnPage(3);
      Object.defineProperty(main, "scrollTop", {
        value: 1000,
        configurable: true,
      });
      drag(main, 300, 150);
      expect(track().style.getPropertyValue("--panel-top")).toBe("1000px");
      release(main, 150);
      act(() => vi.advanceTimersByTime(500));
      expect(track().style.getPropertyValue("--panel-top")).toBe("");
      expect(track().style.getPropertyValue("--panel-height")).toBe("");
    });

    it("asks for the current page first, ahead of its neighbors", () => {
      openOnPage(3);
      const sources = Array.from(document.querySelectorAll("main img")).map(
        (image) => image.getAttribute("src"),
      );
      expect(sources[0]).toBe("/page-3");
    });

    // A neighbor that failed off-screen fires no more events by itself.
    it("retries a neighbor that failed, rather than spinning forever", () => {
      const main = openOnPage(3);
      const next = pageImage(4) as HTMLImageElement;
      Object.defineProperty(next, "complete", { value: true });
      Object.defineProperty(next, "naturalWidth", { value: 0 });
      const assigned = vi.spyOn(next, "src", "set");

      drag(main, 300, 150);
      release(main, 150);
      act(() => vi.advanceTimersByTime(500));
      expect(assigned).toHaveBeenCalledWith("/page-4");
      expect(document.querySelector('main [role="status"]')).not.toBeNull();

      fireEvent.error(next);
      expect(document.querySelector('main [role="status"]')).toBeNull();
    });

    // touch-action pan-y would stop a zoomed page panning sideways.
    it("gives sideways drags back to the browser while zoomed in", () => {
      const viewport = Object.assign(new EventTarget(), { scale: 1 });
      vi.stubGlobal("visualViewport", viewport);
      try {
        const main = openOnPage(3);
        expect(main).toHaveClass("touch-pan-y");
        act(() => {
          viewport.scale = 2;
          viewport.dispatchEvent(new Event("resize"));
        });
        expect(main).not.toHaveClass("touch-pan-y");
        act(() => {
          viewport.scale = 1;
          viewport.dispatchEvent(new Event("resize"));
        });
        expect(main).toHaveClass("touch-pan-y");
      } finally {
        vi.unstubAllGlobals();
      }
    });

    it("lets a key press replace a turn that's still sliding", () => {
      const main = openOnPage(3);
      drag(main, 300, 150);
      release(main, 150);
      fireEvent.keyDown(window, { key: "ArrowLeft" });
      act(() => vi.advanceTimersByTime(500));
      expect(screen.getByTestId("location")).toHaveTextContent("page=2");
      expect(track().style.transform).toBe("");
    });
  });

  // A drag on a zoomed page pans around it.
  it("ignores a swipe while the page is zoomed in", () => {
    vi.stubGlobal(
      "visualViewport",
      Object.assign(new EventTarget(), { scale: 2 }),
    );
    try {
      swipe(openOnPage(3), [300, 400], [100, 400]);
      expect(screen.getByTestId("location")).toHaveTextContent("page=3");
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("leaves swipes alone in the scrolled flow", () => {
    flow.saved = "scrolled";
    swipe(openOnPage(3), [300, 400], [100, 400]);
    expect(screen.getByTestId("location")).toHaveTextContent("page=3");
  });
});

describe("PageReader position", () => {
  const nextPage = () =>
    fireEvent.click(screen.getByRole("button", { name: "Next page" }));
  const flush = () => act(() => vi.advanceTimersByTime(0));

  it("opens at the saved page and saves nothing for opening there", () => {
    position.savedPage = 12;
    renderReader(20);

    expect(screen.getByText("Page 13 of 20")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Page 13" })).toBeInTheDocument();
    flush();
    expect(position.saves).toEqual([]);
  });

  it("opens at the start with nothing saved", () => {
    renderReader(20);

    expect(screen.getByText("Page 1 of 20")).toBeInTheDocument();
    expect(position.saves).toEqual([]);
  });

  it("keeps to the file when the saved page is past its end", () => {
    position.savedPage = 50;
    renderReader(20);

    expect(screen.getByText("Page 20 of 20")).toBeInTheDocument();
  });

  it("shows no page until the saved position has loaded", () => {
    position.savedPage = 12;
    position.loading = true;
    renderReader(20);

    expect(screen.queryByRole("img", { name: /^Page / })).toBeNull();
  });

  it("saves each page turned to, 0-indexed, as far through as it is", () => {
    renderReader(11);

    nextPage();

    expect(position.saves).toHaveLength(1);
    expect(position.saves[0]).toMatchObject({ page: 1, percentage: 0.1 });
    expect(position.saves[0].at_end).toBeUndefined();

    act(() => vi.advanceTimersByTime(1000));
    nextPage();
    expect(position.saves).toHaveLength(2);
    expect(position.saves[1]).toMatchObject({ page: 2, percentage: 0.2 });
  });

  it("lets a page in the URL win, and saves it as opened, not read", () => {
    position.savedPage = 12;
    renderReader(20, undefined, "/?page=3");

    expect(screen.getByText("Page 4 of 20")).toBeInTheDocument();
    expect(position.saves).toHaveLength(1);
    expect(position.saves[0]).toMatchObject({ page: 3, opened: true });
  });

  it("does not report the end for a link to the last page", () => {
    renderReader(20, undefined, "/?page=19");

    expect(position.saves).toHaveLength(1);
    expect(position.saves[0]).toMatchObject({ page: 19, opened: true });
    expect(position.saves[0].at_end).toBeUndefined();
  });

  it("reports a turn after opening from a link as reading", () => {
    renderReader(20, undefined, "/?page=3");
    act(() => vi.advanceTimersByTime(1000));

    nextPage();

    expect(position.saves).toHaveLength(2);
    expect(position.saves[1]).toMatchObject({ page: 4 });
    expect(position.saves[1].opened).toBeUndefined();
  });

  // Back to it later, or a reload, is no new link: reading on elsewhere since
  // must not be replaced by the page the link named.
  it("makes a link's page its own once it has opened there", () => {
    position.savedPage = 12;
    renderReader(20, undefined, "/?page=3");

    expect(position.saves).toHaveLength(1);
    const location = screen.getByTestId("location");
    expect(location).toHaveTextContent("?page=3");
    expect(location.dataset.state).toContain("readerPage");
  });

  it("marks the page it writes to the URL as its own", () => {
    renderReader(20);

    nextPage();

    const location = screen.getByTestId("location");
    expect(location).toHaveTextContent("?page=1");
    expect(location.dataset.state).toContain("readerPage");
  });

  // Back, or a reload of a tab left open while reading elsewhere, lands on a
  // URL the reader wrote itself, which is no link to that page.
  it("prefers the saved page over a page the reader wrote to the URL itself", () => {
    position.savedPage = 12;
    renderReader(20, undefined, {
      search: "?page=3",
      state: { readerPage: true },
    });

    expect(screen.getByText("Page 13 of 20")).toBeInTheDocument();
    expect(position.saves).toEqual([]);
  });

  it("saves nothing when the saved position fails to load", () => {
    position.loadFails = true;
    renderReader(20);

    nextPage();
    act(() => vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS));

    expect(screen.getByText("Page 2 of 20")).toBeInTheDocument();
    expect(position.saves).toEqual([]);
  });

  it("sends the first of quick page turns at once and the last soon after", () => {
    renderReader(20);

    nextPage();
    nextPage();
    nextPage();
    expect(position.saves).toHaveLength(1);
    expect(position.saves[0]).toMatchObject({ page: 1 });

    act(() => vi.advanceTimersByTime(1000));
    expect(position.saves).toHaveLength(2);
    expect(position.saves[1]).toMatchObject({ page: 3 });
  });

  it("saves nothing when the URL names the saved page", () => {
    position.savedPage = 3;
    renderReader(20, undefined, "/?page=3");

    expect(screen.getByText("Page 4 of 20")).toBeInTheDocument();
    expect(position.saves).toEqual([]);
  });

  it("saves the last page as the end", () => {
    position.savedPage = 18;
    renderReader(20);

    nextPage();

    expect(position.saves[0]).toMatchObject({
      page: 19,
      percentage: 1,
      at_end: true,
    });
  });

  it("does not report the end for opening on the last page", () => {
    position.savedPage = 19;
    renderReader(20);

    flush();
    expect(position.saves).toEqual([]);
  });

  describe("in the scrolled flow", () => {
    const scrollTo = (page: number, { atBottom = false, pages = 20 } = {}) => {
      const main = screen.getByRole("main");
      const images = screen.getAllByRole("img", { name: /^Page / });
      expect(images).toHaveLength(pages);
      images.forEach((img, index) => {
        img.parentElement!.getBoundingClientRect = () =>
          ({ top: (index - page) * 1000 }) as DOMRect;
      });
      main.getBoundingClientRect = () => ({ top: 0 }) as DOMRect;
      Object.defineProperties(main, {
        scrollTop: { configurable: true, value: 1 },
        clientHeight: { configurable: true, value: 500 },
        scrollHeight: { configurable: true, value: atBottom ? 501 : 99999 },
      });
      fireEvent.scroll(main);
      act(() => vi.advanceTimersToNextFrame());
    };

    // Pages of a given height, scrolled to scrollTop with a wheel first, as a
    // reader's input comes before the scroll it makes.
    const scrollPages = (height: number) => {
      const main = screen.getByRole("main");
      const images = screen.getAllByRole("img", { name: /^Page / });
      main.getBoundingClientRect = () => ({ top: 0 }) as DOMRect;
      return (scrollTop: number) => {
        images.forEach((img, index) => {
          img.parentElement!.getBoundingClientRect = () =>
            ({ top: index * height - scrollTop }) as DOMRect;
        });
        Object.defineProperties(main, {
          scrollTop: { configurable: true, value: scrollTop },
          clientHeight: { configurable: true, value: 1000 },
          scrollHeight: { configurable: true, value: 20 * height },
        });
        fireEvent.wheel(window);
        fireEvent.scroll(main);
        act(() => vi.advanceTimersToNextFrame());
      };
    };
    const sessionDurations = () =>
      position.saves.flatMap((save) => {
        const session = (save as { session?: { duration: number } }).session;
        return session ? [session.duration] : [];
      });
    const close = () =>
      act(() => {
        window.dispatchEvent(new Event("pagehide"));
      });

    // Short webtoon strips pass in a few seconds each.
    it("counts reading time per screen of short pages", () => {
      flow.saved = "scrolled";
      renderReader(20);
      const scrollTo = scrollPages(200);
      for (let page = 1; page <= 10; page++) {
        act(() => vi.advanceTimersByTime(3_000));
        scrollTo(page * 200);
      }
      act(() => vi.advanceTimersByTime(3_000));
      close();

      expect(sessionDurations()).toEqual([30]);
    });

    it("counts reading time per screen of a page taller than one", () => {
      flow.saved = "scrolled";
      renderReader(20);
      const scrollTo = scrollPages(3000);
      act(() => vi.advanceTimersByTime(60_000));
      scrollTo(1000);
      act(() => vi.advanceTimersByTime(60_000));
      scrollTo(2000);
      act(() => vi.advanceTimersByTime(60_000));
      close();

      expect(sessionDurations()).toEqual([180]);
    });

    it("scrolls to the saved page once the flow is known", () => {
      flow.saved = "scrolled";
      position.savedPage = 12;
      renderReader(20);

      expect(scrollIntoView.mock.contexts[0]).toBe(
        screen.getByRole("img", { name: "Page 13" }).parentElement,
      );
    });

    it("saves a page settled on by scrolling, not every page passed", () => {
      flow.saved = "scrolled";
      renderReader(20);

      fireEvent.wheel(window);
      scrollTo(3);
      scrollTo(4);
      scrollTo(5);
      expect(position.saves).toEqual([]);

      act(() => vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS));

      expect(position.saves).toHaveLength(1);
      expect(position.saves[0]).toMatchObject({ page: 5 });
    });

    // Pages near a short last page settling as they load can leave the view
    // at the bottom, which reads as the last page.
    it("saves nothing for a scroll nobody made", () => {
      flow.saved = "scrolled";
      position.savedPage = 18;
      renderReader(20);

      scrollTo(19, { atBottom: true });
      act(() => vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS));

      expect(position.saves).toEqual([]);
    });

    it("finishes at the bottom once the reader scrolls there", () => {
      flow.saved = "scrolled";
      position.savedPage = 18;
      renderReader(20);

      fireEvent.wheel(window);
      scrollTo(19, { atBottom: true });

      expect(position.saves[0]).toMatchObject({ page: 19, at_end: true });
    });
  });
});
