import { afterEach, describe, expect, it, vi } from "vitest";

import {
  loadBookSpeed,
  MAX_SPEED,
  MIN_SPEED,
  saveBookSpeed,
  stepSpeed,
} from "./playbackSpeed";

afterEach(() => {
  localStorage.clear();
  vi.restoreAllMocks();
});

describe("stepSpeed", () => {
  it("steps by 0.1 without floating point drift", () => {
    let speed = 1;
    for (let i = 0; i < 4; i++) speed = stepSpeed(speed, 1);
    expect(speed).toBe(1.4);
    expect(stepSpeed(1.4, -1)).toBe(1.3);
    expect(stepSpeed(0.7, 1)).toBe(0.8);
  });

  it("moves an off-step preset to the next step in that direction", () => {
    expect(stepSpeed(1.25, 1)).toBe(1.3);
    expect(stepSpeed(1.25, -1)).toBe(1.2);
    expect(stepSpeed(0.75, -1)).toBe(0.7);
  });

  it("stops at the limits", () => {
    expect(MIN_SPEED).toBe(0.5);
    expect(MAX_SPEED).toBe(4);
    expect(stepSpeed(0.5, -1)).toBe(0.5);
    expect(stepSpeed(0.55, -1)).toBe(0.5);
    expect(stepSpeed(4, 1)).toBe(4);
    expect(stepSpeed(3.9, 1)).toBe(4);
  });
});

describe("per-Book speed", () => {
  it("starts a Book with no saved speed at 1x", () => {
    expect(loadBookSpeed(7)).toBe(1);
  });

  it("remembers each Book's speed on its own", () => {
    saveBookSpeed(7, 1.4);
    saveBookSpeed(8, 2);
    expect(loadBookSpeed(7)).toBe(1.4);
    expect(loadBookSpeed(8)).toBe(2);
    expect(loadBookSpeed(9)).toBe(1);
  });

  it("forgets a Book set back to 1x", () => {
    saveBookSpeed(7, 1.4);
    saveBookSpeed(7, 1);
    expect(loadBookSpeed(7)).toBe(1);
    expect(localStorage.length).toBe(0);
  });

  it.each(["fast", "0", "-2", "9", "NaN", ""])(
    "ignores a stored value of %j",
    (stored) => {
      localStorage.setItem("shisho-playback-speed-7", stored);
      expect(loadBookSpeed(7)).toBe(1);
    },
  );

  it("keeps working when storage is unavailable", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    expect(() => saveBookSpeed(7, 1.4)).not.toThrow();
    expect(loadBookSpeed(7)).toBe(1);
  });
});
