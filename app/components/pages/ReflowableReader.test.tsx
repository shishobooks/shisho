import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
          return { toc: mockToc, dir: mockDir };
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
  mockDir = undefined;
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
        vi.useFakeTimers();
        const result = setup(true);
        act(() => vi.advanceTimersByTime(2000));
        expect(result.header).toHaveClass("-translate-y-full");
        return result;
      };
      afterEach(() => {
        vi.useRealTimers();
      });

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
    act(() => {
      vi.advanceTimersByTime(1000);
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
