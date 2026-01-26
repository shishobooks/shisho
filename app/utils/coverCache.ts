// Simple in-memory cache to track which cover images have loaded successfully
// This prevents showing placeholders for covers that are already in browser cache

const loadedCovers = new Set<string>();

export const markCoverLoaded = (url: string) => {
  loadedCovers.add(url);
};

export const isCoverLoaded = (url: string) => {
  return loadedCovers.has(url);
};
