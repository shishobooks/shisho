import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";

export enum QueryKey {
  AudnexusChapters = "AudnexusChapters",
}

export interface AudnexusChapter {
  title: string;
  start_offset_ms: number;
  length_ms: number;
}

export interface AudnexusChaptersResponse {
  asin: string;
  is_accurate: boolean;
  runtime_length_ms: number;
  brand_intro_duration_ms: number;
  brand_outro_duration_ms: number;
  chapters: AudnexusChapter[];
}

/**
 * Fetch chapter data for an Audible ASIN. Disabled by default — enable only
 * when the user explicitly triggers a lookup from the fetch dialog.
 */
export const useAudnexusChapters = (
  asin: string | null,
  options: Omit<
    UseQueryOptions<AudnexusChaptersResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<AudnexusChaptersResponse, ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : false,
    retry: false,
    ...options,
    queryKey: [QueryKey.AudnexusChapters, asin],
    queryFn: async ({ signal }) => {
      if (!asin) {
        throw new Error("ASIN is required");
      }
      const response: AudnexusChaptersResponse = await API.request(
        "GET",
        `/audnexus/books/${encodeURIComponent(asin)}/chapters`,
        null,
        null,
        signal,
      );
      return response;
    },
  });
};
