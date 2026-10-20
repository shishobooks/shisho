import type { DeliveredFileType, File, Rule } from "@/types";

/**
 * The File Type a file's main download button delivers under the user's
 * default Delivery Rules: the delivered File Type of the rule for the file's
 * own type, when the file can become it, and otherwise the file's own type.
 * This is the web UI's own policy, separate from the server's resolver:
 * each file row has its own button, so no choice between a book's Files
 * applies, and a File Type no rule lists still downloads as itself. Without
 * rules (a Share Link, or rules that failed to load) it is the file's own
 * type.
 */
export const defaultDeliveredFileType = (
  file: Pick<File, "file_type" | "reachable_file_types">,
  rules: Rule[] | undefined,
): DeliveredFileType => {
  const rule = rules?.find((r) => r.source_file_type === file.file_type);
  if (rule && file.reachable_file_types.includes(rule.delivered_file_type)) {
    return rule.delivered_file_type;
  }
  return file.file_type;
};

/**
 * A Delivery Rule being edited, with a client-side key that survives
 * reordering so dnd-kit can track the row.
 */
export interface DraftRule extends Rule {
  key: string;
}

let draftKeyCounter = 0;
export const withKeys = (rules: Rule[]): DraftRule[] =>
  rules.map((rule) => ({ ...rule, key: `rule-${++draftKeyCounter}` }));
export const withoutKeys = (rules: DraftRule[]): Rule[] =>
  rules.map(({ source_file_type, delivered_file_type }) => ({
    source_file_type,
    delivered_file_type,
  }));
