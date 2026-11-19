import type { AvailablePlugin } from "@/hooks/queries/plugins";
import type { DetectFileTypesResponse } from "@/types";

// The official repository's metadata enrichers, by plugin id.
export const GOODREADS = "goodreads-enricher";
export const OPEN_LIBRARY = "open-library-enricher";
export const AUDIBLE = "audible-enricher";
export const MANGA = "manga-enricher";

// The four main enrichers, in the order Setup lists them.
const MAIN_ENRICHERS = [GOODREADS, OPEN_LIBRARY, AUDIBLE, MANGA];

export interface Recommendation {
  /** Plugin ids ticked to start with. */
  preselected: string[];
  /** Plugin ids shown unticked beside them. */
  offered: string[];
}

/**
 * Which official plugins Setup recommends for what detection found: Goodreads
 * with Open Library offered for ebooks, Audible for audiobooks, Manga for
 * comics. When nothing was found (or detection failed), Goodreads is ticked
 * and all four main enrichers are shown.
 */
export const recommendPlugins = (
  detected: DetectFileTypesResponse | undefined,
): Recommendation => {
  const ebooks = (detected?.ebooks ?? 0) > 0;
  const audiobooks = (detected?.audiobooks ?? 0) > 0;
  const comics = (detected?.comics ?? 0) > 0;

  if (!ebooks && !audiobooks && !comics) {
    return {
      preselected: [GOODREADS],
      offered: [OPEN_LIBRARY, AUDIBLE, MANGA],
    };
  }

  const preselected = new Set<string>();
  const offered = new Set<string>();
  if (ebooks) {
    preselected.add(GOODREADS);
    offered.add(OPEN_LIBRARY);
  }
  if (audiobooks) preselected.add(AUDIBLE);
  if (comics) preselected.add(MANGA);
  return {
    preselected: MAIN_ENRICHERS.filter((id) => preselected.has(id)),
    offered: MAIN_ENRICHERS.filter((id) => offered.has(id)),
  };
};

/** The key Setup tracks a plugin by. */
export const pluginKey = (plugin: { scope: string; id: string }) =>
  `${plugin.scope}/${plugin.id}`;

export interface PluginChoices {
  /** Recommended plugins, preselected first, then those offered. */
  recommended: AvailablePlugin[];
  /** Every other plugin in the official repository. */
  others: AvailablePlugin[];
  /** Keys of the recommended plugins ticked to start with. */
  preselectedKeys: string[];
}

/**
 * Splits the official repository's plugins into Setup's recommendations and
 * the rest. A recommended plugin the repository doesn't list is left out.
 */
export const pluginChoices = (
  available: AvailablePlugin[],
  recommendation: Recommendation,
): PluginChoices => {
  const official = available.filter((p) => p.is_official);
  const byId = new Map(official.map((p) => [p.id, p]));
  const recommendedIds = [
    ...recommendation.preselected,
    ...recommendation.offered,
  ];
  const recommended = recommendedIds
    .map((id) => byId.get(id))
    .filter((p): p is AvailablePlugin => p !== undefined);
  const others = official
    .filter((p) => !recommendedIds.includes(p.id))
    .sort((a, b) => a.name.localeCompare(b.name));
  // One this version of Shisho can't install stays unticked.
  const preselectedKeys = recommendation.preselected
    .map((id) => byId.get(id))
    .filter(
      (p): p is AvailablePlugin =>
        p !== undefined && p.versions.some((v) => v.compatible),
    )
    .map(pluginKey);
  return { recommended, others, preselectedKeys };
};
