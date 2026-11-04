import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Toaster } from "@/components/ui/sonner";
import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import { showReadingFinishedToast } from "./ReadingFinishedToast";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
vi.mock("@/components/contexts/Theme/context", () => ({
  useTheme: () => ({ theme: "dark" }),
}));

describe("showReadingFinishedToast", () => {
  beforeEach(() => {
    setAuth({ permissions: ["books:read"] });
    vi.spyOn(API, "request").mockResolvedValue({
      book_id: 7,
      rating: null,
      read_throughs: [],
      positions: [],
    });
  });
  afterEach(() => {
    act(() => {
      toast.dismiss();
    });
    vi.restoreAllMocks();
  });

  // It covers the reader's footer for as long as it lasts, so it can be cleared
  // to see underneath, which leaves the finish alone.
  it("can be dismissed without undoing the finish", async () => {
    const onUndo = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <Toaster />
      </QueryClientProvider>,
    );
    act(() => {
      showReadingFinishedToast({ bookId: 7, onUndo });
    });
    expect(await screen.findByText("Marked as read")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Close toast" }));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(screen.queryByText("Marked as read")).not.toBeInTheDocument();
    expect(onUndo).not.toHaveBeenCalled();
  });
});
