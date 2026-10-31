import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";

import { useDebounce } from "@/hooks/useDebounce";
import { ShishoAPIError } from "@/libraries/api";
import { watchUnhandledRejections } from "@/testing/mutations";
import {
  advancePastSearch,
  createSearchQuery,
  SEARCH_LATENCY_MS,
} from "@/testing/searchQuery";

import { MetadataMergeDialog } from "./MetadataMergeDialog";

const SOURCES = [
  { id: 30, name: "Ace", countText: "1 book" },
  { id: 40, name: "Bantam", countText: "2 books" },
  { id: 10, name: "Dutton", countText: "5 books" },
  { id: 20, name: "Riverhead", countText: "3 books" },
];

// Renders the dialog over a real search query, debounced as the detail
// pages do, and waits for the first results.
async function renderSearching() {
  const { useSearch, wrapper } = createSearchQuery(SOURCES, (e) => e.name);
  const Harness = () => {
    const [search, setSearch] = useState("");
    const debounced = useDebounce(search, 200);
    const results = useSearch(debounced);
    return (
      <MetadataMergeDialog
        entities={results.data ?? []}
        entityType="publisher"
        isLoadingEntities={results.isLoading}
        isPending={false}
        isSearchPending={search !== debounced || results.isPlaceholderData}
        onMerge={vi.fn()}
        onOpenChange={vi.fn()}
        onSearch={setSearch}
        open={true}
        targetId={1}
        targetName="Penguin"
      />
    );
  };
  render(<Harness />, { wrapper });
  await act(() => vi.advanceTimersByTimeAsync(SEARCH_LATENCY_MS));
}

const createUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

const entities = [
  { id: 10, name: "Dutton", countText: "5 books" },
  { id: 20, name: "Riverhead", countText: "3 books" },
];

describe("MetadataMergeDialog", () => {
  it("highlights the first source once results for the typed text arrive, so Enter picks it", async () => {
    const user = createUser();
    await renderSearching();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("riv");
    await advancePastSearch();
    await user.keyboard("{Enter}");

    expect(screen.getByRole("combobox")).toHaveTextContent("Riverhead");
  });

  it("holds an Enter pressed before results arrive and picks the first fresh source", async () => {
    const user = createUser();
    await renderSearching();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("riv{Enter}");
    // Past the debounce, the previous results still show while the search
    // runs.
    await act(() => vi.advanceTimersByTimeAsync(250));
    // The open popover hides the trigger from the accessibility tree.
    expect(
      screen.getByRole("combobox", {
        hidden: true,
        name: "Publisher to merge",
      }),
    ).toHaveTextContent("Select publisher");
    await advancePastSearch();

    expect(screen.getByRole("combobox")).toHaveTextContent("Riverhead");
  });

  it("keeps the picked source on the picker and merges it after a search that no longer lists it", async () => {
    const user = createUser();
    const onMerge = vi.fn().mockResolvedValue(undefined);
    const { useSearch, wrapper } = createSearchQuery(SOURCES, (e) => e.name);
    const Harness = () => {
      const [search, setSearch] = useState("");
      const debounced = useDebounce(search, 200);
      const results = useSearch(debounced);
      return (
        <MetadataMergeDialog
          entities={results.data ?? []}
          entityType="publisher"
          isLoadingEntities={results.isLoading}
          isPending={false}
          isSearchPending={search !== debounced || results.isPlaceholderData}
          onMerge={onMerge}
          onOpenChange={vi.fn()}
          onSearch={setSearch}
          open={true}
          targetId={1}
          targetName="Penguin"
        />
      );
    };
    render(<Harness />, { wrapper });
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_LATENCY_MS));

    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByRole("option", { name: /Dutton/ }));
    // Search again for something else, and leave without picking.
    await user.click(screen.getByRole("combobox"));
    await user.keyboard("riv");
    await advancePastSearch();
    await user.keyboard("{Escape}");

    expect(
      screen.getByRole("combobox", { name: "Publisher to merge" }),
    ).toHaveTextContent("Dutton (5 books)");
    expect(screen.getByText(/from "Dutton"/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Merge" }));
    expect(onMerge).toHaveBeenCalledExactlyOnceWith(10);
  });

  it("drops a held Enter when the source picker closes", async () => {
    const user = createUser();
    await renderSearching();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("riv{Enter}");
    // The modal popover turns off pointer events outside it, which
    // userEvent refuses to click through unless told not to check.
    await user
      .setup({ pointerEventsCheck: 0 })
      .click(screen.getByRole("heading", { hidden: true, name: /Merge into/ }));
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    await advancePastSearch();

    expect(screen.getByRole("combobox")).toHaveTextContent("Select publisher");
  });

  it("names the source picker and its search box by what they pick", async () => {
    const user = createUser();

    render(
      <MetadataMergeDialog
        entities={entities}
        entityType="genre"
        isLoadingEntities={false}
        isPending={false}
        isSearchPending={false}
        onMerge={vi.fn()}
        onOpenChange={vi.fn()}
        onSearch={vi.fn()}
        open={true}
        targetId={1}
        targetName="Fantasy"
      />,
    );

    const trigger = screen.getByRole("combobox", {
      name: "Genre to merge",
    });
    expect(trigger).toHaveTextContent("Select genre...");
    await user.click(trigger);
    expect(
      screen.getByRole("combobox", { name: "Search genres" }),
    ).toBeInTheDocument();
  });

  it("enables Merge only once a source is picked", async () => {
    const user = createUser();

    render(
      <MetadataMergeDialog
        entities={entities}
        entityType="genre"
        isLoadingEntities={false}
        isPending={false}
        isSearchPending={false}
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
      isSearchPending: false,
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
        isSearchPending={false}
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
        isSearchPending={false}
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
        isSearchPending={false}
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
        isSearchPending={false}
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
        isSearchPending={false}
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

  it("shows each source's count text in the list, on the picker, and in the merge sentence", async () => {
    const user = createUser();
    render(
      <MetadataMergeDialog
        entities={[{ id: 10, name: "Dutton", countText: "1,200 books" }]}
        entityType="genre"
        isLoadingEntities={false}
        isPending={false}
        isSearchPending={false}
        onMerge={vi.fn()}
        onOpenChange={vi.fn()}
        onSearch={vi.fn()}
        open={true}
        targetId={1}
        targetName="Fantasy"
      />,
    );

    const trigger = screen.getByRole("combobox", { name: "Genre to merge" });
    await user.click(trigger);
    expect(screen.getByRole("option", { name: /Dutton/ })).toHaveTextContent(
      "1,200 books",
    );
    await user.click(screen.getByRole("option", { name: /Dutton/ }));

    expect(trigger).toHaveTextContent("Dutton (1,200 books)");
    expect(screen.getByText(/Move all 1,200 books from/)).toBeInTheDocument();
  });
});
