import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { MetadataMergeDialog } from "./MetadataMergeDialog";

const entities = [
  { id: 10, name: "Dutton Books", count: 3 },
  { id: 20, name: "Ace Books", count: 2 },
];

describe("MetadataMergeDialog", () => {
  it("clears parent search after a merge succeeds", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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
        targetId={30}
        targetName="Target Publisher"
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.type(
      screen.getByPlaceholderText("Search publishers..."),
      "Dutton",
    );
    await user.click(screen.getByText("Dutton Books"));
    await user.click(screen.getByRole("button", { name: "Merge" }));

    await waitFor(() => {
      expect(onMerge).toHaveBeenCalledWith(10);
    });
    expect(onSearch).toHaveBeenLastCalledWith("");
  });

  it("clears parent search when the dialog closes without merging", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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
        targetId={30}
        targetName="Target Publisher"
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
