import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import ReadingFinishedToastBody from "./ReadingFinishedToastBody";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

describe("ReadingFinishedToastBody", () => {
  beforeEach(() => {
    setAuth({ permissions: ["books:read"] });
    vi.spyOn(API, "request").mockResolvedValue({
      book_id: 3,
      status: "read",
      rating: null,
      on_to_read: false,
      read_throughs: [],
    });
  });

  it("offers stars to rate the book and an Undo button", async () => {
    const onUndo = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <ReadingFinishedToastBody bookId={3} onUndo={onUndo} />
      </QueryClientProvider>,
    );

    expect(await screen.findByTestId("star-rating")).toBeInTheDocument();
    await userEvent
      .setup({ advanceTimers: vi.advanceTimersByTime })
      .click(screen.getByRole("button", { name: "Undo" }));
    expect(onUndo).toHaveBeenCalledTimes(1);
  });
});
