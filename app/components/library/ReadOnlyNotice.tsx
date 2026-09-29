import type { ReactNode } from "react";

interface ReadOnlyNoticeProps {
  children?: ReactNode;
}

/**
 * The note above a form a role can view but not save. A read-only form keeps
 * its inputs, disabled, so the values read the same as for a writer; this
 * note says why nothing can be changed. The Save button is hidden, not
 * disabled. See "Read-only forms" in app/AGENTS.md.
 */
const ReadOnlyNotice = ({ children }: ReadOnlyNoticeProps) => (
  <p className="text-sm text-muted-foreground" role="note">
    {children ?? "You can view these settings but not change them."}
  </p>
);

export default ReadOnlyNotice;
