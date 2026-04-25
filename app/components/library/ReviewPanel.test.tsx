import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
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
  file_type: FileTypeEPUB,
  file_role: FileRoleMain,
  filesize_bytes: 1000,
  reviewed: true,
  review_override: undefined,
  review_overridden_at: undefined,
  cover_image_filename: "book.epub.cover.jpg",
};

describe("ReviewPanel", () => {
  it("renders Reviewed toggle ON when all main files are reviewed", () => {
    const files: File[] = [{ ...baseFile, reviewed: true }];

    render(<ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />, {
      wrapper,
    });

    const toggle = screen.getByRole("switch");
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

    const toggle = screen.getByRole("switch");
    expect(toggle).not.toBeChecked();
  });

  it("renders status icon in green when book is reviewed", () => {
    const files: File[] = [
      { ...baseFile, reviewed: true, review_override: undefined },
    ];

    render(<ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />, {
      wrapper,
    });

    const icon = screen.getByLabelText("Reviewed status");
    expect(icon.getAttribute("class")).toMatch(/text-green/);
  });

  it("renders status icon in muted color when book needs review", () => {
    const files: File[] = [
      { ...baseFile, reviewed: false, review_override: undefined },
    ];

    render(<ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />, {
      wrapper,
    });

    const icon = screen.getByLabelText("Needs review status");
    expect(icon.getAttribute("class")).toMatch(/text-muted-foreground/);
  });

  it("status icon has tooltip describing auto behavior when no overrides are set", async () => {
    const files: File[] = [
      { ...baseFile, reviewed: true, review_override: undefined },
    ];

    render(<ReviewPanel book={baseBook} files={files} onChange={vi.fn()} />, {
      wrapper,
    });

    const icon = screen.getByLabelText("Reviewed status");
    const user = createUser();
    await user.hover(icon);
    const tip = await screen.findAllByText(/automatically from the required/);
    expect(tip.length).toBeGreaterThan(0);
  });

  it("status icon has tooltip with date when all files have reviewed override", async () => {
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

    const icon = screen.getByLabelText("Reviewed status");
    const user = createUser();
    await user.hover(icon);
    const tip = await screen.findAllByText(/Manually marked reviewed on/);
    expect(tip.length).toBeGreaterThan(0);
  });

  it("status icon has tooltip with date when all files have unreviewed override", async () => {
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

    const icon = screen.getByLabelText("Needs review status");
    const user = createUser();
    await user.hover(icon);
    const tip = await screen.findAllByText(/Manually marked needs review on/);
    expect(tip.length).toBeGreaterThan(0);
  });

  it("status icon has tooltip indicating mixed state when overrides differ across files", async () => {
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

    const icon = screen.getByLabelText("Needs review status");
    const user = createUser();
    await user.hover(icon);
    const tip = await screen.findAllByText("Manually set on multiple files");
    expect(tip.length).toBeGreaterThan(0);
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

    const toggle = screen.getByRole("switch");
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

    const toggle = screen.getByRole("switch");
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
});
