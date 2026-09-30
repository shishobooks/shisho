import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { LibraryPluginOrderPlugin } from "@/hooks/queries/plugins";

import LibraryPluginsTab from "./LibraryPluginsTab";

const plugins: LibraryPluginOrderPlugin[] = [
  { scope: "shisho", id: "alpha", name: "Alpha", mode: "enabled" },
  { scope: "shisho", id: "beta", name: "Beta", mode: "enabled" },
];

let savePending = false;

vi.mock("@/hooks/queries/plugins", () => ({
  useLibraryPluginOrder: () => ({
    data: { customized: true, plugins },
    error: null,
    isLoading: false,
  }),
  useSetLibraryPluginOrder: () => ({ mutate: vi.fn(), isPending: savePending }),
  useResetLibraryPluginOrder: () => ({ mutate: vi.fn(), isPending: false }),
}));

const createUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

// The move buttons are unlabeled icon buttons, Move up then Move down for
// each row, so Alpha's Move down is the second one.
const moveAlphaDown = async (user: ReturnType<typeof createUser>) => {
  await user.click(screen.getAllByRole("button", { name: "" })[1]);
};

describe("LibraryPluginsTab Save button", () => {
  beforeEach(() => {
    savePending = false;
  });

  it("is disabled until the order changes", async () => {
    const user = createUser();
    render(<LibraryPluginsTab libraryId="1" />);
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();

    await moveAlphaDown(user);

    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  });

  it("is disabled while the save is pending", async () => {
    const user = createUser();
    const { rerender } = render(<LibraryPluginsTab libraryId="1" />);
    await moveAlphaDown(user);
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();

    savePending = true;
    rerender(<LibraryPluginsTab libraryId="1" />);

    expect(screen.getByRole("button", { name: "Saving..." })).toBeDisabled();
  });
});
