import { API } from "@/libraries/api";

// The most a paged list endpoint (GET /libraries, GET /users) returns at once.
const PAGE_SIZE = 100;

/**
 * Fetches every item of a paged list endpoint that answers `{ items, total }`
 * to `limit` and `offset`, for a page that shows the whole list. A short page
 * is the last, whatever total says, so a row deleted meanwhile can't keep the
 * loop going.
 */
export const fetchEveryPage = async <T>(
  path: string,
  signal?: AbortSignal,
): Promise<{ items: T[]; total: number }> => {
  const items: T[] = [];
  for (;;) {
    const page = await API.request<{ items: T[]; total: number }>(
      "GET",
      path,
      null,
      { limit: PAGE_SIZE, offset: items.length },
      signal,
    );
    items.push(...page.items);
    if (page.items.length < PAGE_SIZE || items.length >= page.total) {
      return { items, total: page.total };
    }
  }
};
