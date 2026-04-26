import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeAll, describe, expect, it, vi } from "vitest";

import { FileRoleMain, FileRoleSupplement, type Book } from "@/types";

import BookItem from "./BookItem";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

// Mock mutation hooks — they require a running API
vi.mock("@/hooks/queries/books", () => ({
  useDeleteBook: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useResyncBook: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

function wrap(ui: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>
  );
}

function makeBook(overrides: Partial<Book> = {}): Book {
  return {
    id: 1,
    title: "Test Book",
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    library_id: 1,
    ...overrides,
  } as Book;
}

describe("BookItem — Needs review badge", () => {
  it("shows badge when a main file has reviewed=false", () => {
    const book = makeBook({
      files: [
        {
          id: 10,
          file_role: FileRoleMain,
          file_type: "epub",
          reviewed: false,
        } as never,
      ],
    });
    render(wrap(<BookItem book={book} libraryId="1" />));
    expect(screen.getByText("Needs review")).toBeInTheDocument();
  });

  it("hides badge when all main files are reviewed", () => {
    const book = makeBook({
      files: [
        {
          id: 10,
          file_role: FileRoleMain,
          file_type: "epub",
          reviewed: true,
        } as never,
      ],
    });
    render(wrap(<BookItem book={book} libraryId="1" />));
    expect(screen.queryByText("Needs review")).not.toBeInTheDocument();
  });

  it("hides badge when there are no main files", () => {
    const book = makeBook({
      files: [
        {
          id: 10,
          file_role: FileRoleSupplement,
          file_type: "epub",
          reviewed: false,
        } as never,
      ],
    });
    render(wrap(<BookItem book={book} libraryId="1" />));
    expect(screen.queryByText("Needs review")).not.toBeInTheDocument();
  });

  it("hides badge when book has no files", () => {
    const book = makeBook({ files: [] });
    render(wrap(<BookItem book={book} libraryId="1" />));
    expect(screen.queryByText("Needs review")).not.toBeInTheDocument();
  });
});
