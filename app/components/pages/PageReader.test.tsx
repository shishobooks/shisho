import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { toast } from "sonner";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { rejectingMutate, REJECTION_MESSAGE } from "@/testing/mutations";

import PageReader from "./PageReader";

const settings = vi.hoisted(() => ({
  mutate: undefined as unknown as (...args: never[]) => void,
}));

vi.mock("@/hooks/queries/settings", () => ({
  useUserSettings: () => ({
    data: { preload_count: 3, fit_mode: "fit-height" },
    isLoading: false,
  }),
  useUpdateUserSettings: () => ({ mutate: settings.mutate }),
}));

vi.mock("@/hooks/queries/chapters", () => ({
  useFileChapters: () => ({ data: [] }),
}));

beforeAll(() => {
  // jsdom has no layout, so it leaves scrollTo unimplemented.
  Element.prototype.scrollTo = () => {};
});

afterEach(() => {
  vi.restoreAllMocks();
});

const renderReader = () =>
  render(
    <MemoryRouter>
      <PageReader
        bookId={7}
        fileId={42}
        getPageUrl={(page) => `/page-${page}`}
        libraryId="1"
        totalPages={10}
      />
    </MemoryRouter>,
  );

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
