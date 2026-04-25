import { afterEach, vi } from "vitest";

import "@testing-library/dom";
import "@testing-library/jest-dom/vitest";

// Use fake timers so pending callbacks (e.g. Radix UI's pointer-events
// cleanup setTimeout) can be cleared between tests before jsdom tears down.
// shouldAdvanceTime lets timers fire naturally based on real clock time so
// tests don't need to manually advance.
vi.useFakeTimers({ shouldAdvanceTime: true });

// Mock ResizeObserver for Radix UI components
global.ResizeObserver = class ResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
};

// Polyfill APIs missing from jsdom — required by Radix UI Select which calls
// these in its pointer event handlers and scroll-into-view logic.
if (!HTMLElement.prototype.hasPointerCapture) {
  HTMLElement.prototype.hasPointerCapture = () => false;
}
if (!HTMLElement.prototype.setPointerCapture) {
  HTMLElement.prototype.setPointerCapture = () => {};
}
if (!HTMLElement.prototype.releasePointerCapture) {
  HTMLElement.prototype.releasePointerCapture = () => {};
}
if (!HTMLElement.prototype.scrollIntoView) {
  HTMLElement.prototype.scrollIntoView = () => {};
}

afterEach(() => {
  vi.clearAllTimers();
});
