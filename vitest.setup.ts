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

afterEach(() => {
  vi.clearAllTimers();
});
