import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { MetadataMergeDialog } from "./MetadataMergeDialog";

const createUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

const entities = [
  { id: 10, name: "Dutton", count: 5 },
  { id: 20, name: "Riverhead", count: 3 },
];

describe("MetadataMergeDialog", () => {
  it("clears parent search state after a merge", async () => {
    const user = createUser();
    const onMerge = vi.fn().mockResolvedValue(undefined);
    const onSearch = vi.fn();

    render(
      <MetadataMergeDialog
        entities={entities}
        entityType="publisher"
        isLoadingEntities={false}
        isPending={false}
        onMerge={onMerge}
        onOpenChange={vi.fn()}
        onSearch={onSearch}
        open={true}
        targetId={1}
        targetName="Penguin"
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.type(
      screen.getByPlaceholderText("Search publishers..."),
      "Dutton",
    );
    await user.click(screen.getByText("Dutton"));

    await user.click(screen.getByRole("button", { name: "Merge" }));

    await waitFor(() => {
      expect(onMerge).toHaveBeenCalledWith(10);
    });
    expect(onSearch).toHaveBeenLastCalledWith("");
  });

  it("clears parent search state when the dialog closes", async () => {
    const user = createUser();
    const onOpenChange = vi.fn();
    const onSearch = vi.fn();

    render(
      <MetadataMergeDialog
        entities={entities}
        entityType="publisher"
        isLoadingEntities={false}
        isPending={false}
        onMerge={vi.fn()}
        onOpenChange={onOpenChange}
        onSearch={onSearch}
        open={true}
        targetId={1}
        targetName="Penguin"
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.type(
      screen.getByPlaceholderText("Search publishers..."),
      "Dutton",
    );
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onSearch).toHaveBeenLastCalledWith("");
  });
});
