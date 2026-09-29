import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { FileRoleMain, type Book } from "@/types";

import { BookEditDialog } from "./BookEditDialog";

// Uses the real query hooks, so the People and Series gating inside them is
// what keeps the comboboxes quiet for a role without those permissions.

const auth = vi.hoisted(() => ({ permissions: new Set<string>() }));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    demoMode: false,
    hasPermission: (resource: string, operation: string) =>
      auth.permissions.has(`${resource}:${operation}`),
    canWrite: (resource: string) => auth.permissions.has(`${resource}:write`),
  }),
}));

const book = {
  id: 1,
  title: "Test Book",
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
  library_id: 1,
  authors: [],
  book_series: [],
  files: [
    {
      id: 10,
      file_role: FileRoleMain,
      file_type: "epub",
      reviewed: true,
    },
  ],
} as unknown as Book;

const renderDialog = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <BookEditDialog book={book} onOpenChange={vi.fn()} open={true} />
    </QueryClientProvider>,
  );
};

const requestedPaths = (request: ReturnType<typeof vi.spyOn>) =>
  request.mock.calls.map((call: unknown[]) => call[1]);

const flush = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 250));
  });

describe("BookEditDialog without People or Series Read", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    auth.permissions = new Set(["books:read", "books:write"]);
  });

  it("requests no people or series while typing, and still offers free text", async () => {
    const request = vi
      .spyOn(API, "request")
      .mockResolvedValue({ items: [], total: 0 });
    const user = userEvent.setup();
    renderDialog();

    const authorCombobox = screen
      .getAllByRole("combobox")
      .find((element) => element.textContent === "Add author");
    await user.click(authorCombobox as HTMLElement);
    await user.type(screen.getByPlaceholderText("Search author..."), "Le Guin");
    await flush();

    expect(screen.getByText(/Create new author "Le Guin"/)).toBeInTheDocument();
    expect(requestedPaths(request)).not.toContain("/people");
    expect(requestedPaths(request)).not.toContain("/series");
  });

  it("requests people and series for a role holding their Read permissions", async () => {
    auth.permissions = new Set([
      "books:read",
      "books:write",
      "people:read",
      "series:read",
    ]);
    const request = vi
      .spyOn(API, "request")
      .mockResolvedValue({ items: [], total: 0 });
    renderDialog();

    await flush();

    expect(requestedPaths(request)).toContain("/people");
    expect(requestedPaths(request)).toContain("/series");
  });
});
