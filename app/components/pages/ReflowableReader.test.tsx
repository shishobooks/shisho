import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IDBFactory } from "fake-indexeddb";
import { MemoryRouter } from "react-router-dom";
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

import { useReflowableBlob } from "@/hooks/queries/reflowable";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { READER_SETTINGS_KEY } from "@/hooks/useReaderSettings";
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
// The page progression direction the mocked foliate view reports.
let mockDir: string | undefined;
// Where each section starts along the book, as foliate reports it, and the
// section each table-of-contents link goes to.
let mockFractions: number[] = [];
let mockSectionOf: Record<string, number> = {};
// Whether the mocked renderer is at the end of the book.
let mockAtEnd = false;
// The screen the mocked renderer shows when the book opens.
let mockPage: number | undefined;
// The saved position GET /reading/files/:id/position returns.
let savedCfi: string | null = null;
// A saved position with no locator, only a percentage, as a copy a user
// linked stores (ADR 0017). It replaces savedCfi when set.
let savedPercentageOnly: number | null = null;
const SAVED_AT = "2026-10-01T09:00:00.000Z";
// What POST /reading/files/:id/position reports it did.
let saveResult: object = {};
let positionLoadFails = false;
// What the mocked foliate search finds, one entry per section with matches.
let mockSearchSections: Array<{
  label: string;
  subitems: Array<{
    cfi: string;
    excerpt: { pre: string; match: string; post: string };
  }>;
}> = [];

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
          page: mockPage,
          setStyles: vi.fn(),
          setAttribute: vi.fn(),
        };
        lastLocation = { cfi: "epubcfi(/6/2!/4/2)", fraction: 0 };
        getSectionFractions = () => mockFractions;
        search = vi.fn(async function* () {
          for (const section of mockSearchSections) yield section;
          yield "done";
        });
        clearSearch = vi.fn();
        addAnnotation = vi.fn().mockResolvedValue(undefined);
        deleteAnnotation = vi.fn().mockResolvedValue(undefined);
        get book() {
          return {
            toc: mockToc,
            dir: mockDir,
            resolveHref: (href: string) =>
              href in mockSectionOf
                ? { index: mockSectionOf[href], anchor: () => 0 }
                : null,
          };
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
  // Display settings and imported fonts live in the browser; every test
  // starts with none.
  localStorage.clear();
  vi.stubGlobal("indexedDB", new IDBFactory());
  setAuth({ permissions: ["books:read"] });
  mockAtEnd = false;
  mockPage = undefined;
  mockDir = undefined;
  mockFractions = [];
  mockSectionOf = {};
  savedCfi = null;
  savedPercentageOnly = null;
  saveResult = {};
  positionLoadFails = false;
  mockSearchSections = [];
  vi.spyOn(API, "request").mockImplementation(((
    method: string,
    endpoint: string,
  ) => {
    if (method === "GET" && endpoint === "/reading/files/7/position") {
      if (positionLoadFails) return Promise.reject(new Error("offline"));
      if (savedPercentageOnly !== null) {
        return Promise.resolve({
          position: {
            file_id: 7,
            percentage: savedPercentageOnly,
            occurred_at: SAVED_AT,
          },
        });
      }
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
      const menu = await screen.findByRole("combobox", {
        name: "Jump to chapter",
      });
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
      expect(menu).toHaveTextContent(/^Part Two$/);
    } finally {
      mockToc = [];
    }
  });

  it("marks where each chapter starts along the progress bar", async () => {
    mockToc = [
      { label: "Cover", href: "cover.xhtml" },
      { label: "Part One", href: "" },
      { label: "Chapter 1", href: "ch1.xhtml" },
      { label: "Chapter 2", href: "ch2.xhtml" },
    ];
    mockSectionOf = { "cover.xhtml": 0, "ch1.xhtml": 1, "ch2.xhtml": 2 };
    mockFractions = [0, 0.05, 0.5, 1];
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    try {
      renderReader();
      const progress = screen.getByRole("slider", { name: "Reading progress" });
      // The cover starts the book, at the bar's own start; the part heading
      // goes nowhere.
      await vi.waitFor(() =>
        expect(
          within(progress)
            .getAllByTestId("chapter-mark")
            .map((m) => m.style.left),
        ).toEqual(["5%", "50%"]),
      );
    } finally {
      mockToc = [];
    }
  });

  it("goes to the chapter's exact start on a click on its mark", async () => {
    mockToc = [
      { label: "Chapter 1", href: "ch1.xhtml" },
      { label: "Chapter 2", href: "ch2.xhtml" },
    ];
    mockSectionOf = { "ch1.xhtml": 0, "ch2.xhtml": 1 };
    mockFractions = [0, 0.4, 1];
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    try {
      const { container } = renderReader();
      const view = container.querySelector("foliate-view") as unknown as {
        goTo: ReturnType<typeof vi.fn>;
        goToFraction: ReturnType<typeof vi.fn>;
      };
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const mark = await screen.findByTestId("chapter-mark");
      await user.click(mark);

      // The chapter's link, not the fraction under the pointer.
      expect(view.goTo).toHaveBeenCalledWith("ch2.xhtml");
      expect(view.goToFraction).not.toHaveBeenCalled();
    } finally {
      mockToc = [];
    }
  });

  it("shows progress to a tenth of a percent", async () => {
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);
    const { container } = renderReader();
    const progress = screen.getByRole("slider", { name: "Reading progress" });
    await vi.waitFor(() => expect(progress).toBeInTheDocument());

    act(() => {
      container
        .querySelector("foliate-view")!
        .dispatchEvent(
          new CustomEvent("relocate", { detail: { fraction: 0.2345 } }),
        );
    });

    expect(progress).toHaveAttribute("aria-valuenow", "23.5");
    expect(progress).toHaveAttribute("aria-valuetext", "23.5%");
    expect(screen.getByText("23.5%")).toBeInTheDocument();
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
      const { container } = renderReader();
      const menu = await screen.findByRole("combobox", {
        name: "Jump to chapter",
      });
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      await user.click(menu);
      const options = await screen.findAllByRole("option");
      expect(options.map((o) => o.textContent)).toEqual([
        "Part Two",
        "Chapter 3",
        "Afterword",
      ]);
      expect(options[0]).toHaveAttribute("aria-disabled", "true");
      expect(options[1]).not.toHaveAttribute("aria-disabled", "true");
      // Nested chapters are indented under their heading.
      expect(options[1].style.paddingLeft).toBe("1.5rem");
      expect(options[2].style.paddingLeft).toBe("0.5rem");

      await user.click(options[2]);
      const view = container.querySelector("foliate-view") as unknown as {
        goTo: ReturnType<typeof vi.fn>;
      };
      expect(view.goTo).toHaveBeenCalledWith("after.xhtml");
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

  describe("page buttons once the book opens", () => {
    const openBook = async () => {
      vi.mocked(useReflowableBlob).mockReturnValue({
        data: new Blob(["x"], { type: "application/epub+zip" }),
        isLoading: false,
        isError: false,
        error: null,
        refetch: vi.fn(),
      } as never);
      const { container } = renderReader();
      const view = container.querySelector(
        "foliate-view",
      ) as unknown as HTMLElement & {
        goLeft: ReturnType<typeof vi.fn>;
        goRight: ReturnType<typeof vi.fn>;
      };
      await vi.waitFor(() =>
        expect(screen.getByRole("button", { name: "Next page" })).toBeEnabled(),
      );
      // In document order, which is left to right in the footer.
      const [left, right] = screen.getAllByRole("button", {
        name: /^(Previous|Next) page$/,
      });
      return { view, left, right };
    };

    it("label the left button Previous and the right one Next in a left-to-right book", async () => {
      const { view, left, right } = await openBook();
      expect(left).toHaveAccessibleName("Previous page");
      expect(right).toHaveAccessibleName("Next page");

      await userEvent.click(left);
      expect(view.goLeft).toHaveBeenCalledTimes(1);
      await userEvent.click(right);
      expect(view.goRight).toHaveBeenCalledTimes(1);
    });

    // foliate turns goLeft() into next() for a right-to-left book.
    it("label the left button Next and the right one Previous in a right-to-left book", async () => {
      mockDir = "rtl";
      const { view, left, right } = await openBook();
      expect(left).toHaveAccessibleName("Next page");
      expect(right).toHaveAccessibleName("Previous page");

      await userEvent.click(left);
      expect(view.goLeft).toHaveBeenCalledTimes(1);
      expect(view.goRight).not.toHaveBeenCalled();
    });
  });

  it("shows the extended-wait hint after 10 seconds of loading", () => {
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
    });

    it("leave the controls alone when following a link", () => {
      const { container } = renderWithAutoHide("scrolled");
      const doc = loadSection(container);

      act(() => doc.querySelector("a")!.click());
      expect(header()).not.toHaveClass("-translate-y-full");
    });
  });

  describe("taps on a paginated book", () => {
    const VIEW = { left: 0, top: 0, width: 300, height: 800 };
    const HEADER_HEIGHT = 50;
    const FOOTER_HEIGHT = 90;
    // A paginated section is one wide frame scrolled sideways, so a tap's
    // clientX inside it is far from where the tap is on screen.
    const FRAME_LEFT = -600;

    const rect = (left: number, top: number, width: number, height: number) =>
      ({
        left,
        top,
        width,
        height,
        right: left + width,
        bottom: top + height,
      }) as DOMRect;

    const setup = (hideChrome: boolean) => {
      vi.mocked(useUserSettings).mockReturnValue({
        data: {
          viewer_reflowable_font_size: 100,
          viewer_reflowable_theme: "light",
          viewer_reflowable_flow: "paginated",
          viewer_hide_chrome: hideChrome,
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
      const { container } = renderReader();
      const view = container.querySelector(
        "foliate-view",
      ) as unknown as HTMLElement & {
        goLeft: ReturnType<typeof vi.fn>;
        goRight: ReturnType<typeof vi.fn>;
      };
      view.getBoundingClientRect = () =>
        rect(VIEW.left, VIEW.top, VIEW.width, VIEW.height);
      const header = screen.getByRole("banner");
      const footer = screen.getByRole("contentinfo");
      Object.defineProperty(header, "offsetHeight", { value: HEADER_HEIGHT });
      Object.defineProperty(footer, "offsetHeight", { value: FOOTER_HEIGHT });

      const frame = document.createElement("iframe");
      container.append(frame);
      frame.getBoundingClientRect = () =>
        rect(FRAME_LEFT, VIEW.top, 900, VIEW.height);
      Object.defineProperty(frame, "offsetWidth", { value: 900 });
      const doc = frame.contentDocument!;
      doc.body.innerHTML = '<p>Text <a href="#n">note</a></p>';
      act(() => {
        view.dispatchEvent(new CustomEvent("load", { detail: { doc } }));
      });

      // Taps at a position on screen, inside the book's frame.
      const tapBook = (
        x: number,
        y = 400,
        target: Element = doc.querySelector("p")!,
      ) => {
        const init = {
          bubbles: true,
          clientX: x - FRAME_LEFT,
          clientY: y,
        };
        act(() => {
          target.dispatchEvent(new Event("pointerdown", { bubbles: true }));
          target.dispatchEvent(new MouseEvent("click", init));
        });
      };
      return { view, header, doc, tapBook };
    };

    // A press in the book's frame never reaches the page, where the popover
    // listens for presses outside it.
    it("close the settings instead of turning the page", async () => {
      const { view, tapBook } = setup(false);
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      await user.click(screen.getByRole("button", { name: "Settings" }));
      expect(screen.getByRole("dialog")).toBeInTheDocument();

      tapBook(295);

      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(view.goRight).not.toHaveBeenCalled();

      // The next tap turns the page as usual.
      tapBook(295);
      expect(view.goRight).toHaveBeenCalledTimes(1);
    });

    it("close the settings on the margins instead of turning the page", async () => {
      const { view } = setup(false);
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      await user.click(screen.getByRole("button", { name: "Settings" }));

      fireEvent.pointerDown(view);
      fireEvent.click(view, { clientX: 295, clientY: 400 });

      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(view.goRight).not.toHaveBeenCalled();
    });

    it("close the settings on a link instead of following it", async () => {
      const { view, doc } = setup(false);
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      await user.click(screen.getByRole("button", { name: "Settings" }));

      const link = doc.querySelector("a")!;
      act(() => {
        link.dispatchEvent(new Event("pointerdown", { bubbles: true }));
      });
      // foliate reports the link it's about to follow; a canceled report
      // stays put.
      let follow = true;
      act(() => {
        follow = view.dispatchEvent(
          new CustomEvent("link", {
            detail: { a: link, href: "#n" },
            cancelable: true,
          }),
        );
      });

      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(follow).toBe(false);
    });

    it("close the settings on a link out of the book instead of opening it", async () => {
      const { view, doc } = setup(false);
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      await user.click(screen.getByRole("button", { name: "Settings" }));

      const link = doc.querySelector("a")!;
      act(() => {
        link.dispatchEvent(new Event("pointerdown", { bubbles: true }));
      });
      let open = true;
      act(() => {
        open = view.dispatchEvent(
          new CustomEvent("external-link", {
            detail: { a: link, href: "https://example.com" },
            cancelable: true,
          }),
        );
      });

      expect(open).toBe(false);
    });

    it("follow a link from the keyboard after a tap closed the settings", async () => {
      const { view, doc, tapBook } = setup(false);
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      await user.click(screen.getByRole("button", { name: "Settings" }));
      tapBook(150);
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

      // Enter on a focused link: a click with no press before it. The
      // reader follows links itself, for its history of jumps.
      act(() => {
        view.dispatchEvent(
          new CustomEvent("link", {
            detail: { a: doc.querySelector("a")!, href: "#n" },
            cancelable: true,
          }),
        );
      });
      await act(async () => {});

      expect(
        (view as unknown as { goTo: ReturnType<typeof vi.fn> }).goTo,
      ).toHaveBeenCalledWith("#n");
    });

    it("replace the overlay buttons", () => {
      setup(true);
      expect(
        screen
          .queryAllByRole("button", { hidden: true })
          .filter((button) => button.getAttribute("aria-hidden") === "true"),
      ).toHaveLength(0);
    });

    // Tapping the page is no use from a keyboard. The button stays out of the
    // way of swipes until it has focus.
    it("leave a keyboard way to toggle the controls", () => {
      const { header } = setup(true);
      const toggle = screen.getByRole("button", { name: "Toggle controls" });
      expect(toggle).toHaveClass("sr-only");
      fireEvent.click(toggle);
      expect(header).toHaveClass("-translate-y-full");
    });

    it("leave controls in the book alone", () => {
      const { view, doc, tapBook } = setup(false);
      const button = doc.createElement("button");
      doc.body.append(button);
      tapBook(250, 400, button);
      expect(view.goRight).not.toHaveBeenCalled();
    });

    it("clear a selection on the margins instead of turning the page", () => {
      const { view, doc } = setup(false);
      (
        view as unknown as { renderer: { getContents: () => unknown[] } }
      ).renderer.getContents = () => [{ doc }];
      doc.getSelection()!.selectAllChildren(doc.querySelector("p")!);

      fireEvent.click(view, { clientX: 295, clientY: 400 });
      expect(view.goRight).not.toHaveBeenCalled();
      expect(doc.getSelection()!.isCollapsed).toBe(true);

      fireEvent.click(view, { clientX: 295, clientY: 400 });
      expect(view.goRight).toHaveBeenCalledTimes(1);
    });

    it("go back in the left third", () => {
      const { view, tapBook } = setup(false);
      tapBook(50);
      expect(view.goLeft).toHaveBeenCalledTimes(1);
      expect(view.goRight).not.toHaveBeenCalled();
    });

    it("go forward in the right third", () => {
      const { view, tapBook } = setup(false);
      tapBook(250);
      expect(view.goRight).toHaveBeenCalledTimes(1);
      expect(view.goLeft).not.toHaveBeenCalled();
    });

    it("do nothing in the center third with auto-hide off", () => {
      const { view, header, tapBook } = setup(false);
      tapBook(150);
      expect(view.goLeft).not.toHaveBeenCalled();
      expect(view.goRight).not.toHaveBeenCalled();
      expect(header).not.toHaveClass("-translate-y-full");
    });

    it("toggle the controls in the center third with auto-hide on", () => {
      const { view, header, tapBook } = setup(true);
      tapBook(150);
      expect(header).toHaveClass("-translate-y-full");
      tapBook(150);
      expect(header).not.toHaveClass("-translate-y-full");
      expect(view.goLeft).not.toHaveBeenCalled();
      expect(view.goRight).not.toHaveBeenCalled();
    });

    it("work on foliate's margins, outside the frame", () => {
      const { view } = setup(false);
      fireEvent.click(view, { clientX: 5, clientY: 400 });
      expect(view.goLeft).toHaveBeenCalledTimes(1);
      fireEvent.click(view, { clientX: 295, clientY: 400 });
      expect(view.goRight).toHaveBeenCalledTimes(1);
    });

    it("follow a link instead of turning the page", () => {
      const { view, doc, tapBook } = setup(false);
      tapBook(250, 400, doc.querySelector("a")!);
      expect(view.goRight).not.toHaveBeenCalled();
    });

    // foliate scrolls to whatever takes focus on the next frame. A tap's press
    // and click arrive together, so that would undo the link's jump.
    it("keep focus off a link, so foliate doesn't scroll back to it", () => {
      const { doc } = setup(false);
      expect(fireEvent.mouseDown(doc.querySelector("a")!)).toBe(false);
      expect(fireEvent.mouseDown(doc.querySelector("p")!)).toBe(true);
    });

    it("don't turn the page when they end a selection", () => {
      const { view, doc, tapBook } = setup(false);
      doc.getSelection()!.selectAllChildren(doc.querySelector("p")!);
      tapBook(250);
      expect(view.goRight).not.toHaveBeenCalled();
    });

    it("don't turn the page when they clear a selection", () => {
      const { view, doc } = setup(false);
      const p = doc.querySelector("p")!;
      doc.getSelection()!.selectAllChildren(p);
      // The browser collapses the selection between the press and the click.
      act(() => {
        p.dispatchEvent(new Event("pointerdown", { bubbles: true }));
        doc.getSelection()!.removeAllRanges();
        p.dispatchEvent(
          new MouseEvent("click", {
            bubbles: true,
            clientX: 250 - FRAME_LEFT,
            clientY: 400,
          }),
        );
      });
      expect(view.goRight).not.toHaveBeenCalled();

      // The next tap has nothing left to clear.
      act(() => {
        p.dispatchEvent(new Event("pointerdown", { bubbles: true }));
        p.dispatchEvent(
          new MouseEvent("click", {
            bubbles: true,
            clientX: 250 - FRAME_LEFT,
            clientY: 400,
          }),
        );
      });
      expect(view.goRight).toHaveBeenCalledTimes(1);
    });

    describe("while the controls are hidden", () => {
      const setupHidden = () => {
        const result = setup(true);
        act(() => vi.advanceTimersByTime(2000));
        expect(result.header).toHaveClass("-translate-y-full");
        return result;
      };

      it("bring them back along the top edge, where the gear was", () => {
        const { view, header, tapBook } = setupHidden();
        tapBook(280, 20);
        expect(header).not.toHaveClass("-translate-y-full");
        expect(view.goRight).not.toHaveBeenCalled();
      });

      it("bring them back along the bottom edge", () => {
        const { view, header, tapBook } = setupHidden();
        tapBook(20, 780);
        expect(header).not.toHaveClass("-translate-y-full");
        expect(view.goLeft).not.toHaveBeenCalled();
      });

      it("still turn the page below the top edge and above the bottom edge", () => {
        const { view, header, tapBook } = setupHidden();
        tapBook(280, HEADER_HEIGHT + 1);
        tapBook(20, VIEW.height - FOOTER_HEIGHT - 1);
        expect(view.goRight).toHaveBeenCalledTimes(1);
        expect(view.goLeft).toHaveBeenCalledTimes(1);
        expect(header).toHaveClass("-translate-y-full");
      });
    });

    it("turn the page along the edges while the controls are showing", () => {
      const { view, tapBook } = setup(true);
      tapBook(280, 20);
      expect(view.goRight).toHaveBeenCalledTimes(1);
    });

    // A click moves focus into the frame, and key events there never reach
    // the window.
    it("leave the arrow keys working", () => {
      const { view, doc } = setup(false);
      doc.dispatchEvent(
        new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }),
      );
      expect(view.goRight).toHaveBeenCalledTimes(1);
      doc.dispatchEvent(
        new KeyboardEvent("keydown", { key: "ArrowLeft", bubbles: true }),
      );
      expect(view.goLeft).toHaveBeenCalledTimes(1);
    });

    it("leave browser shortcuts alone", () => {
      const { view, doc } = setup(false);
      doc.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "a",
          metaKey: true,
          bubbles: true,
        }),
      );
      expect(view.goLeft).not.toHaveBeenCalled();
    });
  });

  // A failed load unmounts foliate, and Retry mounts a new element.
  it("handles taps on the book after a retry", () => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: {
        viewer_reflowable_font_size: 100,
        viewer_reflowable_theme: "light",
        viewer_reflowable_flow: "paginated",
      },
      isLoading: false,
    } as never);
    vi.mocked(useReflowableBlob).mockReturnValue({
      isLoading: false,
      error: new Error("boom"),
      isEnabled: false,
      refetch: vi.fn(),
    } as never);
    const client = new QueryClient();
    const tree = () => (
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <ReflowableReader
            bookTitle="Test Book"
            file={{ id: 7, book_id: 3, file_type: "epub" } as never}
          />
        </MemoryRouter>
      </QueryClientProvider>
    );
    const { container, rerender } = render(tree());
    expect(container.querySelector("foliate-view")).toBeNull();
    vi.mocked(useReflowableBlob).mockReturnValue({
      isLoading: true,
      error: null,
      refetch: vi.fn(),
    } as never);
    rerender(tree());

    const view = container.querySelector(
      "foliate-view",
    ) as unknown as HTMLElement & {
      goRight: ReturnType<typeof vi.fn>;
    };
    view.getBoundingClientRect = () =>
      ({ left: 0, top: 0, width: 300, right: 300, bottom: 800 }) as DOMRect;
    fireEvent.click(view, { clientX: 295, clientY: 400 });
    expect(view.goRight).toHaveBeenCalledTimes(1);
  });

  describe("a fixed-layout book", () => {
    const touchEvent = (type: string, screenX: number, screenY = 400) =>
      Object.assign(new Event(type, { bubbles: true }), {
        touches: type === "touchend" ? [] : [{ screenX, screenY }],
        changedTouches: [{ screenX, screenY }],
      });

    const setup = (flow: "paginated" | "scrolled", isFixedLayout = true) => {
      vi.mocked(useUserSettings).mockReturnValue({
        data: {
          viewer_reflowable_font_size: 100,
          viewer_reflowable_theme: "light",
          viewer_reflowable_flow: flow,
        },
        isLoading: false,
      } as never);
      vi.mocked(useReflowableBlob).mockReturnValue({
        data: undefined,
        isLoading: true,
        error: null,
        refetch: vi.fn(),
      } as never);
      const { container } = renderReader();
      const view = container.querySelector(
        "foliate-view",
      ) as unknown as HTMLElement & {
        goLeft: ReturnType<typeof vi.fn>;
        goRight: ReturnType<typeof vi.fn>;
        isFixedLayout: boolean;
      };
      view.isFixedLayout = isFixedLayout;
      view.getBoundingClientRect = () =>
        ({ left: 0, top: 0, width: 300, right: 300, bottom: 800 }) as DOMRect;
      const frame = document.createElement("iframe");
      container.append(frame);
      const doc = frame.contentDocument!;
      act(() => {
        view.dispatchEvent(new CustomEvent("load", { detail: { doc } }));
      });
      return { view, doc };
    };
    const swipe = (target: EventTarget, from: number, to: number) => {
      target.dispatchEvent(touchEvent("touchstart", from));
      target.dispatchEvent(touchEvent("touchend", to));
    };

    it("turns the page on a swipe inside the page's frame", () => {
      const { view, doc } = setup("paginated");
      swipe(doc, 300, 100);
      expect(view.goRight).toHaveBeenCalledTimes(1);
      swipe(doc, 100, 300);
      expect(view.goLeft).toHaveBeenCalledTimes(1);
    });

    it("turns the page on a swipe on the space around the page", () => {
      const { view } = setup("paginated");
      swipe(view, 300, 100);
      expect(view.goRight).toHaveBeenCalledTimes(1);
    });

    // It can't scroll, so the Scrolled setting changes nothing for it.
    it("turns the page by swipe and tap under the Scrolled setting too", () => {
      const { view, doc } = setup("scrolled");
      swipe(doc, 300, 100);
      expect(view.goRight).toHaveBeenCalledTimes(1);
      fireEvent.click(view, { clientX: 295, clientY: 400 });
      expect(view.goRight).toHaveBeenCalledTimes(2);
    });

    it("gives sideways drags back to the browser while zoomed in", () => {
      const viewport = Object.assign(new EventTarget(), { scale: 1 });
      vi.stubGlobal("visualViewport", viewport);
      try {
        const { view, doc } = setup("paginated");
        (
          view as unknown as { renderer: { getContents: () => unknown[] } }
        ).renderer.getContents = () => [{ doc }];
        act(() => {
          view.dispatchEvent(new CustomEvent("load", { detail: { doc } }));
        });
        expect(doc.documentElement.style.touchAction).toBe("pan-y pinch-zoom");

        act(() => {
          viewport.scale = 2;
          viewport.dispatchEvent(new Event("resize"));
        });
        expect(doc.documentElement.style.touchAction).toBe("");
        expect(view.style.touchAction).toBe("");
      } finally {
        vi.unstubAllGlobals();
      }
    });

    it("leaves a reflowable book's swipes to foliate", () => {
      const { view, doc } = setup("paginated", false);
      swipe(doc, 300, 100);
      swipe(view, 300, 100);
      expect(view.goRight).not.toHaveBeenCalled();
    });
  });

  it("leaves a margin click alone in scrolled flow", () => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: {
        viewer_reflowable_font_size: 100,
        viewer_reflowable_theme: "light",
        viewer_reflowable_flow: "scrolled",
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
    const { container } = renderReader();
    const view = container.querySelector(
      "foliate-view",
    ) as unknown as HTMLElement & {
      goRight: ReturnType<typeof vi.fn>;
    };
    view.getBoundingClientRect = () =>
      ({
        left: 0,
        top: 0,
        width: 300,
        height: 800,
        right: 300,
        bottom: 800,
      }) as DOMRect;

    fireEvent.click(view, { clientX: 295, clientY: 400 });

    expect(view.goRight).not.toHaveBeenCalled();
    expect(screen.getByRole("banner")).not.toHaveClass("-translate-y-full");
  });

  it("toasts when saving auto-hide fails", async () => {
    const error = vi.spyOn(toast, "error");
    vi.mocked(useUserSettings).mockReturnValue({
      data: { viewer_hide_chrome: false },
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
    await user.click(
      screen.getByRole("switch", { name: "Auto-hide controls" }),
    );

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

  it("says the font size with its unit", async () => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: { viewer_hide_chrome: false },
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
    ).toHaveAttribute("aria-valuetext", "16 px");
    expect(screen.getByRole("slider", { name: "Margins" })).toHaveAttribute(
      "aria-valuetext",
      "7%",
    );
  });

  it("keeps focus in the settings when a step reaches the end of its range", async () => {
    localStorage.setItem(READER_SETTINGS_KEY, JSON.stringify({ margins: 14 }));
    vi.mocked(useUserSettings).mockReturnValue({
      data: { viewer_hide_chrome: false },
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
    const increase = await screen.findByRole("button", {
      name: "Increase margins",
    });

    act(() => increase.focus());
    await user.keyboard("{Enter}");

    // The button it pressed is now disabled, so focus moves to the other.
    expect(increase).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Decrease margins" }),
    ).toHaveFocus();
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
    ).toHaveAttribute("aria-valuenow", "16");
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

  it("opens a saved place with no location at its percentage", async () => {
    savedPercentageOnly = 0.4;
    const view = await openBook();
    expect(view.init).toHaveBeenCalledWith({
      lastLocation: { fraction: 0.4 },
    });

    // Opening there isn't reading, so closing saves no place.
    act(() => {
      vi.advanceTimersByTime(60_000);
      window.dispatchEvent(new Event("pagehide"));
    });
    for (const save of positionSaves()) {
      expect(save.cfi).toBeUndefined();
      expect(save.percentage).toBeUndefined();
    }
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

  it.each([
    ["a saved place", "epubcfi(/6/14!/4/2/10)"],
    ["no saved place", null],
  ])(
    "sends only the session when closed without moving, from %s",
    async (_, cfi) => {
      savedCfi = cfi;
      // foliate reports the first page above 0%, and a layout on another
      // screen size starts somewhere other than the saved location, so
      // where it opened must never read as reading.
      const view = await openBook();
      (view as unknown as { lastLocation: object }).lastLocation = {
        cfi: "epubcfi(/6/4!/4/2)",
        fraction: 0.125,
      };
      act(() => {
        vi.advanceTimersByTime(60_000);
        window.dispatchEvent(new Event("pagehide"));
      });
      const saves = positionSaves();
      expect(saves).toEqual([
        expect.objectContaining({ session: expect.anything() }),
      ]);
      expect(saves[0].cfi).toBeUndefined();
      expect(saves[0].at_end).toBeUndefined();
      expect(saves[0].percentage).toBeUndefined();
    },
  );

  it("sends the place with the session once the reader has moved", async () => {
    const view = await openBook();
    relocate(view, {
      cfi: "epubcfi(/6/6!/4/2)",
      fraction: 0.2,
      reason: "page",
    });
    // Long enough to count as reading the page, and before the save timer.
    act(() => {
      vi.setSystemTime(Date.now() + 10_000);
      window.dispatchEvent(new Event("pagehide"));
    });
    expect(positionSaves()).toEqual([
      expect.objectContaining({
        cfi: "epubcfi(/6/6!/4/2)",
        session: expect.anything(),
      }),
    ]);
  });

  it("sends a place again after its save failed", async () => {
    const view = await openBook();
    const request = vi.mocked(API.request);
    const respond = request.getMockImplementation()!;
    request.mockImplementationOnce((method, endpoint) =>
      method === "POST"
        ? Promise.reject(new Error("offline"))
        : (respond as never as typeof API.request)(method, endpoint),
    );
    mockAtEnd = true;
    relocate(view, { cfi: "epubcfi(/6/40!/4/2)", fraction: 1, reason: "page" });
    await act(async () => {});

    act(() => {
      vi.advanceTimersByTime(60_000);
      window.dispatchEvent(new Event("pagehide"));
    });
    const saves = positionSaves();
    expect(saves).toHaveLength(2);
    expect(saves[1]).toEqual(
      expect.objectContaining({
        cfi: "epubcfi(/6/40!/4/2)",
        at_end: true,
        occurred_at: saves[0].occurred_at,
      }),
    );
  });

  it("refreshes the book's status when reading resumes it", async () => {
    const client = new QueryClient();
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const view = await openBook(client);
    saveResult = { resumed: true };
    relocate(view, {
      cfi: "epubcfi(/6/4!/4/2)",
      fraction: 0.1,
      reason: "page",
    });
    await act(async () => {
      vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS);
    });
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ["BookReading", 3],
    });
  });

  // Every touch ends in a snap, even a tap that moves nothing, and the location
  // it reports can differ from a saved one taken at another screen size.
  it("doesn't count a snap that stays on the page as reading", async () => {
    const view = await openBook();
    const renderer = (view as unknown as { renderer: { page: number } })
      .renderer;
    renderer.page = 4;
    relocate(view, {
      cfi: "epubcfi(/6/2!/4/4)",
      fraction: 0.2,
      reason: "page",
    });
    await act(async () => {
      vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS);
    });
    const saves = positionSaves().length;

    relocate(view, {
      cfi: "epubcfi(/6/2!/4/5)",
      fraction: 0.2,
      reason: "snap",
    });
    await act(async () => {
      vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS);
    });
    expect(positionSaves()).toHaveLength(saves);

    // A swipe also ends in a snap, onto a new page.
    renderer.page = 5;
    relocate(view, {
      cfi: "epubcfi(/6/2!/4/8)",
      fraction: 0.3,
      reason: "snap",
    });
    await act(async () => {
      vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS);
    });
    expect(positionSaves()).toHaveLength(saves + 1);
  });

  it("doesn't count scrolling within the first screen in scrolled flow", async () => {
    const view = await openBook();
    const renderer = (
      view as unknown as {
        renderer: { scrolled: boolean; start: number; size: number };
      }
    ).renderer;
    renderer.scrolled = true;
    renderer.size = 800;
    renderer.start = 120;
    relocate(view, {
      cfi: "epubcfi(/6/2!/4/4)",
      fraction: 0.01,
      reason: "scroll",
    });
    await act(async () => {
      vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS);
    });
    expect(positionSaves()).toEqual([]);

    renderer.start = 900;
    relocate(view, {
      cfi: "epubcfi(/6/2!/4/8)",
      fraction: 0.05,
      reason: "scroll",
    });
    await act(async () => {
      vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS);
    });
    expect(positionSaves()).toEqual([
      expect.objectContaining({ cfi: "epubcfi(/6/2!/4/8)" }),
    ]);
  });

  // A scroll moves the location every few seconds, but time counts per screen.
  it("counts scrolling within one screen as one page of reading", async () => {
    mockPage = 1;
    const view = await openBook();
    const renderer = (
      view as unknown as {
        renderer: {
          scrolled: boolean;
          start: number;
          size: number;
          page: number;
        };
      }
    ).renderer;
    renderer.scrolled = true;
    renderer.size = 800;
    renderer.start = 900;
    renderer.page = 1;
    for (let i = 1; i <= 10; i++) {
      relocate(view, {
        cfi: `epubcfi(/6/2!/4/${2 * i})`,
        fraction: 0.05,
        reason: "scroll",
      });
      act(() => vi.advanceTimersByTime(3_000));
    }
    act(() => {
      window.dispatchEvent(new Event("pagehide"));
    });

    // From opening to closing is one stay on one screen.
    const sessions = positionSaves().filter((save) => save.session);
    expect(sessions).toHaveLength(1);
    const session = sessions[0].session!;
    const span =
      (Date.parse(session.ended_at) - Date.parse(session.started_at)) / 1000;
    expect(span).toBeGreaterThanOrEqual(30);
    expect(session.duration).toBe(Math.round(span));
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

describe("ReflowableReader display settings", () => {
  type MockRenderer = {
    setStyles: ReturnType<typeof vi.fn>;
    setAttribute: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: { viewer_hide_chrome: false },
      isLoading: false,
    } as never);
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);
    // jsdom has no object URLs.
    let next = 0;
    URL.createObjectURL = vi.fn(() => `blob:http://localhost/font-${++next}`);
    URL.revokeObjectURL = vi.fn();
    // jsdom has no FontFace, which import uses when the browser has it.
    vi.stubGlobal("FontFace", undefined);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // Opens the book and returns the renderer the reader styles.
  const openBook = async () => {
    const { container, unmount } = renderReader();
    const view = container.querySelector("foliate-view") as unknown as {
      renderer: MockRenderer;
    };
    await vi.waitFor(() => expect(view.renderer.setStyles).toHaveBeenCalled());
    return { renderer: view.renderer, unmount };
  };

  const css = (renderer: MockRenderer) =>
    renderer.setStyles.mock.lastCall?.[0] as string;
  const attribute = (renderer: MockRenderer, name: string) => {
    const calls = renderer.setAttribute.mock.calls.filter(([n]) => n === name);
    return calls[calls.length - 1]?.[1];
  };

  const openSettings = async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await user.click(screen.getByRole("button", { name: "Settings" }));
    return user;
  };

  type User = ReturnType<typeof userEvent.setup>;
  const pick = (name: string) => (user: User) =>
    user.click(screen.getByRole("button", { name }));

  it.each<[string, (user: User) => Promise<void>, (r: MockRenderer) => void]>([
    [
      "font",
      async (user) => {
        await user.click(screen.getByRole("combobox", { name: "Font" }));
        await user.click(screen.getByRole("option", { name: "OpenDyslexic" }));
      },
      (r) => {
        expect(css(r)).toContain('font-family: "OpenDyslexic", sans-serif');
        expect(css(r)).toMatch(/@font-face[^}]*OpenDyslexic-Regular/);
      },
    ],
    [
      "size",
      async (user) => {
        act(() => screen.getByRole("slider", { name: "Font size" }).focus());
        await user.keyboard("{ArrowRight}");
      },
      (r) => expect(css(r)).toContain("font-size: 16.5px !important"),
    ],
    [
      "size, one step at a time",
      async (user) => {
        await pick("Increase font size")(user);
        await pick("Increase font size")(user);
        await pick("Decrease font size")(user);
      },
      (r) => expect(css(r)).toContain("font-size: 16.5px !important"),
    ],
    [
      "forced font size",
      (user) =>
        user.click(screen.getByRole("switch", { name: "Force font size" })),
      (r) => expect(css(r)).toContain("font-size: inherit !important"),
    ],
    [
      "line spacing",
      async (user) => {
        await pick("Increase line spacing")(user);
        await pick("Increase line spacing")(user);
      },
      (r) => expect(css(r)).toContain("line-height: 1.55 !important"),
    ],
    [
      "margins",
      async (user) => {
        act(() => screen.getByRole("slider", { name: "Margins" }).focus());
        await user.keyboard("{End}");
      },
      (r) => expect(attribute(r, "gap")).toBe("15%"),
    ],
    [
      "justification",
      (user) =>
        user.click(screen.getByRole("switch", { name: "Justify text" })),
      (r) => expect(css(r)).toContain("text-align: start !important"),
    ],
    [
      "theme",
      pick("Black"),
      (r) => expect(css(r)).toContain("background: #000000 !important"),
    ],
    [
      "layout",
      pick("Scrolled"),
      (r) => expect(attribute(r, "flow")).toBe("scrolled"),
    ],
    [
      "pages",
      pick("One"),
      (r) => expect(attribute(r, "max-column-count")).toBe("1"),
    ],
  ])("applies the %s and remembers it", async (_, change, check) => {
    const first = await openBook();
    const user = await openSettings();
    await change(user);
    check(first.renderer);
    first.unmount();

    const reopened = await openBook();
    check(reopened.renderer);
  });

  it("keeps a section's own centered text out of justification", async () => {
    const { container } = renderReader();
    const doc = document.implementation.createHTMLDocument("section");
    doc.body.innerHTML = '<p>Body.</p><p align="center">* * *</p>';
    act(() => {
      container
        .querySelector("foliate-view")!
        .dispatchEvent(new CustomEvent("load", { detail: { doc } }));
    });
    // The section's elements come from another document, which jest-dom's
    // matchers refuse.
    expect(doc.documentElement.hasAttribute("data-shisho-align")).toBe(true);
    const [body, centered] = doc.querySelectorAll("p");
    expect(centered.hasAttribute("data-shisho-own-align")).toBe(true);
    expect(body.hasAttribute("data-shisho-own-align")).toBe(false);
  });

  it("lays the shown section out again once a newly chosen font arrives", async () => {
    const { renderer } = await openBook();
    let fontArrives = () => {};
    const doc = {
      fonts: {
        check: vi.fn(() => false),
        load: vi.fn(
          () => new Promise<void>((resolve) => (fontArrives = resolve)),
        ),
      },
    };
    const view = renderer as MockRenderer & {
      getContents: () => unknown[];
      render: ReturnType<typeof vi.fn>;
    };
    view.getContents = () => [{ doc }];
    view.render = vi.fn();
    const user = await openSettings();
    await user.click(screen.getByRole("combobox", { name: "Font" }));
    await user.click(screen.getByRole("option", { name: "Literata" }));

    expect(doc.fonts.load).toHaveBeenCalledWith('1em "Literata"');
    expect(view.render).not.toHaveBeenCalled();
    await act(async () => fontArrives());
    expect(view.render).toHaveBeenCalled();
  });

  it("offers the number of pages only for the paginated layout", async () => {
    await openBook();
    const user = await openSettings();
    const pages = screen.getByRole("group", { name: "Pages" });
    expect(within(pages).getByRole("button", { name: "One" })).toBeEnabled();

    await pick("Scrolled")(user);

    expect(within(pages).getByRole("button", { name: "One" })).toBeDisabled();
    expect(screen.getByText("Only for Paginated")).toBeInTheDocument();
  });

  it("doesn't save display settings to the account", async () => {
    const mutate = vi.fn();
    vi.mocked(useUpdateUserSettings).mockReturnValue({ mutate } as never);
    await openBook();
    const user = await openSettings();
    await pick("Dark")(user);
    expect(mutate).not.toHaveBeenCalled();
    expect(JSON.parse(localStorage.getItem(READER_SETTINGS_KEY)!)).toEqual(
      expect.objectContaining({ theme: "dark" }),
    );
  });

  it("starts from the account's old settings in a browser that stored none", async () => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: {
        viewer_hide_chrome: false,
        viewer_reflowable_font_size: 140,
        viewer_reflowable_theme: "sepia",
        viewer_reflowable_flow: "scrolled",
      },
      isLoading: false,
    } as never);
    const { renderer } = await openBook();
    // 140% of the browser's 16px.
    expect(css(renderer)).toContain("font-size: 22.5px !important");
    expect(css(renderer)).toContain("background: #f4ecd8 !important");
    expect(attribute(renderer, "flow")).toBe("scrolled");
  });

  it("follows a change made in another tab", async () => {
    const { renderer } = await openBook();
    act(() => {
      localStorage.setItem(
        READER_SETTINGS_KEY,
        JSON.stringify({ theme: "dark" }),
      );
      window.dispatchEvent(
        new StorageEvent("storage", { key: READER_SETTINGS_KEY }),
      );
    });
    expect(css(renderer)).toContain("background: #1a1a1a !important");
  });

  const fontFile = (name: string) =>
    new File([new Uint8Array([0, 1, 0, 0])], name);

  it("imports a font, reads in it, keeps it, and removes it", async () => {
    const first = await openBook();
    const user = await openSettings();
    await user.upload(screen.getByLabelText("Font file"), fontFile("Mine.ttf"));

    const imported = await screen.findByRole("list", {
      name: "Imported fonts",
    });
    expect(within(imported).getByText("Mine")).toBeInTheDocument();
    await vi.waitFor(() =>
      expect(css(first.renderer)).toContain(
        'url("blob:http://localhost/font-1") format("truetype")',
      ),
    );
    expect(css(first.renderer)).toMatch(/font-family: "Imported Font /);
    first.unmount();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith(
      "blob:http://localhost/font-1",
    );

    const reopened = await openBook();
    await vi.waitFor(() =>
      expect(css(reopened.renderer)).toContain(
        'url("blob:http://localhost/font-2")',
      ),
    );
    const user2 = await openSettings();
    await user2.click(
      await screen.findByRole("button", { name: "Remove Mine" }),
    );

    await vi.waitFor(() =>
      expect(css(reopened.renderer)).not.toContain("font-family"),
    );
    expect(
      screen.queryByRole("list", { name: "Imported fonts" }),
    ).not.toBeInTheDocument();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith(
      "blob:http://localhost/font-2",
    );
    expect(JSON.parse(localStorage.getItem(READER_SETTINGS_KEY)!).font).toBe(
      "book",
    );
  });

  it("waits for an imported font before styling the book", async () => {
    const first = await openBook();
    const user = await openSettings();
    await user.upload(screen.getByLabelText("Font file"), fontFile("Mine.otf"));
    await screen.findByRole("list", { name: "Imported fonts" });
    first.unmount();

    const { container } = renderReader();
    const view = container.querySelector("foliate-view") as unknown as {
      renderer: MockRenderer;
    };
    await vi.waitFor(() => expect(view.renderer.setStyles).toHaveBeenCalled());
    // The first styles it gets already use the font.
    expect(view.renderer.setStyles.mock.calls[0][0]).toContain(
      'format("opentype")',
    );
  });

  it("says plainly when the browser can't store an imported font", async () => {
    const error = vi.spyOn(toast, "error");
    // A browser's storage errors are Errors with the browser's own wording.
    vi.stubGlobal("indexedDB", {
      open: () => {
        throw new Error("The quota has been exceeded.");
      },
    });
    await openBook();
    const user = await openSettings();
    await user.upload(screen.getByLabelText("Font file"), fontFile("Mine.ttf"));

    await vi.waitFor(() =>
      expect(error).toHaveBeenCalledWith("Failed to import the font"),
    );
  });

  it("refuses a file that isn't a font", async () => {
    const error = vi.spyOn(toast, "error");
    await openBook();
    const user = userEvent.setup({
      advanceTimers: vi.advanceTimersByTime,
      applyAccept: false,
    });
    await user.click(screen.getByRole("button", { name: "Settings" }));
    await user.upload(screen.getByLabelText("Font file"), fontFile("a.zip"));

    await vi.waitFor(() =>
      expect(error).toHaveBeenCalledWith(
        "Choose a TTF, OTF, or WOFF2 font file.",
      ),
    );
    expect(
      screen.queryByRole("list", { name: "Imported fonts" }),
    ).not.toBeInTheDocument();
  });
});

describe("ReflowableReader footnotes", () => {
  const EPUB_NS = "http://www.idpf.org/2007/ops";
  const xhtml = (body: string) =>
    new DOMParser().parseFromString(
      `<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="${EPUB_NS}"><body>${body}</body></html>`,
      "application/xhtml+xml",
    );

  type MockView = HTMLElement & {
    goTo: ReturnType<typeof vi.fn>;
    renderer: { getContents: () => unknown[] };
  };

  // An AZW3 book resolves links asynchronously.
  const openBook = async (doc: Document, { async = false } = {}) => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: { viewer_hide_chrome: false },
      isLoading: false,
    } as never);
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);
    const { container } = renderReader();
    const view = container.querySelector("foliate-view") as unknown as MockView;
    await screen.findByRole("button", { name: "Next page", hidden: true });
    view.renderer.getContents = () => [{ doc, index: 1 }];
    Object.defineProperty(view, "book", {
      value: {
        resolveHref: (href: string) => {
          const resolved = {
            index: 1,
            anchor: (d: Document) => d.getElementById(href.split("#")[1]),
          };
          return async ? Promise.resolve(resolved) : resolved;
        },
        sections: [],
      },
    });
    return view;
  };

  // foliate's report that a link in the book was clicked. Returns whether
  // foliate would go on to follow it.
  const clickLink = (view: HTMLElement, a: Element, href: string) => {
    let follow = true;
    act(() => {
      follow = view.dispatchEvent(
        new CustomEvent("link", { detail: { a, href }, cancelable: true }),
      );
    });
    return follow;
  };

  const NOTE_DOC = `<p>Text<a epub:type="noteref" href="#n1">1</a> and <a href="#end">a link</a>.</p>
    <aside epub:type="footnote" id="n1"><p>The footnote's text.</p></aside>
    <p id="end">The end.</p>`;

  it("opens a footnote in a popover instead of jumping to it", async () => {
    const doc = xhtml(NOTE_DOC);
    const view = await openBook(doc);

    const follow = clickLink(
      view,
      doc.querySelector("a")!,
      "chapter2.xhtml#n1",
    );

    expect(follow).toBe(false);
    const popover = await screen.findByRole("dialog", { name: "Footnote" });
    expect(within(popover).getByText("The footnote's text.")).toBeVisible();
    expect(view.goTo).not.toHaveBeenCalled();
  });

  it("opens a footnote in an AZW3 book, whose links resolve later", async () => {
    const doc = xhtml(NOTE_DOC);
    const view = await openBook(doc, { async: true });

    expect(clickLink(view, doc.querySelector("a")!, "part2.html#n1")).toBe(
      false,
    );

    const popover = await screen.findByRole("dialog", { name: "Footnote" });
    expect(within(popover).getByText("The footnote's text.")).toBeVisible();
    expect(view.goTo).not.toHaveBeenCalled();
  });

  it("shows a footnote in the reader's theme and type, not the app's", async () => {
    localStorage.setItem(
      READER_SETTINGS_KEY,
      JSON.stringify({ theme: "black", font: "literata", size: 20 }),
    );
    const doc = xhtml(NOTE_DOC);
    const view = await openBook(doc);

    clickLink(view, doc.querySelector("a")!, "chapter2.xhtml#n1");

    const popover = await screen.findByRole("dialog", { name: "Footnote" });
    expect(popover).toHaveStyle({
      backgroundColor: "rgb(0, 0, 0)",
      color: "rgb(200, 200, 200)",
      fontSize: "18px",
    });
    // The chosen font, under a name that can't replace the app's own fonts.
    expect(popover.style.fontFamily).toMatch(/^"?Shisho Note Font"?, serif$/);
    expect(popover.querySelector("style")?.textContent).toMatch(
      /@font-face \{\s*font-family: "Shisho Note Font";\s*src: url\("[^"]*Literata/,
    );
    expect(
      within(popover).getByRole("button", { name: "Go to note" }),
    ).toHaveStyle({ color: "rgb(138, 180, 248)" });
  });

  it("links from the popover to the full note", async () => {
    const doc = xhtml(NOTE_DOC);
    const view = await openBook(doc);
    clickLink(view, doc.querySelector("a")!, "chapter2.xhtml#n1");
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    await user.click(await screen.findByRole("button", { name: "Go to note" }));

    expect(view.goTo).toHaveBeenCalledWith("chapter2.xhtml#n1");
    expect(
      screen.queryByRole("dialog", { name: "Footnote" }),
    ).not.toBeInTheDocument();
  });

  it("counts going to the note as a jump", async () => {
    const doc = xhtml(NOTE_DOC);
    const view = await openBook(doc);
    const relocate = (cfi: string, section: number) =>
      view.dispatchEvent(
        new CustomEvent("relocate", {
          detail: { cfi, section: { current: section }, reason: "navigation" },
        }),
      );
    act(() => relocate("reading", 1));
    view.goTo.mockImplementation(() => {
      relocate("notes", 2);
      return Promise.resolve();
    });
    clickLink(view, doc.querySelector("a")!, "chapter2.xhtml#n1");
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    await user.click(await screen.findByRole("button", { name: "Go to note" }));

    expect(
      screen.getByRole("button", { name: "Back to previous location" }),
    ).toBeEnabled();
  });

  it.each(["page", "anchor"])(
    "closes the popover when the page moves or lays out again (%s)",
    async (reason) => {
      const doc = xhtml(NOTE_DOC);
      const view = await openBook(doc);
      clickLink(view, doc.querySelector("a")!, "chapter2.xhtml#n1");
      await screen.findByRole("dialog", { name: "Footnote" });

      act(() => {
        view.dispatchEvent(
          new CustomEvent("relocate", {
            detail: { fraction: 0.5, reason },
          }),
        );
      });

      expect(
        screen.queryByRole("dialog", { name: "Footnote" }),
      ).not.toBeInTheDocument();
    },
  );

  it("closes the popover on a tap in the book", async () => {
    const doc = xhtml(NOTE_DOC);
    const view = await openBook(doc);
    // foliate's report of the section, which the reader listens to.
    act(() => {
      view.dispatchEvent(new CustomEvent("load", { detail: { doc } }));
    });
    clickLink(view, doc.querySelector("a")!, "chapter2.xhtml#n1");
    await screen.findByRole("dialog", { name: "Footnote" });

    act(() => {
      doc
        .querySelector("p")!
        .dispatchEvent(new Event("pointerdown", { bubbles: true }));
    });

    expect(
      screen.queryByRole("dialog", { name: "Footnote" }),
    ).not.toBeInTheDocument();
  });

  it("follows an ordinary link", async () => {
    const doc = xhtml(NOTE_DOC);
    const view = await openBook(doc);

    clickLink(view, doc.querySelectorAll("a")[1], "chapter2.xhtml#end");
    await act(async () => {});

    expect(view.goTo).toHaveBeenCalledWith("chapter2.xhtml#end");
    expect(
      screen.queryByRole("dialog", { name: "Footnote" }),
    ).not.toBeInTheDocument();
  });

  it("jumps to a note it can't read the text of", async () => {
    const doc = xhtml(`<p>Text<sup><a href="#gone">1</a></sup></p>`);
    const view = await openBook(doc);

    clickLink(view, doc.querySelector("a")!, "chapter2.xhtml#gone");

    await vi.waitFor(() =>
      expect(view.goTo).toHaveBeenCalledWith("chapter2.xhtml#gone"),
    );
    expect(
      screen.queryByRole("dialog", { name: "Footnote" }),
    ).not.toBeInTheDocument();
  });
});

describe("ReflowableReader search", () => {
  const openBook = async (hideChrome = false) => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: { viewer_hide_chrome: hideChrome },
      isLoading: false,
    } as never);
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);
    const { container } = renderReader();
    const view = container.querySelector(
      "foliate-view",
    ) as unknown as HTMLElement & {
      goLeft: ReturnType<typeof vi.fn>;
      goRight: ReturnType<typeof vi.fn>;
      goTo: ReturnType<typeof vi.fn>;
    };
    await vi.waitFor(() =>
      expect(screen.getByRole("button", { name: "Search" })).toBeEnabled(),
    );
    return { view };
  };
  const user = () => userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

  it("goes to a result the search found", async () => {
    mockSearchSections = [
      {
        label: "Chapter Four",
        subitems: [
          {
            cfi: "epubcfi(/6/8!/4/2,/1:3,/1:10)",
            excerpt: { pre: "No ", match: "lantern", post: "." },
          },
        ],
      },
    ];
    const { view } = await openBook();
    const u = user();
    await u.click(screen.getByRole("button", { name: "Search" }));
    await u.type(
      screen.getByRole("searchbox", { name: "Search in book" }),
      "lantern",
    );
    await u.click(await screen.findByRole("button", { name: "No lantern." }));
    expect(view.goTo).toHaveBeenCalledWith("epubcfi(/6/8!/4/2,/1:3,/1:10)");
  });

  it("doesn't turn pages on keys typed in the search field", async () => {
    const { view } = await openBook();
    const u = user();
    await u.click(screen.getByRole("button", { name: "Search" }));
    await u.type(
      screen.getByRole("searchbox", { name: "Search in book" }),
      "dad{ArrowLeft}{ArrowRight}",
    );
    expect(view.goLeft).not.toHaveBeenCalled();
    expect(view.goRight).not.toHaveBeenCalled();
  });

  it("keeps the controls showing while it's open", async () => {
    await openBook(true);
    const u = user();
    await u.click(screen.getByRole("button", { name: "Search" }));
    act(() => vi.advanceTimersByTime(5000));
    expect(screen.getByRole("banner")).not.toHaveClass("-translate-y-full");

    await u.click(screen.getByRole("button", { name: "Close search" }));
    act(() => vi.advanceTimersByTime(5000));
    expect(screen.getByRole("banner")).toHaveClass("-translate-y-full");
  });

  it("closes when the display settings open, so the two don't overlap", async () => {
    await openBook();
    const u = user();
    await u.click(screen.getByRole("button", { name: "Search" }));
    expect(
      screen.getByRole("searchbox", { name: "Search in book" }),
    ).toBeInTheDocument();
    await u.click(screen.getByRole("button", { name: "Settings" }));
    expect(
      screen.queryByRole("searchbox", { name: "Search in book" }),
    ).toBeNull();
  });
});

describe("ReflowableReader location history", () => {
  type HistoryView = HTMLElement & {
    goTo: ReturnType<typeof vi.fn>;
    goToFraction: ReturnType<typeof vi.fn>;
    init: ReturnType<typeof vi.fn>;
  };

  // foliate reports where it is with each move. A place's section stands
  // for its screen: the mocked renderer always shows the same page number.
  const relocate = (
    view: HTMLElement,
    cfi: string,
    section: number,
    reason = "page",
  ) =>
    act(() => {
      sectionOf[cfi] = section;
      view.dispatchEvent(
        new CustomEvent("relocate", {
          detail: {
            cfi,
            fraction: section / 10,
            section: { current: section },
            reason,
          },
        }),
      );
    });

  // Where each target of a jump lands.
  let landsAt: Record<string, [string, number]> = {};
  // The section of each place foliate reported, for resolving it again.
  let sectionOf: Record<string, number> = {};

  const openBook = async () => {
    vi.mocked(useUserSettings).mockReturnValue({
      data: { viewer_hide_chrome: false },
      isLoading: false,
    } as never);
    vi.mocked(useReflowableBlob).mockReturnValue({
      data: new Blob(["x"], { type: "application/epub+zip" }),
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);
    const { container } = renderReader();
    const view = container.querySelector(
      "foliate-view",
    ) as unknown as HistoryView;
    await vi.waitFor(() => expect(view.init).toHaveBeenCalled());
    await act(async () => {});
    const land = (target: string) => {
      const [cfi, section] = landsAt[target] ?? [target, 99];
      relocate(view, cfi, section, "navigation");
      return Promise.resolve();
    };
    view.goTo.mockImplementation(land);
    Object.assign(view, {
      resolveNavigation: (cfi: string) =>
        cfi in sectionOf ? { index: sectionOf[cfi] } : null,
    });
    view.goToFraction.mockImplementation((f: number) => land(`fraction:${f}`));
    // Where the book opened.
    relocate(view, "start", 0, "anchor");
    return view;
  };

  const back = () =>
    screen.getByRole("button", { name: "Back to previous location" });
  const forward = () =>
    screen.getByRole("button", { name: "Forward to next location" });
  const user = () => userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  const toTheEnd = async (u: ReturnType<typeof user>) => {
    screen.getByRole("slider", { name: "Reading progress" }).focus();
    await u.keyboard("{End}");
  };

  beforeEach(() => {
    landsAt = { "fraction:1": ["end", 9] };
    sectionOf = {};
  });

  it("goes back to where a jump left from, then forward to where back left", async () => {
    const view = await openBook();
    const u = user();
    expect(back()).toBeDisabled();
    expect(forward()).toBeDisabled();

    await toTheEnd(u);
    expect(back()).toBeEnabled();
    expect(forward()).toBeDisabled();

    // Paging around where the jump landed.
    relocate(view, "end-later", 9);
    await u.click(back());
    expect(view.goTo).toHaveBeenLastCalledWith("start");
    expect(back()).toBeDisabled();
    expect(forward()).toBeEnabled();

    await u.click(forward());
    expect(view.goTo).toHaveBeenLastCalledWith("end-later");
    expect(back()).toBeEnabled();
    expect(forward()).toBeDisabled();
  });

  it("saves going back as the reading position", async () => {
    landsAt.start = ["start", 0];
    await openBook();
    const u = user();
    await toTheEnd(u);
    await u.click(back());
    await act(async () => {
      vi.advanceTimersByTime(POSITION_SAVE_DELAY_MS);
    });
    const saves = vi
      .mocked(API.request)
      .mock.calls.filter(
        ([method, endpoint]) =>
          method === "POST" && endpoint === "/reading/files/7/position",
      )
      .map(([, , payload]) => (payload as UpdatePositionPayload).cfi);
    expect(saves[saves.length - 1]).toBe("start");
  });

  it("doesn't count turning pages, even into the next chapter", async () => {
    const view = await openBook();
    relocate(view, "page-2", 0);
    // foliate reports reaching the next section as a navigation too.
    relocate(view, "chapter-2", 1, "navigation");
    relocate(view, "chapter-2-page-2", 1, "scroll");
    expect(back()).toBeDisabled();
  });

  it("doesn't count a jump that doesn't move the page", async () => {
    landsAt["fraction:1"] = ["start", 0];
    await openBook();
    await toTheEnd(user());
    expect(back()).toBeDisabled();
  });

  it("counts following a link in the book", async () => {
    landsAt["chapter3.xhtml"] = ["chapter-3", 3];
    const view = await openBook();
    const doc = document.implementation.createHTMLDocument("section");
    doc.body.innerHTML = '<p><a href="chapter3.xhtml">On</a></p>';
    let followedByFoliate = true;
    act(() => {
      followedByFoliate = view.dispatchEvent(
        new CustomEvent("link", {
          detail: { a: doc.querySelector("a"), href: "chapter3.xhtml" },
          cancelable: true,
        }),
      );
    });
    // The reader follows it itself, to know where it left from.
    expect(followedByFoliate).toBe(false);
    await act(async () => {});
    expect(view.goTo).toHaveBeenCalledWith("chapter3.xhtml");
    expect(back()).toBeEnabled();
  });

  it("counts one search as one jump, from where it moved off the page", async () => {
    mockSearchSections = [
      {
        label: "Chapter One",
        subitems: ["here", "there", "further"].map((cfi) => ({
          cfi,
          excerpt: { pre: "", match: "lantern", post: ` ${cfi}` },
        })),
      },
    ];
    landsAt = {
      here: ["start", 0],
      there: ["there", 4],
      further: ["further", 6],
    };
    const view = await openBook();
    const u = user();
    await u.click(screen.getByRole("button", { name: "Search" }));
    const field = screen.getByRole("searchbox", { name: "Search in book" });
    await u.type(field, "lantern");
    await screen.findByRole("button", { name: "lantern here" });

    // The first result is on the page the search started from.
    await u.type(field, "{Enter}");
    await act(async () => {});
    expect(view.goTo).toHaveBeenLastCalledWith("here");
    expect(back()).toBeDisabled();

    await u.type(field, "{Enter}");
    await act(async () => {});
    expect(view.goTo).toHaveBeenLastCalledWith("there");
    expect(back()).toBeEnabled();
    await u.type(field, "{Enter}");
    await act(async () => {});
    expect(view.goTo).toHaveBeenLastCalledWith("further");

    await u.click(back());
    expect(view.goTo).toHaveBeenLastCalledWith("start");
    expect(back()).toBeDisabled();
  });

  it("skips going back to a place already on screen", async () => {
    landsAt = { a: ["a", 3], b: ["b", 5] };
    const view = await openBook();
    // A section of four paragraphs, of which the screen shows the last two.
    const doc = document.implementation.createHTMLDocument("section");
    doc.body.innerHTML = "<p>1</p><p>2</p><p>3</p><p>4</p>";
    const paragraphs = doc.querySelectorAll("p");
    const screenRange = doc.createRange();
    screenRange.setStartBefore(paragraphs[2]);
    screenRange.setEndAfter(paragraphs[3]);
    Object.assign(view, {
      resolveNavigation: (cfi: string) => ({
        index: cfi in sectionOf ? sectionOf[cfi] : 7,
        anchor: () => paragraphs[cfi === "shown" ? 3 : 0],
      }),
    });
    const u = user();
    // Jump from the shown place to b, then from b on.
    relocate(view, "shown", 7);
    await toTheEnd(u);
    act(() =>
      view.dispatchEvent(
        new CustomEvent("relocate", {
          detail: {
            cfi: "shown-again",
            section: { current: 7 },
            range: screenRange,
            reason: "page",
          },
        }),
      ),
    );
    sectionOf["shown-again"] = 7;
    // Paged back to where the jump left from: Back has nowhere new to go.
    await u.click(back());
    expect(view.goTo).not.toHaveBeenCalledWith("shown");
    expect(back()).toBeDisabled();
  });

  it("counts a new search as a new jump", async () => {
    mockSearchSections = [
      {
        label: "Chapter One",
        subitems: ["there"].map((cfi) => ({
          cfi,
          excerpt: { pre: "", match: "lantern", post: ` ${cfi}` },
        })),
      },
    ];
    landsAt = { there: ["there", 4] };
    const view = await openBook();
    const u = user();
    await u.click(screen.getByRole("button", { name: "Search" }));
    const field = screen.getByRole("searchbox", { name: "Search in book" });
    await u.type(field, "lantern");
    await screen.findByRole("button", { name: "lantern there" });
    await u.type(field, "{Enter}");
    await act(async () => {});

    // Paged on, then searched again.
    relocate(view, "elsewhere", 6);
    await u.clear(field);
    await u.type(field, "lanterns");
    await screen.findByRole("button", { name: "lantern there" });
    await u.type(field, "{Enter}");
    await act(async () => {});

    await u.click(back());
    expect(view.goTo).toHaveBeenLastCalledWith("elsewhere");
    await u.click(back());
    expect(view.goTo).toHaveBeenLastCalledWith("start");
  });

  it("keeps the way back when a move doesn't happen", async () => {
    const view = await openBook();
    const u = user();
    await toTheEnd(u);
    // foliate ignores a move just after a page turn.
    view.goTo.mockImplementation(() => Promise.resolve());
    await u.click(back());
    expect(view.goTo).toHaveBeenLastCalledWith("start");
    expect(back()).toBeEnabled();
    expect(forward()).toBeDisabled();
  });

  // A section of paragraphs, and what foliate reports showing part of it.
  const section = () => {
    const doc = document.implementation.createHTMLDocument("section");
    doc.body.innerHTML =
      "<p>Zero zero</p><p>One one</p><p>Two two</p><p>Three three</p>";
    const text = Array.from(doc.querySelectorAll("p"), (p) => p.firstChild!);
    const range = (
      [startNode, startOffset]: [Node, number],
      [endNode, endOffset]: [Node, number],
    ) => {
      const r = doc.createRange();
      r.setStart(startNode, startOffset);
      r.setEnd(endNode, endOffset);
      return r;
    };
    return { text, range };
  };
  const relocateShowing = (
    view: HTMLElement,
    cfi: string,
    range: Range,
    reason = "page",
  ) => {
    sectionOf[cfi] = 3;
    act(() => {
      view.dispatchEvent(
        new CustomEvent("relocate", {
          detail: { cfi, section: { current: 3 }, range, reason },
        }),
      );
    });
  };

  it("counts a short jump that moves the page, though the old place still shows", async () => {
    const { text, range } = section();
    const view = await openBook();
    // The reader was at paragraph one; scrolling up a paragraph to zero
    // still shows the top of one.
    relocateShowing(view, "one", range([text[1], 0], [text[3], 5]));
    Object.assign(view, {
      resolveNavigation: (cfi: string) => ({
        index: sectionOf[cfi],
        anchor: () => range([text[1], 0], [text[1], 0]),
      }),
    });
    view.goToFraction.mockImplementation(() => {
      relocateShowing(
        view,
        "zero",
        range([text[0], 0], [text[2], 5]),
        "navigation",
      );
      return Promise.resolve();
    });
    await toTheEnd(user());
    expect(back()).toBeEnabled();
  });

  it("doesn't skip going back to the page just before, which ends where this one begins", async () => {
    const { text, range } = section();
    const view = await openBook();
    // The place the jump left from begins mid-paragraph, at the very point
    // where the page before it ends.
    relocateShowing(view, "origin", range([text[2], 3], [text[3], 5]));
    Object.assign(view, {
      resolveNavigation: (cfi: string) =>
        cfi === "origin"
          ? { index: 3, anchor: () => range([text[2], 3], [text[3], 5]) }
          : null,
    });
    view.goToFraction.mockImplementation(() => {
      relocate(view, "far", 9, "navigation");
      return Promise.resolve();
    });
    const u = user();
    await toTheEnd(u);
    // Paged back by hand to the page before the origin.
    relocateShowing(view, "page-before", range([text[0], 0], [text[2], 3]));
    await u.click(back());
    expect(view.goTo).toHaveBeenLastCalledWith("origin");
  });

  it("explains each button on hover, even before there's anywhere to go", async () => {
    await openBook();
    const u = user();
    expect(back()).toBeDisabled();
    // A disabled button gets no pointer events, so the tooltip hangs from
    // what holds it.
    await u.hover(back().parentElement!);
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "Back to where you were before a jump, such as a link, a chapter, or a search result",
    );
    await u.unhover(back().parentElement!);
    await u.hover(forward().parentElement!);
    expect(
      await screen.findByRole("tooltip", { name: /Forward to where you were/ }),
    ).toHaveTextContent("Forward to where you were when you went back");
  });
});
