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

// Nothing a test leaves on the clock reaches the next one
// (app/testing/fakeTimers.test.tsx). React's scheduler queues renders on
// these timers and never queues again once its pending one is cleared, so
// whatever is due runs first: the test's components are unmounted by then.
// The scheduler also keeps the timer functions it found when it loaded, so a
// test must not swap the clock with vi.useFakeTimers or vi.useRealTimers.
afterEach(() => {
  vi.advanceTimersByTime(0);
  vi.clearAllTimers();
});
