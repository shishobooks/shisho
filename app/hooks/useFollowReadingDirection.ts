import { useState } from "react";

const STORAGE_KEY = "shisho-reader-follow-reading-direction";

const readStored = (): boolean => {
  try {
    return localStorage.getItem(STORAGE_KEY) !== "false";
  } catch {
    // Blocked storage: follow, as the default does.
    return true;
  }
};

// Whether the page reader follows each File's Reading Direction, as this
// browser remembers it. On until the reader turns it off; off, every File runs
// left to right.
export const useFollowReadingDirection = () => {
  const [follow, setFollowState] = useState(readStored);
  const setFollow = (next: boolean) => {
    setFollowState(next);
    try {
      localStorage.setItem(STORAGE_KEY, String(next));
    } catch {
      // Blocked storage: the choice lasts until the reader closes.
    }
  };
  return [follow, setFollow] as const;
};
