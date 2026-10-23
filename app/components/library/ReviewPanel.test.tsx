import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, describe, expect, it, vi } from "vitest";

import {
  FileRoleMain,
  FileTypeCBZ,
  FileTypeEPUB,
  FileTypeM4B,
  ReviewOverrideReviewed,
  ReviewOverrideUnreviewed,
  type Book,
  type File,
} from "@/types";

import { ReviewPanel } from "./ReviewPanel";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

// Mock review criteria hook
const mockCriteria = {
  book_fields: ["authors", "description", "cover", "genres"],
  audio_fields: ["narrators"],
  universal_candidates: [],
  audio_candidates: [],
  override_count: 0,
  main_file_count: 0,
};

vi.mock("@/hooks/queries/review", () => ({
  useReviewCriteria: () => ({ data: mockCriteria }),
  useSetFileReview: () => ({ mutate: vi.fn(), isPending: false }),
  useSetBookReview: () => ({ mutate: vi.fn(), isPending: false }),
}));

const createUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime, delay: null });

function wrapper({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

// Minimal Book fixture
const baseBook: Book = {
  id: 1,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
  library_id: 1,
  filepath: "/library/book",
  title: "Test Book",
  title_source: "manual",
  sort_title: "Test Book",
  sort_title_source: "manual",
  author_source: "manual",
  cover_cache_key: "",
  files: [],
  authors: [
    { id: 1, book_id: 1, person_id: 1, sort_order: 0, role: undefined },
  ],
  description: "A great book",
  book_genres: [{ id: 1, book_id: 1, genre_id: 1 }],
};

// Minimal File fixture (main, EPUB, reviewed)
const baseFile: File = {
  id: 10,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
  library_id: 1,
  book_id: 1,
  filepath: "/library/book/book.epub",
  display_name: "book.epub",
  file_type: FileTypeEPUB,
  file_role: FileRoleMain,
  filesize_bytes: 1000,
  reviewed: true,
  review_override: undefined,
  review_overridden_at: undefined,
  cover_image_filename: "book.epub.cover.jpg",
  is_preferred_cover: false,
  reachable_file_types: [],
};

describe("ReviewPanel", () => {
  it("groups the checkbox under its Review label", () => {
    const files: File[] = [{ ...baseFile, reviewed: true }];

    render(<ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />, {
      wrapper,
    });

    const group = screen.getByRole("group", { name: "Review" });
    expect(
      within(group).getByRole("checkbox", { name: "Reviewed" }),
    ).toBeInTheDocument();
  });

  it("renders Reviewed toggle ON when all main files are reviewed", () => {
    const files: File[] = [{ ...baseFile, reviewed: true }];

    render(<ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />, {
      wrapper,
    });

    const toggle = screen.getByRole("checkbox", { name: "Reviewed" });
    expect(toggle).toBeChecked();
  });

  it("renders Reviewed toggle OFF when any main file is not reviewed", () => {
    const files: File[] = [
      { ...baseFile, id: 10, reviewed: true },
      {
        ...baseFile,
        id: 11,
        file_type: FileTypeM4B,
        reviewed: false,
        cover_image_filename: "book.m4b.cover.jpg",
      },
    ];

    render(<ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />, {
      wrapper,
    });

    const toggle = screen.getByRole("checkbox", { name: "Reviewed" });
    expect(toggle).not.toBeChecked();
  });

  it("names each checkbox when two panels are on the page", () => {
    const files: File[] = [{ ...baseFile, reviewed: true }];

    render(
      <>
        <ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />
        <ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />
      </>,
      { wrapper },
    );

    const boxes = screen.getAllByRole("checkbox", { name: "Reviewed" });
    expect(boxes).toHaveLength(2);
    expect(boxes[0].id).not.toBe(boxes[1].id);
  });

  it("says the state is set automatically when no overrides are set", () => {
    const files: File[] = [
      { ...baseFile, reviewed: true, review_override: undefined },
    ];

    render(<ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />, {
      wrapper,
    });

    // Said under the checkbox, not in a tooltip, so it reads on a phone too.
    expect(
      screen.getByRole("checkbox", { name: "Reviewed" }),
    ).toHaveAccessibleDescription(/Set automatically from the review criteria/);
  });

  it("says when every file was marked reviewed by hand", () => {
    const files: File[] = [
      {
        ...baseFile,
        reviewed: true,
        review_override: ReviewOverrideReviewed,
        review_overridden_at: "2024-06-15T10:00:00Z",
      },
    ];

    render(<ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />, {
      wrapper,
    });

    expect(
      screen.getByText(/Marked reviewed by hand on Jun 15, 2024/),
    ).toBeInTheDocument();
  });

  it("says when every file was marked as needing review by hand", () => {
    const files: File[] = [
      {
        ...baseFile,
        reviewed: false,
        review_override: ReviewOverrideUnreviewed,
        review_overridden_at: "2024-06-15T10:00:00Z",
      },
    ];

    render(<ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />, {
      wrapper,
    });

    expect(
      screen.getByText(/Marked as needing review by hand on Jun 15, 2024/),
    ).toBeInTheDocument();
  });

  it("says when overrides differ across files", () => {
    const files: File[] = [
      {
        ...baseFile,
        id: 10,
        reviewed: true,
        review_override: ReviewOverrideReviewed,
        review_overridden_at: "2024-06-15T10:00:00Z",
      },
      {
        ...baseFile,
        id: 11,
        file_type: FileTypeCBZ,
        reviewed: false,
        review_override: ReviewOverrideUnreviewed,
        review_overridden_at: "2024-06-10T10:00:00Z",
        cover_image_filename: "book.cbz.cover.jpg",
      },
    ];

    render(<ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />, {
      wrapper,
    });

    expect(screen.getByText(/Set by hand on some files/)).toBeInTheDocument();
  });

  it("shows missing fields list when book needs review", () => {
    const bookWithoutDescription: Book = {
      ...baseBook,
      description: undefined,
    };

    const files: File[] = [
      {
        ...baseFile,
        reviewed: false,
        review_override: undefined,
        cover_image_filename: undefined,
        cover_mime_type: undefined,
      },
    ];

    render(
      <ReviewPanel
        book={bookWithoutDescription}
        files={files}
        onChange={vi.fn()}
      />,
      { wrapper },
    );

    // Should show missing fields: description and cover (from criteria: authors, description, cover, genres)
    // Authors are present in baseBook, genres are present, so only description and cover missing
    expect(screen.getByText(/Missing:/)).toBeInTheDocument();
    expect(screen.getByText(/description/)).toBeInTheDocument();
    expect(screen.getByText(/cover/)).toBeInTheDocument();
  });

  it("shows missing fields aggregated across files without qualifier when missing on all", () => {
    const bookNoDescription: Book = {
      ...baseBook,
      description: undefined,
    };

    // Two files both missing description (book-level) and cover (file-level)
    const files: File[] = [
      {
        ...baseFile,
        id: 10,
        file_type: FileTypeEPUB,
        reviewed: false,
        cover_image_filename: undefined,
        cover_mime_type: undefined,
      },
      {
        ...baseFile,
        id: 11,
        file_type: FileTypeCBZ,
        reviewed: false,
        cover_image_filename: undefined,
        cover_mime_type: undefined,
      },
    ];

    render(
      <ReviewPanel book={bookNoDescription} files={files} onChange={vi.fn()} />,
      { wrapper },
    );

    // description is book-level → missing on both → listed once without qualifier
    // cover is file-level → missing on both → listed once without qualifier
    const hint = screen.getByText(/Missing:/);
    expect(hint.textContent).not.toMatch(/EPUB/);
    expect(hint.textContent).not.toMatch(/CBZ/);
    expect(hint.textContent).toMatch(/description/);
    expect(hint.textContent).toMatch(/cover/);
  });

  it("qualifies missing fields with file type when missing on only some files", () => {
    // EPUB is missing cover, M4B has cover but missing narrators
    const files: File[] = [
      {
        ...baseFile,
        id: 10,
        file_type: FileTypeEPUB,
        reviewed: false,
        cover_image_filename: undefined,
        cover_mime_type: undefined,
      },
      {
        ...baseFile,
        id: 11,
        file_type: FileTypeM4B,
        reviewed: false,
        cover_image_filename: "book.m4b.cover.jpg",
        narrators: [],
      },
    ];

    render(<ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />, {
      wrapper,
    });

    // cover is only missing on EPUB → should be qualified
    // narrators is only missing on M4B → should be qualified
    const hint = screen.getByText(/Missing:/);
    expect(hint.textContent).toMatch(/cover.*EPUB|EPUB.*cover/);
    expect(hint.textContent).toMatch(/narrators.*M4B|M4B.*narrators/);
  });

  it("names files that share a type instead of repeating the type", () => {
    // Two audiobooks miss a cover and the EPUB has one, so "cover (M4B)"
    // twice would not say which audiobooks.
    const files: File[] = [
      {
        ...baseFile,
        id: 10,
        file_type: FileTypeEPUB,
        reviewed: false,
        cover_image_filename: "book.epub.cover.jpg",
      },
      {
        ...baseFile,
        id: 11,
        file_type: FileTypeM4B,
        display_name: "Read by Fry",
        reviewed: false,
        cover_image_filename: undefined,
        cover_mime_type: undefined,
        narrators: [{ id: 1 } as never],
      },
      {
        ...baseFile,
        id: 12,
        file_type: FileTypeM4B,
        display_name: "Read by Dale",
        reviewed: false,
        cover_image_filename: undefined,
        cover_mime_type: undefined,
        narrators: [{ id: 2 } as never],
      },
    ];

    render(<ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />, {
      wrapper,
    });

    const hint = screen.getByText(/Missing:/);
    expect(hint).toHaveTextContent("cover (Read by Fry, Read by Dale)");
    expect(hint.textContent).not.toMatch(/M4B/);
  });

  it("does not show missing fields hint when book is fully reviewed", () => {
    const files: File[] = [{ ...baseFile, reviewed: true }];

    render(<ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />, {
      wrapper,
    });

    expect(screen.queryByText(/Missing:/)).not.toBeInTheDocument();
  });

  it("toggle click fires onChange with reviewed override when toggling on", async () => {
    const user = createUser();
    const onChange = vi.fn();
    const files: File[] = [{ ...baseFile, reviewed: false }];

    render(<ReviewPanel book={baseBook} files={files} onChange={onChange} />, {
      wrapper,
    });

    const toggle = screen.getByRole("checkbox", { name: "Reviewed" });
    await user.click(toggle);

    expect(onChange).toHaveBeenCalledWith(ReviewOverrideReviewed);
  });

  it("toggle click fires onChange with unreviewed override when toggling off", async () => {
    const user = createUser();
    const onChange = vi.fn();
    const files: File[] = [{ ...baseFile, reviewed: true }];

    render(<ReviewPanel book={baseBook} files={files} onChange={onChange} />, {
      wrapper,
    });

    const toggle = screen.getByRole("checkbox", { name: "Reviewed" });
    await user.click(toggle);

    expect(onChange).toHaveBeenCalledWith(ReviewOverrideUnreviewed);
  });

  it("returns null when there are no main files", () => {
    const { container } = render(
      <ReviewPanel book={baseBook} files={[]} onChange={vi.fn()} />,
      { wrapper },
    );

    expect(container.firstChild).toBeNull();
  });

  describe("controlled mode (toggleValue prop)", () => {
    it("toggle reflects toggleValue=true even when committed reviewed is false", () => {
      const files: File[] = [
        { ...baseFile, reviewed: false, review_override: undefined },
      ];

      render(
        <ReviewPanel
          book={baseBook}
          files={files}
          onChange={vi.fn()}
          toggleValue={true}
        />,
        { wrapper },
      );

      expect(screen.getByRole("checkbox", { name: "Reviewed" })).toBeChecked();
    });

    it("toggle reflects toggleValue=false even when committed reviewed is true", () => {
      const files: File[] = [
        { ...baseFile, reviewed: true, review_override: undefined },
      ];

      render(
        <ReviewPanel
          book={baseBook}
          files={files}
          onChange={vi.fn()}
          toggleValue={false}
        />,
        { wrapper },
      );

      expect(
        screen.getByRole("checkbox", { name: "Reviewed" }),
      ).not.toBeChecked();
    });

    it("falls back to committed reviewed when toggleValue is undefined", () => {
      const files: File[] = [
        { ...baseFile, reviewed: true, review_override: undefined },
      ];

      render(
        <ReviewPanel
          book={baseBook}
          files={files}
          onChange={vi.fn()}
          toggleValue={undefined}
        />,
        { wrapper },
      );

      expect(screen.getByRole("checkbox", { name: "Reviewed" })).toBeChecked();
    });

    it("missing-fields hint follows committed state, not toggleValue", () => {
      const bookWithoutDescription: Book = {
        ...baseBook,
        description: undefined,
      };
      const files: File[] = [
        {
          ...baseFile,
          reviewed: false,
          review_override: undefined,
          cover_image_filename: "x.jpg",
          cover_mime_type: "image/jpeg",
        },
      ];

      // Even with toggleValue=true (drafted as reviewed), the missing-fields
      // hint anchors to the saved data and still reports description missing.
      render(
        <ReviewPanel
          book={bookWithoutDescription}
          files={files}
          onChange={vi.fn()}
          toggleValue={true}
        />,
        { wrapper },
      );

      expect(screen.getByText(/Missing:.*description/)).toBeInTheDocument();
    });
  });
});
