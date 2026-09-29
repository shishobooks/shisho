import { render, screen } from "@testing-library/react";
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
