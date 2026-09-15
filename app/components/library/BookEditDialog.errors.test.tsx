import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Book } from "@/types";

import { BookEditDialog } from "./BookEditDialog";

const book: Book = {
  id: 1,
  library_id: 1,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
  filepath: "/library/book",
  title: "Test Book",
  title_source: "manual",
  sort_title: "Test Book",
  sort_title_source: "manual",
  author_source: "manual",
  cover_cache_key: "1",
  files: [
    {
      id: 10,
      book_id: 1,
      library_id: 1,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      filepath: "/library/book.epub",
      file_type: "epub",
      file_role: "main",
      filesize_bytes: 100,
      is_preferred_cover: false,
      reviewed: false,
    },
  ],
};

function Editor() {
  const [open, setOpen] = useState(true);
  return <BookEditDialog book={book} onOpenChange={setOpen} open={open} />;
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("book save failures", () => {
  it.each([
    {
      path: "/api/books/1",
      status: 403,
      message: "This action is unavailable in the demo.",
    },
    { path: "/api/books/1", status: 500, message: "Could not save metadata." },
    {
      path: "/api/books/1/review",
      status: 403,
      message: "Review changes are not allowed.",
    },
  ])(
    "shows $status from $path and preserves edits for retry",
    async ({ path, status, message }) => {
      vi.stubGlobal("__APP_VERSION__", "test");
      let fail = true;
      vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
        const url = String(input).split("?")[0];
        if (init?.method !== "GET") {
          if (url === path && fail) {
            return Response.json(
              { error: { code: "rejected", message } },
              { status },
            );
          }
          return Response.json({ ...book, title: "Edited title" });
        }
        if (url === "/api/settings/review-criteria") {
          return Response.json({ book_fields: [], audiobook_fields: [] });
        }
        return Response.json({ items: [], total: 0 });
      });
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      });
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const view = render(
        <QueryClientProvider client={client}>
          <Editor />
        </QueryClientProvider>,
      );
      try {
        await user.clear(screen.getByLabelText("Title", { exact: true }));
        await user.type(
          screen.getByLabelText("Title", { exact: true }),
          "Edited title",
        );
        if (path.endsWith("/review"))
          await user.click(screen.getByRole("switch"));
        await user.click(screen.getByRole("button", { name: "Save Changes" }));

        expect(await screen.findByRole("alert")).toHaveTextContent(message);
        expect(
          screen.getByRole("dialog", { name: "Edit Book" }),
        ).toBeInTheDocument();
        expect(screen.getByLabelText("Title", { exact: true })).toHaveValue(
          "Edited title",
        );
        if (path.endsWith("/review"))
          expect(screen.getByRole("switch")).toBeChecked();

        fail = false;
        await user.click(screen.getByRole("button", { name: "Save Changes" }));
        await waitFor(() =>
          expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
        );
      } finally {
        view.unmount();
        client.clear();
      }
    },
  );
});
