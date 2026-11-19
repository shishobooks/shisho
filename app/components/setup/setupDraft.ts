/**
 * Setup's choices, kept in browser storage until Finish so a reload resumes
 * with them. The step itself is in the URL.
 */
export interface SetupDraft {
  /** The admin who made these choices; another user on the browser starts fresh. */
  userId: number;
  name: string;
  paths: string[];
  organize: boolean;
  /**
   * Keys (`scope/id`) of the plugins ticked on the enrichment step, or null
   * until the admin changes the recommended ticks.
   */
  plugins: string[] | null;
}

const STORAGE_KEY = "shisho:setup";

export const newSetupDraft = (userId: number): SetupDraft => ({
  userId,
  name: "",
  paths: [""],
  organize: true,
  plugins: null,
});

const isDraft = (value: unknown): value is SetupDraft => {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  return (
    typeof v.userId === "number" &&
    typeof v.name === "string" &&
    Array.isArray(v.paths) &&
    v.paths.every((p) => typeof p === "string") &&
    typeof v.organize === "boolean" &&
    (v.plugins === null ||
      (Array.isArray(v.plugins) &&
        v.plugins.every((p) => typeof p === "string")))
  );
};

const readStored = (): SetupDraft | null => {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    return isDraft(parsed) ? parsed : null;
  } catch {
    return null;
  }
};

/** The user's saved draft, if they have one. */
export const storedSetupDraft = (userId: number): SetupDraft | null => {
  const draft = readStored();
  return draft?.userId === userId ? draft : null;
};

/** The user's saved draft, or a fresh one. */
export const loadSetupDraft = (userId: number): SetupDraft =>
  storedSetupDraft(userId) ?? newSetupDraft(userId);

export const saveSetupDraft = (draft: SetupDraft) => {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(draft));
  } catch {
    // Without storage, a reload starts over; Setup still works.
  }
};

/** Forgets the draft, on Finish or "Skip setup". */
export const clearSetupDraft = () => {
  try {
    window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    // Nothing was saved.
  }
};

export interface LibraryStepErrors {
  name?: string;
  paths?: string;
}

/** What's missing before the library step can move on. */
export const libraryStepErrors = (draft: SetupDraft): LibraryStepErrors => {
  const errors: LibraryStepErrors = {};
  if (!draft.name.trim()) errors.name = "Give your library a name.";
  if (!draft.paths.some((p) => p.trim() !== "")) {
    errors.paths = "Choose at least one folder.";
  }
  return errors;
};
