import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";

import { ShishoAPIError } from "@/libraries/api";
import { watchUnhandledRejections } from "@/testing/mutations";

import { MetadataMergeDialog } from "./MetadataMergeDialog";

const createUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

const entities = [
  { id: 10, name: "Dutton", count: 5 },
  { id: 20, name: "Riverhead", count: 3 },
];

describe("MetadataMergeDialog", () => {
  it("enables Merge only once a source is picked", async () => {
    const user = createUser();

    render(
      <MetadataMergeDialog
        entities={entities}
        entityType="genre"
        isLoadingEntities={false}
        isPending={false}
        onMerge={vi.fn()}
        onOpenChange={vi.fn()}
        onSearch={vi.fn()}
        open={true}
        targetId={1}
        targetName="Fantasy"
      />,
    );

    expect(screen.getByRole("button", { name: "Merge" })).toBeDisabled();

    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByText("Dutton"));

    expect(screen.getByRole("button", { name: "Merge" })).toBeEnabled();
  });

  it("disables Set as child when the picked source becomes an ancestor", async () => {
    const user = createUser();
    const props = {
      entities,
      entityType: "publisher" as const,
      isLoadingEntities: false,
      isPending: false,
      onMerge: vi.fn(),
      onOpenChange: vi.fn(),
      onSearch: vi.fn(),
      open: true,
      targetId: 1,
      targetName: "Penguin",
    };

    const { rerender } = render(
      <MetadataMergeDialog
        {...props}
        setChildConfig={{
          disabledIds: [],
          isPending: false,
          onSetChild: vi.fn(),
        }}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByText("Dutton"));
    await user.click(screen.getByRole("radio", { name: /Set as child/i }));
    expect(screen.getByRole("button", { name: "Set as child" })).toBeEnabled();

    // The ancestor list refetches and now includes the picked source.
    rerender(
      <MetadataMergeDialog
        {...props}
        setChildConfig={{
          disabledIds: [10],
          isPending: false,
          onSetChild: vi.fn(),
        }}
      />,
    );
    expect(screen.getByRole("button", { name: "Set as child" })).toBeDisabled();
  });

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

  it("clears parent search state after setting a child", async () => {
    const user = createUser();
    const onSetChild = vi.fn().mockResolvedValue(undefined);
    const onSearch = vi.fn();

    render(
      <MetadataMergeDialog
        entities={entities}
        entityType="publisher"
        isLoadingEntities={false}
        isPending={false}
        onMerge={vi.fn()}
        onOpenChange={vi.fn()}
        onSearch={onSearch}
        open={true}
        setChildConfig={{
          disabledIds: [],
          isPending: false,
          onSetChild,
        }}
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
    await user.click(screen.getByRole("radio", { name: /Set as child/i }));
    await user.click(screen.getByRole("button", { name: /Set as child/i }));

    await waitFor(() => {
      expect(onSetChild).toHaveBeenCalledWith(10);
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

describe("MetadataMergeDialog failures", () => {
  it("toasts a rejected merge and stays open with the selection kept", async () => {
    const user = createUser();
    const toastError = vi.spyOn(toast, "error");
    const onMerge = vi
      .fn()
      .mockRejectedValue(
        new ShishoAPIError("Merge conflicts with an alias", "conflict", 409),
      );
    const onOpenChange = vi.fn();
    const watcher = watchUnhandledRejections();

    render(
      <MetadataMergeDialog
        entities={entities}
        entityType="genre"
        isLoadingEntities={false}
        isPending={false}
        onMerge={onMerge}
        onOpenChange={onOpenChange}
        onSearch={vi.fn()}
        open={true}
        targetId={1}
        targetName="Fantasy"
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByText("Dutton"));
    await user.click(screen.getByRole("button", { name: "Merge" }));

    await waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "Merge conflicts with an alias",
        undefined,
      );
    });
    expect(await watcher.stop()).toEqual([]);
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(screen.getByRole("combobox")).toHaveTextContent("Dutton");
  });

  it("toasts a rejected set-child with the fallback for a server fault", async () => {
    const user = createUser();
    const toastError = vi.spyOn(toast, "error");
    const onSetChild = vi
      .fn()
      .mockRejectedValue(
        new ShishoAPIError(
          "Internal Server Error",
          "internal_server_error",
          500,
        ),
      );
    const onOpenChange = vi.fn();
    const watcher = watchUnhandledRejections();

    render(
      <MetadataMergeDialog
        entities={entities}
        entityType="publisher"
        isLoadingEntities={false}
        isPending={false}
        onMerge={vi.fn()}
        onOpenChange={onOpenChange}
        onSearch={vi.fn()}
        open={true}
        setChildConfig={{ disabledIds: [], isPending: false, onSetChild }}
        targetId={1}
        targetName="Penguin"
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByText("Dutton"));
    await user.click(screen.getByRole("radio", { name: /Set as child/i }));
    await user.click(screen.getByRole("button", { name: /Set as child/i }));

    await waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "Failed to set as child",
        undefined,
      );
    });
    expect(await watcher.stop()).toEqual([]);
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(screen.getByRole("combobox")).toHaveTextContent("Dutton");
  });
});
