import { useUpdateCheck } from "@/hooks/queries/updateCheck";
import type { UpdateCheckResponse } from "@/types/generated/updatecheck";
import { formatCount } from "@/utils/format";

/**
 * The server's update check when it reports a newer release, otherwise
 * undefined. Roles without Config Read never ask, so they always get
 * undefined.
 */
export const useAvailableUpdate = (): UpdateCheckResponse | undefined => {
  const { data } = useUpdateCheck();
  return data?.update_available ? data : undefined;
};

/** The phrase naming how far behind the server is. */
export const updateLabel = (update: UpdateCheckResponse) =>
  `${formatCount(update.releases_behind, "release")} behind`;
