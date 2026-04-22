import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import { useEpubBlob } from "@/hooks/queries/epub";

import EPUBReader from "./EPUBReader";

vi.mock("@/hooks/queries/epub", () => ({
  useEpubBlob: vi.fn(),
}));

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
});
