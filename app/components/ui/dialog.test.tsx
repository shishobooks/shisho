import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "./dialog";

describe("DialogContent open focus", () => {
  it("focuses the dialog itself rather than the header's close button", async () => {
    render(
      <Dialog open>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Share</DialogTitle>
            <DialogDescription>Share this book</DialogDescription>
          </DialogHeader>
          <DialogBody>
            <input aria-label="Label" />
          </DialogBody>
        </DialogContent>
      </Dialog>,
    );

    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect(dialog).toHaveFocus());
    expect(screen.getByRole("button", { name: "Close" })).not.toHaveFocus();
  });

  it("keeps focusing the first field of a dialog that has no header close button", async () => {
    render(
      <Dialog open>
        <DialogContent aria-describedby={undefined}>
          <DialogTitle className="sr-only">Search</DialogTitle>
          <input aria-label="Search" />
        </DialogContent>
      </Dialog>,
    );

    const input = await screen.findByRole("textbox", { name: "Search" });
    await waitFor(() => expect(input).toHaveFocus());
  });

  it("leaves focus alone when the caller prevents the default", async () => {
    render(
      <Dialog open>
        <DialogContent onOpenAutoFocus={(event) => event.preventDefault()}>
          <DialogHeader>
            <DialogTitle>Custom</DialogTitle>
            <DialogDescription>Custom focus</DialogDescription>
          </DialogHeader>
        </DialogContent>
      </Dialog>,
    );

    const dialog = await screen.findByRole("dialog");
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(dialog).not.toHaveFocus();
    expect(screen.getByRole("button", { name: "Close" })).not.toHaveFocus();
  });
});
