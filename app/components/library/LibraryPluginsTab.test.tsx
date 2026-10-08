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

// Alpha is the first row, so its Move down is the first one.
const moveAlphaDown = async (user: ReturnType<typeof createUser>) => {
  await user.click(
    screen.getAllByRole("button", { name: "Move plugin down" })[0],
  );
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
