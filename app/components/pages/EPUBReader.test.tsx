import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeAll, describe, expect, it, vi } from "vitest";

import { useEpubBlob } from "@/hooks/queries/epub";
import {
  useUpdateViewerSettings,
  useViewerSettings,
} from "@/hooks/queries/settings";

import EPUBReader from "./EPUBReader";

// Prevent jsdom from trying to execute the real foliate view.js (it uses
// browser-only module specifiers and dynamic imports that jsdom can't resolve).
vi.mock("@/libraries/foliate/view.js", () => ({}));

vi.mock("@/hooks/queries/epub", () => ({
  useEpubBlob: vi.fn(),
}));

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
        book = { toc: [] };
      },
    );
  }
});

vi.mock("@/hooks/queries/settings", () => ({
  useViewerSettings: vi.fn(() => ({ data: undefined, isLoading: true })),
  useUpdateViewerSettings: vi.fn(() => ({ mutate: vi.fn() })),
}));

const renderReader = () => {
  const client = new QueryClient();
  const file = {
    id: 7,
    book_id: 3,
    file_type: "epub",
  } as never;
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <EPUBReader bookTitle="Test Book" file={file} libraryId="1" />
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

describe("EPUBReader", () => {
  it("shows a loading indicator while fetching the EPUB", () => {
    vi.mocked(useEpubBlob).mockReturnValue({
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
    vi.mocked(useEpubBlob).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error("boom"),
      refetch,
    } as never);

    renderReader();
    expect(screen.getByText(/couldn't load/i)).toBeInTheDocument();
    screen.getByRole("button", { name: /retry/i }).click();
    expect(refetch).toHaveBeenCalled();
  });

  it("shows the extended-wait hint after 10 seconds of loading", () => {
    vi.useFakeTimers();
    vi.mocked(useEpubBlob).mockReturnValue({
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

  it("updates settings when the theme button is clicked", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const mutate = vi.fn();
    vi.mocked(useViewerSettings).mockReturnValue({
      data: {
        preload_count: 3,
        fit_mode: "fit-height",
        viewer_epub_font_size: 100,
        viewer_epub_theme: "light",
        viewer_epub_flow: "paginated",
      },
      isLoading: false,
    } as never);
    vi.mocked(useUpdateViewerSettings).mockReturnValue({ mutate } as never);

    vi.mocked(useEpubBlob).mockReturnValue({
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
      expect.objectContaining({ viewer_epub_theme: "dark" }),
    );
  });
});
