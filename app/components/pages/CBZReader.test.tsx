import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { CBZPageKey, FileTypeCBZ, FileTypePDF, type File } from "@/types";

import CBZReader from "./CBZReader";
import PDFReader from "./PDFReader";

const pageReaderProps = vi.hoisted(() => ({
  current: undefined as undefined | { getPageUrl: (page: number) => string },
}));

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/components/pages/PageReader", () => ({
  default: (props: { getPageUrl: (page: number) => string }) => {
    pageReaderProps.current = props;
    return null;
  },
}));

const file = {
  id: 42,
  book_id: 7,
  page_count: 10,
  updated_at: "2024-06-01T00:00:00Z",
} as File;

describe("page readers", () => {
  it("CBZReader requests pages with the file's cache key and the page key", () => {
    render(
      <CBZReader file={{ ...file, file_type: FileTypeCBZ }} libraryId="1" />,
    );
    expect(pageReaderProps.current?.getPageUrl(3)).toBe(
      `/api/books/files/42/page/3?v=1717200000000&r=${CBZPageKey}`,
    );
  });

  it("PDFReader requests pages with the file's cache key and the render key", () => {
    render(
      <PDFReader file={{ ...file, file_type: FileTypePDF }} libraryId="1" />,
    );
    expect(pageReaderProps.current?.getPageUrl(3)).toBe(
      "/api/books/files/42/page/3?v=1717200000000&r=200-85",
    );
  });
});
