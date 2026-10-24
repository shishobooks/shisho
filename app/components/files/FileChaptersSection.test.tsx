import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { forwardRef, useImperativeHandle } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { File } from "@/types";

import FileChaptersSection from "./FileChaptersSection";

const editor = vi.hoisted(() => ({
  chapters: [] as unknown[],
  editing: false,
  fetchFromAudible: vi.fn(),
  addChapter: vi.fn(),
  cancel: vi.fn(),
  save: vi.fn(),
}));

vi.mock("@/hooks/queries/chapters", () => ({
  useFileChapters: () => ({ data: editor.chapters }),
}));

// The real editor is covered by FileChaptersEditor.test.tsx; this stand-in
// shows which mode the section asked for and records handle calls.
vi.mock("@/components/files/FileChaptersEditor", () => ({
  default: forwardRef<unknown, { isEditing: boolean }>(function Editor(
    { isEditing },
    ref,
  ) {
    editor.editing = isEditing;
    useImperativeHandle(ref, () => ({
      fetchFromAudible: editor.fetchFromAudible,
      addChapter: editor.addChapter,
      cancel: editor.cancel,
      save: editor.save,
    }));
    return <div>{isEditing ? "editor" : "chapter list"}</div>;
  }),
}));

const file = (overrides: Partial<File> = {}) =>
  ({
    id: 42,
    book_id: 7,
    library_id: 1,
    file_type: "m4b",
    file_role: "main",
    ...overrides,
  }) as File;

const renderSection = (props: { file?: File; canEdit?: boolean } = {}) =>
  render(
    <FileChaptersSection
      canEdit={props.canEdit ?? true}
      file={props.file ?? file()}
      onActionStateChange={vi.fn()}
    />,
  );

const user = () => userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

beforeEach(() => {
  editor.chapters = [{ id: 1 }, { id: 2 }];
  vi.clearAllMocks();
});

describe("FileChaptersSection", () => {
  it("counts the chapters above their window", () => {
    renderSection();

    expect(
      screen.getByRole("heading", { name: "2 chapters" }),
    ).toBeInTheDocument();
    expect(screen.getByText("chapter list")).toBeInTheDocument();
  });

  it("fetches from Audible in one click, straight into edit mode", async () => {
    renderSection();

    await user().click(
      screen.getByRole("button", { name: "Fetch from Audible" }),
    );

    expect(editor.fetchFromAudible).toHaveBeenCalledOnce();
    expect(editor.editing).toBe(true);
    expect(
      screen.getByRole("heading", { name: "Editing chapters" }),
    ).toBeInTheDocument();
  });

  it("puts every edit action in the bar while editing", async () => {
    renderSection();
    const u = user();

    await u.click(screen.getByRole("button", { name: "Edit chapters" }));
    await u.click(screen.getByRole("button", { name: "Add chapter" }));
    await u.click(screen.getByRole("button", { name: "Cancel" }));

    expect(editor.addChapter).toHaveBeenCalledOnce();
    expect(editor.cancel).toHaveBeenCalledOnce();
    expect(
      screen.getByRole("button", { name: "Fetch from Audible" }),
    ).toBeInTheDocument();
    // Save waits for the editor to say there is something to save.
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("offers Fetch from Audible on an audiobook with no chapters yet", () => {
    editor.chapters = [];
    renderSection();

    expect(
      screen.getByRole("heading", { name: "Chapters" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Fetch from Audible" }),
    ).toBeInTheDocument();
  });

  it("offers no editing for an EPUB with no chapters, which cannot gain any", () => {
    editor.chapters = [];
    renderSection({ file: file({ file_type: "epub" }) });

    expect(
      screen.queryByRole("button", { name: "Edit chapters" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Fetch from Audible" }),
    ).not.toBeInTheDocument();
  });

  it("leaves out a supplement's section when it has no chapters", () => {
    editor.chapters = [];
    const { container } = renderSection({
      file: file({ file_role: "supplement", file_type: "pdf" }),
    });

    expect(container).toBeEmptyDOMElement();
  });

  it("shows no actions to a role that cannot edit", () => {
    renderSection({ canEdit: false });

    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
