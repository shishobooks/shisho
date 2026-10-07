// The tabs of the Advanced plugin settings dialog, kept out of the component
// file so react-refresh can still hot-reload it. The first entry is the
// default section.
export const ADVANCED_SECTIONS = ["order", "repositories"] as const;

export type AdvancedSection = (typeof ADVANCED_SECTIONS)[number];

export const isAdvancedSection = (value: string): value is AdvancedSection =>
  (ADVANCED_SECTIONS as readonly string[]).includes(value);
