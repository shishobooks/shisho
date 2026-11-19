import { act } from "@testing-library/react";
import { vi } from "vitest";

/** Lets query cache updates reach components, which they do on a timer. */
export const flush = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 50));
  });

/**
 * MockEventSource stands in for the browser's EventSource, so a page test
 * can send the server's events. Install it with
 * `vi.stubGlobal("EventSource", MockEventSource)` after clearing
 * `MockEventSource.instances`, and render the page inside `WithSSE`
 * (`@/testing/WithSSE`).
 */
export class MockEventSource {
  static instances: MockEventSource[] = [];
  url: string;
  onerror: (() => void) | null = null;
  onopen: (() => void) | null = null;
  listeners: Record<string, ((event: MessageEvent) => void)[]> = {};
  close = vi.fn();

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    (this.listeners[type] ??= []).push(listener);
  }

  removeEventListener(type: string, listener: (event: MessageEvent) => void) {
    this.listeners[type] = (this.listeners[type] ?? []).filter(
      (l) => l !== listener,
    );
  }

  // Query cache updates reach components on a timer, so this flushes too.
  async emit(type: string, data: object) {
    const event = new MessageEvent(type, { data: JSON.stringify(data) });
    act(() => {
      this.listeners[type]?.forEach((l) => l(event));
    });
    await flush();
  }
}

/** The open stream: the app opens one, at the root. */
export const eventStream = () => MockEventSource.instances[0];
