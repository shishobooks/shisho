import { describe, expect, it } from "vitest";

import {
  pageTurnForKey,
  searchShortcutForKey,
  turnInDirection,
} from "@/utils/readerKeys";

const keyOn = (key: string, target: Element) => {
  const event = new KeyboardEvent("keydown", { key, bubbles: true });
  Object.defineProperty(event, "target", { value: target });
  return event;
};

describe("pageTurnForKey", () => {
  it("leaves Shift+Arrow to extend a selection", () => {
    const event = new KeyboardEvent("keydown", {
      key: "ArrowRight",
      shiftKey: true,
    });
    expect(pageTurnForKey(event)).toBeNull();
  });

  it("turns pages with the arrow keys and A and D", () => {
    const body = document.body;
    expect(pageTurnForKey(keyOn("ArrowRight", body))).toBe("next");
    expect(pageTurnForKey(keyOn("d", body))).toBe("next");
    expect(pageTurnForKey(keyOn("ArrowLeft", body))).toBe("prev");
    expect(pageTurnForKey(keyOn("A", body))).toBe("prev");
    expect(pageTurnForKey(keyOn("x", body))).toBeNull();
  });

  it("leaves keys meant for a control alone", () => {
    // The settings popover's font size slider and the chapter menu use the
    // arrow keys themselves; a page turn there would count as reading.
    const popover = document.createElement("div");
    popover.setAttribute("role", "dialog");
    const slider = document.createElement("span");
    slider.setAttribute("role", "slider");
    popover.append(slider);
    const select = document.createElement("select");
    const input = document.createElement("input");
    // The chapter menu's closed trigger and its open list, which take typed
    // letters to jump to an entry.
    const trigger = document.createElement("button");
    trigger.setAttribute("role", "combobox");
    const list = document.createElement("div");
    list.setAttribute("role", "listbox");
    const option = document.createElement("div");
    list.append(option);

    expect(pageTurnForKey(keyOn("ArrowLeft", slider))).toBeNull();
    expect(pageTurnForKey(keyOn("ArrowRight", select))).toBeNull();
    expect(pageTurnForKey(keyOn("d", input))).toBeNull();
    expect(pageTurnForKey(keyOn("ArrowRight", trigger))).toBeNull();
    expect(pageTurnForKey(keyOn("d", option))).toBeNull();
  });

  it("still turns pages from the reading progress bar", () => {
    const bar = document.createElement("div");
    bar.setAttribute("role", "slider");
    expect(pageTurnForKey(keyOn("ArrowRight", bar))).toBe("next");
  });

  it("leaves browser shortcuts alone", () => {
    const shortcut = (init: KeyboardEventInit) =>
      pageTurnForKey(new KeyboardEvent("keydown", { bubbles: true, ...init }));
    expect(shortcut({ key: "d", metaKey: true })).toBeNull();
    expect(shortcut({ key: "a", ctrlKey: true })).toBeNull();
    expect(shortcut({ key: "ArrowLeft", altKey: true })).toBeNull();
    expect(shortcut({ key: "D", shiftKey: true })).toBe("next");
  });
});

describe("turnInDirection", () => {
  it("keeps a turn left to right and reverses it right to left", () => {
    expect(turnInDirection("next", false)).toBe("next");
    expect(turnInDirection("next", true)).toBe("prev");
    expect(turnInDirection("prev", true)).toBe("next");
    expect(turnInDirection(null, true)).toBeNull();
  });
});

describe("searchShortcutForKey", () => {
  const shortcut = (init: KeyboardEventInit) =>
    searchShortcutForKey(new KeyboardEvent("keydown", init));

  it("finds with Cmd+F or Ctrl+F", () => {
    expect(shortcut({ key: "f", metaKey: true })).toBe("find");
    expect(shortcut({ key: "f", ctrlKey: true })).toBe("find");
    expect(shortcut({ key: "F", metaKey: true, shiftKey: true })).toBeNull();
  });

  it("steps with Cmd+G, Ctrl+G, and F3, and back with Shift", () => {
    expect(shortcut({ key: "g", metaKey: true })).toBe("next");
    expect(shortcut({ key: "g", ctrlKey: true })).toBe("next");
    expect(shortcut({ key: "G", metaKey: true, shiftKey: true })).toBe(
      "previous",
    );
    expect(shortcut({ key: "F3" })).toBe("next");
    expect(shortcut({ key: "F3", shiftKey: true })).toBe("previous");
  });

  it("leaves other keys alone", () => {
    expect(shortcut({ key: "f" })).toBeNull();
    expect(shortcut({ key: "g" })).toBeNull();
    expect(shortcut({ key: "f", metaKey: true, altKey: true })).toBeNull();
    expect(shortcut({ key: "F3", metaKey: true })).toBeNull();
  });
});
