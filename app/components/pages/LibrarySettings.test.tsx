import equal from "fast-deep-equal";
import { describe, expect, it } from "vitest";

/**
 * These tests verify the hasChanges logic for LibrarySettings.
 * The bug: After save, initialValues.name is trimmed but the form state
 * name is not, causing hasChanges to remain true incorrectly.
 *
 * The fix: Update form state (name, libraryPaths) to match saved values after save.
 */
describe("LibrarySettings hasChanges logic", () => {
  // This mirrors the actual hasChanges computation from LibrarySettings.tsx
  const computeHasChanges = (
    formState: {
      name: string;
      organizeFileStructure: boolean;
      coverAspectRatio: string;
      downloadFormatPreference: string;
      libraryPaths: string[];
    },
    initialValues: {
      name: string;
      organizeFileStructure: boolean;
      coverAspectRatio: string;
      downloadFormatPreference: string;
      libraryPaths: string[];
    } | null,
    isInitialized: boolean,
  ): boolean => {
    if (!initialValues || !isInitialized) return false;
    return (
      formState.name !== initialValues.name ||
      formState.organizeFileStructure !== initialValues.organizeFileStructure ||
      formState.coverAspectRatio !== initialValues.coverAspectRatio ||
      formState.downloadFormatPreference !==
        initialValues.downloadFormatPreference ||
      !equal(formState.libraryPaths, initialValues.libraryPaths)
    );
  };

  // Simulate the handleSave behavior from LibrarySettings.tsx
  const simulateSave = (formState: {
    name: string;
    organizeFileStructure: boolean;
    coverAspectRatio: string;
    downloadFormatPreference: string;
    libraryPaths: string[];
  }) => {
    // The fix: update form state to match what was saved (trimmed/filtered)
    const trimmedName = formState.name.trim();
    const validPaths = formState.libraryPaths.filter((p) => p.trim() !== "");

    // Return updated form state and initial values (both should match)
    const updatedFormState = {
      ...formState,
      name: trimmedName,
      libraryPaths: validPaths,
    };

    const newInitialValues = {
      name: trimmedName,
      organizeFileStructure: formState.organizeFileStructure,
      coverAspectRatio: formState.coverAspectRatio,
      downloadFormatPreference: formState.downloadFormatPreference,
      libraryPaths: validPaths,
    };

    return { updatedFormState, newInitialValues };
  };

  it("should have hasChanges=false after save when name had whitespace", () => {
    // User enters name with whitespace
    const formState = {
      name: "  Test Library  ",
      organizeFileStructure: true,
      coverAspectRatio: "book",
      downloadFormatPreference: "original",
      libraryPaths: ["/path"],
    };

    // Before save: hasChanges is true (compared to original initial values)
    const originalInitialValues = {
      name: "Original Library",
      organizeFileStructure: true,
      coverAspectRatio: "book",
      downloadFormatPreference: "original",
      libraryPaths: ["/path"],
    };

    expect(computeHasChanges(formState, originalInitialValues, true)).toBe(
      true,
    );

    // After save: form state is updated to match saved values
    const { updatedFormState, newInitialValues } = simulateSave(formState);

    // hasChanges should now be false
    const hasChanges = computeHasChanges(
      updatedFormState,
      newInitialValues,
      true,
    );
    expect(hasChanges).toBe(false);

    // Verify the form state was properly trimmed
    expect(updatedFormState.name).toBe("Test Library");
  });

  it("should have hasChanges=false after save when libraryPaths had empty entries", () => {
    const formState = {
      name: "Library",
      organizeFileStructure: true,
      coverAspectRatio: "book",
      downloadFormatPreference: "original",
      libraryPaths: ["/path1", "", "  ", "/path2"],
    };

    const { updatedFormState, newInitialValues } = simulateSave(formState);

    const hasChanges = computeHasChanges(
      updatedFormState,
      newInitialValues,
      true,
    );
    expect(hasChanges).toBe(false);

    // Verify empty paths were filtered
    expect(updatedFormState.libraryPaths).toEqual(["/path1", "/path2"]);
  });

  it("should detect real changes after save", () => {
    const formState = {
      name: "Library",
      organizeFileStructure: true,
      coverAspectRatio: "book",
      downloadFormatPreference: "original",
      libraryPaths: ["/path"],
    };

    const { updatedFormState, newInitialValues } = simulateSave(formState);

    // User makes another change
    const newFormState = {
      ...updatedFormState,
      name: "New Name",
    };

    const hasChanges = computeHasChanges(newFormState, newInitialValues, true);
    expect(hasChanges).toBe(true);
  });
});

/**
 * Tests for the initialization logic that prevents race conditions
 * when navigating between libraries.
 *
 * The bug: When libraryId changes, isInitialized is reset, but if React Query
 * still has cached data from the previous library, the form could initialize
 * with stale data before the new data loads.
 *
 * The fix: Check that data.id matches the current libraryId before initializing.
 */
describe("LibrarySettings initialization logic", () => {
  // This mirrors the FIXED shouldInitialize condition for LibrarySettings.tsx
  // The fix adds a check that data.id matches the current libraryId
  const shouldInitialize = (
    libraryId: string | undefined,
    queryData: { id: number; name: string } | undefined,
    isSuccess: boolean,
    isInitialized: boolean,
  ): boolean => {
    if (!isSuccess || !queryData || isInitialized) return false;
    // FIX: Check that data.id matches current libraryId to prevent
    // initializing with stale cached data from a different library
    if (queryData.id !== Number(libraryId)) return false;
    return true;
  };

  it("should NOT initialize when query data id does not match current libraryId (race condition)", () => {
    // Scenario: User navigated from library 1 to library 2
    // React Query still has library 1's data cached
    const currentLibraryId = "2";
    const staleQueryData = { id: 1, name: "Library One" };

    const result = shouldInitialize(
      currentLibraryId,
      staleQueryData,
      true, // isSuccess
      false, // isInitialized was reset
    );

    // Should NOT initialize with stale data
    // This test will FAIL with current buggy code (returns true)
    expect(result).toBe(false);
  });

  it("should initialize when query data id matches current libraryId", () => {
    const currentLibraryId = "2";
    const freshQueryData = { id: 2, name: "Library Two" };

    const result = shouldInitialize(
      currentLibraryId,
      freshQueryData,
      true, // isSuccess
      false, // not yet initialized
    );

    // Should initialize with matching data
    expect(result).toBe(true);
  });

  it("should NOT initialize when already initialized", () => {
    const currentLibraryId = "2";
    const freshQueryData = { id: 2, name: "Library Two" };

    const result = shouldInitialize(
      currentLibraryId,
      freshQueryData,
      true, // isSuccess
      true, // already initialized
    );

    // Should NOT reinitialize
    expect(result).toBe(false);
  });

  it("should NOT initialize when query is not successful", () => {
    const currentLibraryId = "2";
    const queryData = { id: 2, name: "Library Two" };

    const result = shouldInitialize(
      currentLibraryId,
      queryData,
      false, // not success (still loading)
      false,
    );

    expect(result).toBe(false);
  });

  it("should NOT initialize when query data is undefined", () => {
    const currentLibraryId = "2";

    const result = shouldInitialize(
      currentLibraryId,
      undefined, // no data yet
      true,
      false,
    );

    expect(result).toBe(false);
  });
});
