import { afterEach, describe, expect, it, vi } from "vitest";

import { copyText } from "./clipboard";

const setClipboard = (value: unknown) =>
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value,
  });

afterEach(() => {
  setClipboard(undefined);
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

// jsdom has no execCommand, so install one that reports what was selected
// and where the selected element lived.
const stubExecCommand = (result: boolean) => {
  const seen: { text?: string; parent?: Element | null } = {};
  const execCommand = vi.fn(() => {
    const active = document.activeElement as HTMLTextAreaElement | null;
    seen.text = active?.value;
    seen.parent = active?.parentElement;
    return result;
  });
  Object.defineProperty(document, "execCommand", {
    configurable: true,
    value: execCommand,
  });
  return { execCommand, seen };
};

describe("copyText", () => {
  it("uses the async clipboard when it exists", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    setClipboard({ writeText });

    await expect(copyText("hello")).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith("hello");
  });

  it("falls back to execCommand without the async clipboard", async () => {
    setClipboard(undefined);
    const { execCommand, seen } = stubExecCommand(true);

    await expect(copyText("http://lan/share/abc")).resolves.toBe(true);
    expect(execCommand).toHaveBeenCalledWith("copy");
    expect(seen.text).toBe("http://lan/share/abc");
    expect(document.querySelector("textarea")).toBeNull();
  });

  it("selects inside the open dialog so a focus trap cannot pull focus away", async () => {
    setClipboard(undefined);
    const dialog = document.createElement("div");
    dialog.setAttribute("role", "dialog");
    const button = document.createElement("button");
    dialog.appendChild(button);
    document.body.appendChild(dialog);
    button.focus();
    const { seen } = stubExecCommand(true);

    await copyText("text");

    expect(seen.text).toBe("text");
    expect(seen.parent).toBe(dialog);
    expect(document.activeElement).toBe(button);
  });

  it("falls back when the async clipboard rejects", async () => {
    setClipboard({ writeText: vi.fn().mockRejectedValue(new Error("denied")) });
    const { execCommand } = stubExecCommand(true);

    await expect(copyText("x")).resolves.toBe(true);
    expect(execCommand).toHaveBeenCalled();
  });

  it("reports failure when nothing can copy", async () => {
    setClipboard(undefined);
    stubExecCommand(false);

    await expect(copyText("x")).resolves.toBe(false);
  });
});
