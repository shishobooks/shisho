import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router-dom";
import { toast } from "sonner";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { rejectingMutate, REJECTION_MESSAGE } from "@/testing/mutations";
import type { PageFlow } from "@/types";

import PageReader from "./PageReader";

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

vi.mock("@/hooks/queries/chapters", () => ({
  useFileChapters: () => ({ data: [] }),
}));

// The saved flow, the flow the page shapes suggest, and the save mutation.
const flow = vi.hoisted(() => ({
  saved: null as PageFlow | null,
  detected: "paginated" as PageFlow | undefined,
  detectedFrom: undefined as string[] | null | undefined,
  readingLoading: false,
  mutate: undefined as unknown as (...args: never[]) => void,
}));

vi.mock("@/hooks/queries/reading", () => ({
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
});

// Shows the reader's URL query, which it updates as the page changes.
function LocationProbe() {
  return <output data-testid="location">{useLocation().search}</output>;
}

const getPageUrl = (page: number, size?: number) =>
  size ? `/page-${page}?size=${size}` : `/page-${page}`;

const renderReader = (totalPages = 10, title?: string, initialPath = "/") =>
  render(
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
    </MemoryRouter>,
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
      <MemoryRouter>
        <PageReader
          bookId={7}
          fileId={42}
          getPageUrl={getPageUrl}
          libraryId="1"
          totalPages={5}
        />
      </MemoryRouter>,
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
  it("prefetches nearby pages one at a time, but not in the scrolled flow", () => {
    settings.mutate = vi.fn();
    const { unmount } = renderReader(5);
    expect(document.querySelectorAll('link[rel="prefetch"]').length).toBe(3);
    unmount();

    // Lazy loading already fetches the pages near the view; prefetching
    // around every page scrolled past would queue PDF renders on the server.
    flow.saved = "scrolled";
    renderReader(5);
    expect(document.querySelectorAll('link[rel="prefetch"]')).toHaveLength(0);
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
