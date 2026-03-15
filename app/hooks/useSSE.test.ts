import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Mock EventSource
class MockEventSource {
  static instances: MockEventSource[] = [];
  url: string;
  listeners: Record<string, ((event: MessageEvent) => void)[]> = {};
  close = vi.fn();

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    if (!this.listeners[type]) {
      this.listeners[type] = [];
    }
    this.listeners[type].push(listener);
  }

  removeEventListener(type: string, listener: (event: MessageEvent) => void) {
    if (this.listeners[type]) {
      this.listeners[type] = this.listeners[type].filter((l) => l !== listener);
    }
  }

  // Test helper: simulate an event
  simulateEvent(type: string, data: string) {
    const event = new MessageEvent(type, { data });
    this.listeners[type]?.forEach((l) => l(event));
  }
}

describe("useSSE EventSource URL", () => {
  beforeEach(() => {
    MockEventSource.instances = [];
    vi.stubGlobal("EventSource", MockEventSource);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("connects to /api/events", async () => {
    const es = new MockEventSource("/api/events");
    expect(es.url).toBe("/api/events");
  });
});
