import { CircleArrowUp, ExternalLink } from "lucide-react";

import { updateLabel } from "@/components/layout/useAvailableUpdate";
import type { UpdateCheckResponse } from "@/types/generated/updatecheck";

// Every release's notes are on this page, newest first, so it covers each
// release between the running version and the latest.
export const RELEASES_URL = "https://github.com/shishobooks/shisho/releases";

interface UpdateNoticeProps {
  update: UpdateCheckResponse;
}

/**
 * Takes the place of the version footer while a newer release is out, and
 * links to the release notes.
 */
const UpdateNotice = ({ update }: UpdateNoticeProps) => (
  <a
    className="group mx-auto flex w-fit flex-col gap-1 rounded-md border border-primary/30 bg-primary/5 px-3 py-2 transition-colors hover:border-primary/50 hover:bg-primary/10"
    href={RELEASES_URL}
    rel="noopener noreferrer"
    target="_blank"
  >
    <span className="flex items-center gap-1.5 text-xs font-medium text-primary">
      <CircleArrowUp aria-hidden="true" className="h-3.5 w-3.5 shrink-0" />
      Update available
      <ExternalLink
        aria-hidden="true"
        className="ml-auto h-3 w-3 shrink-0 text-muted-foreground transition-colors group-hover:text-primary"
      />
    </span>
    <span className="font-mono text-[11px] text-foreground/80">
      {update.current_version} → {update.latest_version}
    </span>
    <span className="text-[11px] text-muted-foreground">
      {updateLabel(update)}
    </span>
  </a>
);

export default UpdateNotice;
