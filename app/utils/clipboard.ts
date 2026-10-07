// Copies text to the clipboard and reports whether it worked.
// navigator.clipboard exists only in secure contexts (HTTPS or localhost), and
// many self-hosted servers are reached over plain HTTP on a LAN, so fall back
// to a hidden textarea and execCommand there.
// On false, toast an error rather than a success.
export const copyText = async (text: string): Promise<boolean> => {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fall through to the legacy path, e.g. when permission is denied.
    }
  }

  // Put the textarea inside an open dialog, if any: a modal's focus trap
  // pulls focus back from anything outside it before the copy runs.
  const previous =
    document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;
  const container = previous?.closest('[role="dialog"]') ?? document.body;
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  container.appendChild(textarea);
  textarea.focus();
  textarea.select();
  try {
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    textarea.remove();
    previous?.focus();
  }
};
