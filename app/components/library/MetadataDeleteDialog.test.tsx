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
