import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import { type File } from "@/types";

import FileDetailsTab from "./FileDetailsTab";

vi.mock("@/hooks/queries/plugins", () => ({
  usePluginIdentifierTypes: () => ({ data: undefined }),
}));

function wrap(ui: React.ReactNode, libraryId = "1") {
  return (
    <MemoryRouter
      initialEntries={[`/libraries/${libraryId}/books/10/files/100`]}
    >
      <Routes>
        <Route
          element={ui}
          path="/libraries/:libraryId/books/:bookId/files/:fileId"
        />
      </Routes>
    </MemoryRouter>
  );
}

function makeFile(overrides: Partial<File> = {}): File {
  return {
    id: 100,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    book_id: 10,
    filepath: "/library/book/file.epub",
    file_type: "epub",
    file_role: "main",
    filesize_bytes: 1024,
    ...overrides,
  } as File;
}

describe("FileDetailsTab", () => {
  describe("publisher link", () => {
    it("renders publisher name as a link to the publisher detail page", () => {
      const file = makeFile({
        publisher_id: 42,
        publisher: {
          id: 42,
          created_at: "2024-01-01T00:00:00Z",
          updated_at: "2024-01-01T00:00:00Z",
          library_id: 1,
          name: "Penguin Books",
          aliases: [],
          file_count: 5,
        },
      });

      render(wrap(<FileDetailsTab file={file} />, "1"));

      const link = screen.getByRole("link", { name: "Penguin Books" });
      expect(link).toBeInTheDocument();
      expect(link).toHaveAttribute("href", "/libraries/1/publishers/42");
    });

    it("does not render publisher section when file has no publisher", () => {
      const file = makeFile();

      render(wrap(<FileDetailsTab file={file} />));

      expect(screen.queryByText("Publisher")).not.toBeInTheDocument();
    });
  });
});
