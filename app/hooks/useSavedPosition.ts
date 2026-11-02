import { useEffect, useState } from "react";

import { useFilePosition } from "@/hooks/queries/reading";
import { toastRequestError } from "@/libraries/api";

// The Reading Position a reader opens at, fetched anew so a place saved in
// another browser since wins over a cached one. A failed load opens at the
// start, and the reader passes failed on to useReadingProgress to save
// nothing, so a move there can't replace the place saved on the server. How
// the first load went decides for as long as the reader stays open: the reader
// opened by then, so a refetch succeeding or failing later changes neither
// where it is nor whether it may save.
export const useSavedPosition = (fileId: number) => {
  const query = useFilePosition(fileId, {
    refetchOnMount: "always",
    retry: 1,
  });
  const loadFailed = Boolean(query.error) && !query.isFetching;
  const [settled, setSettled] = useState(false);
  const [failed, setFailed] = useState(false);
  if (
    !settled &&
    ((query.isFetchedAfterMount && !query.isFetching) ||
      loadFailed ||
      !query.isEnabled)
  ) {
    setSettled(true);
    setFailed(loadFailed);
  }
  useEffect(() => {
    if (failed) {
      toastRequestError(query.error, "Failed to load your reading position");
    }
  }, [failed, query.error]);

  return { saved: query.data?.position ?? null, settled, failed };
};
