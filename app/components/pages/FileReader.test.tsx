import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import { useBook } from "@/hooks/queries/books";

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
vi.mock("./EPUBReader", () => ({
  default: () => <div>epub-reader</div>,
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

  return render(
    <MemoryRouter initialEntries={["/libraries/1/books/7/files/42/read"]}>
      <Routes>
        <Route
          element={<FileReader />}
          path="/libraries/:libraryId/books/:bookId/files/:fileId/read"
        />
      </Routes>
    </MemoryRouter>,
  );
};

describe("FileReader dispatch", () => {
  it("renders the M4B player for m4b files", () => {
    renderAt("m4b");
    expect(screen.getByText("m4b-player")).toBeInTheDocument();
  });

  it("renders the CBZ reader for cbz files", () => {
    renderAt("cbz");
    expect(screen.getByText("cbz-reader")).toBeInTheDocument();
  });

  it("shows an unsupported message for unknown file types", () => {
    renderAt("txt");
    expect(screen.getByText(/not supported/i)).toBeInTheDocument();
  });
});
