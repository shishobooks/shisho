import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";

import { ShishoAPIError } from "@/libraries/api";
import { watchUnhandledRejections } from "@/testing/mutations";

import { MetadataDeleteDialog } from "./MetadataDeleteDialog";

const renderDialog = (
  onDelete: () => Promise<unknown>,
  onOpenChange = vi.fn(),
) =>
  render(
    <MetadataDeleteDialog
      entityName="Fantasy"
      entityType="genre"
      isPending={false}
      onDelete={onDelete}
      onOpenChange={onOpenChange}
      open={true}
    />,
  );

describe("MetadataDeleteDialog", () => {
  it.each([
    [1, "It will be removed from 1 book."],
    [1234, "It will be removed from 1,234 books."],
  ])("states the impact on %i affected books", (affectedBookCount, impact) => {
    render(
      <MetadataDeleteDialog
        affectedBookCount={affectedBookCount}
        entityName="Fantasy"
        entityType="genre"
        isPending={false}
        onDelete={vi.fn()}
        onOpenChange={vi.fn()}
        open={true}
      />,
    );

    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent(impact);
    expect(dialog).toHaveTextContent(
      "A metadata refresh or reset can add it back if a book's files or plugins still name it.",
    );
  });

  it("wraps the name at word boundaries", () => {
    renderDialog(vi.fn());

    expect(screen.getByText('"Fantasy"')).toHaveClass("break-words");
  });

  it("states no impact when nothing is affected", () => {
    renderDialog(vi.fn());

    const dialog = screen.getByRole("dialog");
    expect(dialog).not.toHaveTextContent("It will be removed from");
    expect(dialog).not.toHaveTextContent("can add it back");
  });

  it("toasts a rejected delete and stays open", async () => {
    const toastError = vi.spyOn(toast, "error");
    // A plain async function, not a vi.fn: Vitest's spy handles the promise
    // its mock returns, which would hide an unhandled rejection.
    const onDelete = async () => {
      throw new ShishoAPIError("Genre still has books", "conflict", 409);
    };
    const onOpenChange = vi.fn();
    const watcher = watchUnhandledRejections();
    renderDialog(onDelete, onOpenChange);

    await userEvent.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "Genre still has books",
        undefined,
      );
    });
    expect(await watcher.stop()).toEqual([]);
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("names the action when the server gives no message", async () => {
    const toastError = vi.spyOn(toast, "error");
    const watcher = watchUnhandledRejections();
    renderDialog(async () => {
      throw new TypeError("Failed to fetch");
    });

    await userEvent.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(toastError).toHaveBeenCalledWith(
        "Failed to delete genre",
        undefined,
      );
    });
    expect(await watcher.stop()).toEqual([]);
  });
});
