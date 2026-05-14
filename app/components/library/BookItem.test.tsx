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

describe("BookItem — Series number badge", () => {
  const seriesId = 42;

  it("shows badge when series_number is 0", () => {
    const book = makeBook({
      book_series: [
        {
          id: 1,
          book_id: 1,
          series_id: seriesId,
          series_number: 0,
        } as never,
      ],
      files: [
        {
          id: 10,
          file_role: FileRoleMain,
          file_type: "epub",
          reviewed: true,
        } as never,
      ],
    });
    render(wrap(<BookItem book={book} libraryId="1" seriesId={seriesId} />));
    // The badge should be a <span> with specific classes, containing "0"
    const badge = screen.getByText("0", { selector: "span" });
    expect(badge).toBeInTheDocument();
    expect(badge.className).toContain("bg-primary");
  });

  it("shows badge when series_number is a positive number", () => {
    const book = makeBook({
      book_series: [
        {
          id: 1,
          book_id: 1,
          series_id: seriesId,
          series_number: 3,
        } as never,
      ],
      files: [
        {
          id: 10,
          file_role: FileRoleMain,
          file_type: "epub",
          reviewed: true,
        } as never,
      ],
    });
    render(wrap(<BookItem book={book} libraryId="1" seriesId={seriesId} />));
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("hides badge when series_number is null", () => {
    const book = makeBook({
      book_series: [
        {
          id: 1,
          book_id: 1,
          series_id: seriesId,
          series_number: null,
        } as never,
      ],
      files: [
        {
          id: 10,
          file_role: FileRoleMain,
          file_type: "epub",
          reviewed: true,
        } as never,
      ],
    });
    render(wrap(<BookItem book={book} libraryId="1" seriesId={seriesId} />));
    // The formatted output of null is "", so there should be no badge content
    // We check that "0" or any number text isn't rendered as a badge
    const title = screen.getByText("Test Book");
    // The title container should not have leading-[1.6] class
    expect(title.className).not.toContain("leading-[1.6]");
  });

  it("hides badge when series_number is undefined", () => {
    const book = makeBook({
      book_series: [
        {
          id: 1,
          book_id: 1,
          series_id: seriesId,
        } as never,
      ],
      files: [
        {
          id: 10,
          file_role: FileRoleMain,
          file_type: "epub",
          reviewed: true,
        } as never,
      ],
    });
    render(wrap(<BookItem book={book} libraryId="1" seriesId={seriesId} />));
    const title = screen.getByText("Test Book");
    expect(title.className).not.toContain("leading-[1.6]");
  });

  it("applies leading-[1.6] class when series_number is 0", () => {
    const book = makeBook({
      book_series: [
        {
          id: 1,
          book_id: 1,
          series_id: seriesId,
          series_number: 0,
        } as never,
      ],
      files: [
        {
          id: 10,
          file_role: FileRoleMain,
          file_type: "epub",
          reviewed: true,
        } as never,
      ],
    });
    render(wrap(<BookItem book={book} libraryId="1" seriesId={seriesId} />));
    const title = screen.getByText("Test Book");
    // The parent div that has leading-[1.6] is the closest ancestor with that class
    expect(title.closest("div")?.className).toContain("leading-[1.6]");
  });
});

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
