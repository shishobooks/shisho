import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import { useBook } from "@/hooks/queries/books";
import { ShishoAPIError } from "@/libraries/api";

import FileReader from "./FileReader";

vi.mock("@/hooks/queries/books", () => ({
  useBook: vi.fn(),
}));

// Stub the heavy readers so this test only checks dispatch by file_type.
vi.mock("./M4BReader", () => ({
  default: () => <div>m4b-player</div>,
}));
vi.mock("./CBZReader", () => ({
  default: () => <div>cbz-reader</div>,
}));
vi.mock("./PDFReader", () => ({
  default: () => <div>pdf-reader</div>,
}));
vi.mock("./ReflowableReader", () => ({
  default: () => <div>reflowable-reader</div>,
}));

const renderAt = (fileType: string) => {
  vi.mocked(useBook).mockReturnValue({
    data: {
      id: 7,
      title: "Book",
      files: [{ id: 42, book_id: 7, file_type: fileType }],
    },
    isLoading: false,
  } as never);

  return renderReader();
};

const renderReader = () =>
  render(
    <MemoryRouter initialEntries={["/libraries/1/books/7/files/42/read"]}>
      <Routes>
        <Route
          element={<FileReader />}
          path="/libraries/:libraryId/books/:bookId/files/:fileId/read"
        />
      </Routes>
    </MemoryRouter>,
  );

describe("FileReader dispatch", () => {
  it("renders the M4B player for m4b files", () => {
    renderAt("m4b");
    expect(screen.getByText("m4b-player")).toBeInTheDocument();
  });

  it("renders the CBZ reader for cbz files", () => {
    renderAt("cbz");
    expect(screen.getByText("cbz-reader")).toBeInTheDocument();
  });

  it.each(["epub", "azw3", "mobi"])(
    "renders the Reflowable reader for %s files",
    (fileType) => {
      renderAt(fileType);
      expect(screen.getByText("reflowable-reader")).toBeInTheDocument();
    },
  );

  it("shows an unsupported message for unknown file types", () => {
    renderAt("txt");
    expect(screen.getByText(/not supported/i)).toBeInTheDocument();
  });
});

const failedBook = (error: unknown) =>
  vi.mocked(useBook).mockReturnValue({
    data: undefined,
    error,
    isError: true,
    isLoading: false,
    isFetching: false,
    isEnabled: true,
    refetch: vi.fn(),
  } as never);

describe("FileReader load failure", () => {
  it("shows an alert instead of spinning when the book fails to load", () => {
    failedBook(
      new ShishoAPIError("Internal Server Error", "internal_server_error", 500),
    );
    renderReader();

    expect(screen.getByRole("alert")).toHaveTextContent(/^Failed to load book/);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("says the file was not found when the book is gone", () => {
    failedBook(new ShishoAPIError("Book not found", "not_found", 404));
    renderReader();

    expect(screen.getByText("File Not Found")).toBeInTheDocument();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("says the file was not found when the book has no such file", () => {
    vi.mocked(useBook).mockReturnValue({
      data: { id: 7, title: "Book", files: [] },
      error: null,
      isLoading: false,
    } as never);
    renderReader();

    expect(screen.getByText("File Not Found")).toBeInTheDocument();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});
