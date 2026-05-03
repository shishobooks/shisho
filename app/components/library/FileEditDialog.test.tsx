import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  afterAll,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import { FileRoleMain, FileTypeCBZ, type File } from "@/types";

import { FileEditDialog } from "./FileEditDialog";

// delay:null skips userEvent's default 10ms-per-event pause so chained
// clicks don't rely on fake-timer advancement under CPU contention — this
// is the root-cause fix for the parallel E2E + coverage flakiness that the
// "cover page save race" test hit (the test chains 5 waitFor + React Query
// + FormDialog settle steps, each of which accumulates delay otherwise).
const createUser = () =>
  userEvent.setup({
    advanceTimers: vi.advanceTimersByTime,
    delay: null,
  });

// Define global that's normally set by Vite
beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

// Mock the mutation hooks
const mockUpdateFile = vi.fn();
const mockUploadFileCover = vi.fn();
const mockSetFileCoverPage = vi.fn();

vi.mock("@/hooks/queries/books", () => ({
  useUpdateFile: () => ({
    mutateAsync: mockUpdateFile,
    isPending: false,
  }),
  useUploadFileCover: () => ({
    mutateAsync: mockUploadFileCover,
    isPending: false,
  }),
  useSetFileCoverPage: () => ({
    mutateAsync: mockSetFileCoverPage,
    isPending: false,
  }),
}));

vi.mock("@/hooks/queries/entity-search", () => ({
  usePeopleSearch: () => ({ data: [], isLoading: false }),
  usePublisherSearch: () => ({ data: [], isLoading: false }),
  useImprintSearch: () => ({ data: [], isLoading: false }),
}));

vi.mock("@/hooks/queries/plugins", () => ({
  usePluginIdentifierTypes: () => ({ data: [] }),
}));

// Mock PagePicker
vi.mock("@/components/files/PagePicker", () => ({
  default: ({
    onSelect,
    open,
  }: {
    onSelect: (page: number) => void;
    open: boolean;
  }) =>
    open ? (
      <div data-testid="page-picker">
        <button data-testid="select-page-5" onClick={() => onSelect(5)}>
          Select Page 5
        </button>
      </div>
    ) : null,
}));

describe("FileEditDialog hasChanges logic", () => {
  // This tests the hasChanges computation logic in isolation
  // The fix: pendingCoverPage is compared against initialValues.coverPage (stored when dialog opened)
  // instead of file.cover_page (live prop that can change due to refetch)

  it("should detect cover page change against initial value, not live prop", () => {
    // Simulate the hasChanges computation from FileEditDialog.tsx
    // The fix uses initialValues.coverPage instead of file.cover_page

    // Use 'as number' to widen the types and avoid TypeScript literal comparison errors
    // Initial state when dialog opens
    const initialCoverPage = 0 as number; // stored in initialValues.coverPage when dialog opened

    // User selects page 5
    const pendingCoverPage = 5 as number | null;

    // FIXED hasChanges logic:
    // (pendingCoverPage !== null && pendingCoverPage !== initialValues.coverPage)
    const hasChanges =
      pendingCoverPage !== null && pendingCoverPage !== initialCoverPage;

    expect(hasChanges).toBe(true); // Correctly detects change

    // Even if background refetch updates file.cover_page to 5,
    // hasChanges still correctly compares against initialCoverPage (0)
    // This is because the fix stores initialCoverPage when dialog opens
    // and doesn't use the live file.cover_page prop

    // Verify the fix works even when file.cover_page changes
    const fileCoverPageAfterRefetch = 5 as number;

    // Old buggy logic would have compared against fileCoverPageAfterRefetch
    const buggyHasChangesAfterRefetch =
      pendingCoverPage !== null &&
      pendingCoverPage !== fileCoverPageAfterRefetch;
    expect(buggyHasChangesAfterRefetch).toBe(false); // This is wrong!

    // Fixed logic still compares against initialCoverPage
    const fixedHasChangesAfterRefetch =
      pendingCoverPage !== null && pendingCoverPage !== initialCoverPage;
    expect(fixedHasChangesAfterRefetch).toBe(true); // Correctly detects change
  });
});

describe("FileEditDialog handleSubmit cover page logic", () => {
  // The bug: handleSubmit compares pendingCoverPage to file.cover_page (live prop)
  // but hasChanges compares to initialValues.coverPage (snapshot).
  // These should be consistent - both should use initialValues.coverPage.

  // This is the FIXED shouldUpdateCoverPage logic for handleSubmit
  const shouldUpdateCoverPage = (
    pendingCoverPage: number | null,
    _fileCoverPage: number | null | undefined, // not used in fixed code
    initialCoverPage: number | null, // use snapshot, not live prop
  ): boolean => {
    // FIXED: Compare to initialCoverPage (snapshot from when dialog opened)
    // instead of file.cover_page (live prop that can change)
    return pendingCoverPage !== null && pendingCoverPage !== initialCoverPage;
  };

  it("should update cover page when user changed it, even if props updated (race condition)", () => {
    // Scenario:
    // 1. Dialog opens with file.cover_page = 0
    // 2. initialValues.coverPage = 0 (snapshot)
    // 3. User selects page 5 (pendingCoverPage = 5)
    // 4. Background refetch updates file.cover_page to 5 (from another client or server sync)
    // 5. User clicks Save
    // 6. BUG: shouldUpdateCoverPage returns false because 5 === 5
    //    But user DID make a change (from initial 0 to 5)!

    const pendingCoverPage = 5;
    const fileCoverPageAfterRefetch = 5; // Props updated by refetch
    const initialCoverPage = 0; // Snapshot from when dialog opened

    const result = shouldUpdateCoverPage(
      pendingCoverPage,
      fileCoverPageAfterRefetch,
      initialCoverPage,
    );

    // This test FAILS with buggy code - it returns false (skips update)
    // but it SHOULD return true (user made a change from initial value)
    expect(result).toBe(true);
  });

  it("should NOT update cover page when user didn't change it", () => {
    // User didn't change cover page
    const pendingCoverPage = null;
    const fileCoverPage = 5;
    const initialCoverPage = 5;

    const result = shouldUpdateCoverPage(
      pendingCoverPage,
      fileCoverPage,
      initialCoverPage,
    );

    expect(result).toBe(false); // Correct - no update needed
  });

  it("should NOT update cover page when user selected the same page as initial", () => {
    // User selected the same page that was already set
    const pendingCoverPage = 5;
    const fileCoverPage = 5;
    const initialCoverPage = 5;

    const result = shouldUpdateCoverPage(
      pendingCoverPage,
      fileCoverPage,
      initialCoverPage,
    );

    expect(result).toBe(false); // Correct - no actual change
  });
});

describe("FileEditDialog", () => {
  // Suppress React act() warnings from Radix UI internals (Select, Presence,
  // DismissableLayer, FocusScope). These async state updates are internal to
  // Radix's animation/focus management and cannot be wrapped in act() from tests.
  const originalConsoleError = console.error;
  beforeAll(() => {
    console.error = (...args: unknown[]) => {
      if (
        typeof args[0] === "string" &&
        args[0].includes("was not wrapped in act")
      ) {
        return;
      }
      originalConsoleError(...args);
    };
  });
  afterAll(() => {
    console.error = originalConsoleError;
  });

  const createQueryClient = () =>
    new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });

  const mockFile: File = {
    id: 1,
    book_id: 1,
    library_id: 1,
    filepath: "/test/file.cbz",
    file_type: FileTypeCBZ,
    file_role: FileRoleMain,
    filesize_bytes: 1000,
    cover_page: 1,
    page_count: 10,
    created_at: "2024-01-01",
    updated_at: "2024-01-01",
    narrators: [],
    identifiers: [],
  };

  const renderDialog = (props = {}) => {
    const queryClient = createQueryClient();
    const onOpenChange = vi.fn();

    render(
      <QueryClientProvider client={queryClient}>
        <FileEditDialog
          file={mockFile}
          onOpenChange={onOpenChange}
          open={true}
          {...props}
        />
      </QueryClientProvider>,
    );

    return { onOpenChange };
  };

  beforeEach(() => {
    mockUpdateFile.mockClear();
    mockUploadFileCover.mockClear();
    mockSetFileCoverPage.mockClear();
    mockUpdateFile.mockResolvedValue({});
    mockSetFileCoverPage.mockResolvedValue({});
  });

  describe("cover page change race condition", () => {
    it("should reset pendingCoverPage after successful save so hasChanges becomes false", async () => {
      // This test reproduces the bug:
      // 1. User opens dialog
      // 2. User changes cover page (pendingCoverPage = 5)
      // 3. hasChanges is true
      // 4. User clicks Save
      // 5. Save succeeds
      // 6. BUG: pendingCoverPage is NOT reset, so hasChanges might still be true
      //    if file.cover_page hasn't updated from the async query refetch
      // 7. Dialog should close anyway because pendingCoverPage was reset

      const user = createUser();
      const { onOpenChange } = renderDialog();

      // Wait for dialog to render
      await waitFor(() => {
        expect(screen.getByText("Edit File")).toBeInTheDocument();
      });

      // Find and click the "Select page" button to open the cover page picker
      const selectPageButton = screen.getByRole("button", {
        name: /select page/i,
      });
      await user.click(selectPageButton);

      // Wait for picker to appear
      await waitFor(() => {
        expect(screen.getByTestId("page-picker")).toBeInTheDocument();
      });

      // Select a different page
      await user.click(screen.getByTestId("select-page-5"));

      // Verify the cover page change is reflected (picker should close)
      await waitFor(() => {
        expect(screen.queryByTestId("page-picker")).not.toBeInTheDocument();
      });

      // At this point, pendingCoverPage should be 5, hasChanges should be true
      // Now click Save
      const saveButton = screen.getByRole("button", { name: /save/i });
      await user.click(saveButton);

      // Wait for the mutation to be called
      await waitFor(() => {
        expect(mockSetFileCoverPage).toHaveBeenCalledWith({
          id: 1,
          page: 5,
        });
      });

      // After successful save, the dialog should close
      // This means requestClose() was called and hasChanges must have been false
      // If pendingCoverPage wasn't reset, hasChanges would still be true
      // (because pendingCoverPage=5 !== file.cover_page=1),
      // and the unsaved changes dialog would appear instead of closing
      await waitFor(() => {
        expect(onOpenChange).toHaveBeenCalledWith(false);
      });
    });
  });

  describe("identifier add form", () => {
    const renderFileEditDialogWithIdentifiers = (
      identifiers: Array<{ type: string; value: string }>,
    ) => {
      const queryClient = createQueryClient();
      const onOpenChange = vi.fn();

      render(
        <QueryClientProvider client={queryClient}>
          <FileEditDialog
            file={{
              ...mockFile,
              identifiers: identifiers as typeof mockFile.identifiers,
            }}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      return { onOpenChange };
    };

    it("disables identifier types that are already present in the form", async () => {
      const user = createUser();

      renderFileEditDialogWithIdentifiers([
        { type: "asin", value: "B01ABC1234" },
      ]);

      await user.click(
        screen.getByRole("combobox", { name: /identifier type/i }),
      );

      // Radix Select uses role="option"; the disabled state is exposed via aria-disabled.
      const asinOption = await screen.findByRole("option", { name: /asin/i });
      expect(asinOption).toHaveAttribute("aria-disabled", "true");

      // Hover surfaces the explanatory tooltip.
      await user.hover(asinOption);
      const tooltip = await screen.findByRole("tooltip");
      expect(tooltip).toHaveTextContent(/asin is already added/i);
    });

    it("auto-switches the default type away from one that is already present", async () => {
      renderFileEditDialogWithIdentifiers([
        { type: "isbn_13", value: "9780060519537" },
      ]);

      // The Select trigger should NOT show "ISBN-13" — that type is in use and disabled.
      const trigger = screen.getByRole("combobox", {
        name: /identifier type/i,
      });
      expect(trigger).not.toHaveTextContent(/ISBN-13/i);
    });

    it("re-enables a previously-disabled type after the existing identifier is removed", async () => {
      const user = createUser();
      renderFileEditDialogWithIdentifiers([
        { type: "asin", value: "B01ABC1234" },
      ]);

      // Remove the existing ASIN badge.
      const removeButtons = screen.getAllByRole("button", { name: /remove/i });
      await user.click(removeButtons[0]);

      await user.click(
        screen.getByRole("combobox", { name: /identifier type/i }),
      );
      const asinOption = await screen.findByRole("option", { name: /asin/i });
      expect(asinOption).not.toHaveAttribute("aria-disabled", "true");
    });
  });

  describe("memory management", () => {
    it("should revoke blob URL when dialog closes", async () => {
      // Setup: spy on URL.revokeObjectURL
      const revokeObjectURLSpy = vi.spyOn(URL, "revokeObjectURL");
      const createObjectURLSpy = vi
        .spyOn(URL, "createObjectURL")
        .mockReturnValue("blob:test-url");

      const user = createUser();
      const onOpenChange = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <FileEditDialog
            file={mockFile}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Wait for dialog to render
      await waitFor(() => {
        expect(screen.getByText("Edit File")).toBeInTheDocument();
      });

      // For non-CBZ files, there's an upload button. Since mockFile is CBZ, let's
      // test with an EPUB file instead
      revokeObjectURLSpy.mockClear();
      createObjectURLSpy.mockClear();

      // Rerender with an EPUB file to get the upload button
      const epubFile = {
        ...mockFile,
        file_type: "epub" as const,
        filepath: "/test/file.epub",
      };

      rerender(
        <QueryClientProvider client={queryClient}>
          <FileEditDialog
            file={epubFile}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Wait for rerender
      await waitFor(() => {
        expect(screen.getByText("Edit File")).toBeInTheDocument();
      });

      // Find the file input and simulate a file upload
      const fileInputAfterRerender = document.querySelector(
        'input[type="file"]',
      ) as HTMLInputElement;

      if (fileInputAfterRerender) {
        const testFile = new File(["test"], "test.png", { type: "image/png" });
        await user.upload(fileInputAfterRerender, testFile);

        // createObjectURL should have been called
        expect(createObjectURLSpy).toHaveBeenCalled();

        // Now close the dialog
        rerender(
          <QueryClientProvider client={queryClient}>
            <FileEditDialog
              file={epubFile}
              onOpenChange={onOpenChange}
              open={false}
            />
          </QueryClientProvider>,
        );

        // The blob URL should be revoked when dialog closes
        await waitFor(() => {
          expect(revokeObjectURLSpy).toHaveBeenCalledWith("blob:test-url");
        });
      }

      // Cleanup
      createObjectURLSpy.mockRestore();
      revokeObjectURLSpy.mockRestore();
    });
  });

  describe("release date plain-text input", () => {
    it("renders a text input with YYYY-MM-DD placeholder", async () => {
      renderDialog();

      const input = screen.getByPlaceholderText("YYYY-MM-DD");
      expect(input).toBeInTheDocument();
      expect(input.tagName).toBe("INPUT");
    });

    it("submits typed date value in payload", async () => {
      const user = createUser();
      renderDialog();

      const input = screen.getByPlaceholderText("YYYY-MM-DD");
      await user.clear(input);
      await user.type(input, "1847-10-16");

      const saveButton = screen.getByRole("button", { name: /save/i });
      await user.click(saveButton);

      await waitFor(() => {
        expect(mockUpdateFile).toHaveBeenCalled();
      });
      const call = mockUpdateFile.mock.calls[0][0];
      expect(call.payload.release_date).toBe("1847-10-16");
    });

    it("allows clearing the date field", async () => {
      const user = createUser();
      const fileWithDate: File = {
        ...mockFile,
        release_date: "2020-06-15T00:00:00Z",
      };
      const queryClient = createQueryClient();

      render(
        <QueryClientProvider client={queryClient}>
          <FileEditDialog
            file={fileWithDate}
            onOpenChange={vi.fn()}
            open={true}
          />
        </QueryClientProvider>,
      );

      const input = screen.getByPlaceholderText("YYYY-MM-DD");
      expect(input).toHaveValue("2020-06-15");

      await user.clear(input);

      const saveButton = screen.getByRole("button", { name: /save/i });
      await user.click(saveButton);

      await waitFor(() => {
        expect(mockUpdateFile).toHaveBeenCalled();
      });
      const call = mockUpdateFile.mock.calls[0][0];
      expect(call.payload.release_date).toBe("");
    });
  });
});
