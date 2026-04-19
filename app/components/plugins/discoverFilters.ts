import type { AvailablePlugin } from "@/hooks/queries/plugins";

/** Pure filter function for the Discover tab. */
export const filterPlugins = (
  plugins: AvailablePlugin[],
  search: string,
  capability: string,
  source: string,
): AvailablePlugin[] => {
  return plugins.filter((p) => {
    if (search) {
      const q = search.toLowerCase();
      const nameMatch = p.name.toLowerCase().includes(q);
      const descMatch = (p.description ?? "").toLowerCase().includes(q);
      if (!nameMatch && !descMatch) return false;
    }
    if (capability !== "all") {
      const caps = p.versions[0]?.capabilities;
      if (!caps) return false;
      const capKey = capability as keyof typeof caps;
      if (!caps[capKey]) return false;
    }
    if (source !== "all" && p.scope !== source) return false;
    return true;
  });
};
