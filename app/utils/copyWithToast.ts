import { toast } from "sonner";

import { copyText } from "@/utils/clipboard";

// Copies text and toasts the outcome, for a copy button.
export const copyTextWithToast = async (
  text: string,
  failure = "Could not copy",
): Promise<void> => {
  if (await copyText(text)) {
    toast.success("Copied to clipboard");
  } else {
    toast.error(failure);
  }
};
