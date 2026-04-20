import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  AuthContext,
  type AuthContextValue,
} from "@/components/contexts/Auth/context";
import { QueryKey as BooksQueryKey } from "@/hooks/queries/books";
import { QueryKey as JobsQueryKey } from "@/hooks/queries/jobs";

import { useSSE } from "./useSSE";

// Mock EventSource
class MockEventSource {
  static instances: MockEventSource[] = [];
  url: string;
  onerror: (() => void) | null = null;
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

  simulateEvent(type: string, data: string) {
    const event = new MessageEvent(type, { data });
    this.listeners[type]?.forEach((l) => l(event));
  }
}

function createWrapper(isAuthenticated: boolean, queryClient: QueryClient) {
  const authValue: AuthContextValue = {
    user: isAuthenticated
      ? {
          id: 1,
          username: "test",
          role_id: 1,
          role_name: "admin",
          permissions: [],
          must_change_password: false,
        }
      : null,
    isLoading: false,
    isAuthenticated,
    needsSetup: false,
    login: vi.fn(),
    logout: vi.fn(),
    hasPermission: () => true,
    hasLibraryAccess: () => true,
    refetch: vi.fn(),
    setAuthUser: vi.fn(),
  };

  return ({ children }: { children: ReactNode }) =>
    createElement(
      AuthContext.Provider,
      { value: authValue },
      createElement(QueryClientProvider, { client: queryClient }, children),
    );
}

describe("useSSE", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    MockEventSource.instances = [];
    vi.stubGlobal("EventSource", MockEventSource);
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    queryClient.clear();
  });

  it("opens EventSource when authenticated", () => {
    renderHook(() => useSSE(), {
      wrapper: createWrapper(true, queryClient),
    });

    expect(MockEventSource.instances).toHaveLength(1);
    expect(MockEventSource.instances[0].url).toBe("/api/events");
  });

  it("does not open EventSource when not authenticated", () => {
    renderHook(() => useSSE(), {
      wrapper: createWrapper(false, queryClient),
    });

    expect(MockEventSource.instances).toHaveLength(0);
  });

  it("closes EventSource on unmount", () => {
    const { unmount } = renderHook(() => useSSE(), {
      wrapper: createWrapper(true, queryClient),
    });

    const es = MockEventSource.instances[0];
    unmount();

    expect(es.close).toHaveBeenCalled();
  });

  it("invalidates job queries on job.created event", () => {
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    renderHook(() => useSSE(), {
      wrapper: createWrapper(true, queryClient),
    });

    act(() => {
      MockEventSource.instances[0].simulateEvent("job.created", '{"job_id":1}');
    });

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: [JobsQueryKey.ListJobs] }),
    );
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: [JobsQueryKey.LatestScanJob] }),
    );
  });

  it("invalidates job and log queries on job.status_changed event", () => {
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    renderHook(() => useSSE(), {
      wrapper: createWrapper(true, queryClient),
    });

    act(() => {
      MockEventSource.instances[0].simulateEvent(
        "job.status_changed",
        '{"job_id":5,"status":"in_progress","type":"scan"}',
      );
    });

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: [JobsQueryKey.RetrieveJob, "5"],
      }),
    );
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: [JobsQueryKey.ListJobLogs, "5"],
      }),
    );
  });

  it("invalidates book queries when scan completes", () => {
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    renderHook(() => useSSE(), {
      wrapper: createWrapper(true, queryClient),
    });

    act(() => {
      MockEventSource.instances[0].simulateEvent(
        "job.status_changed",
        '{"job_id":1,"status":"completed","type":"scan"}',
      );
    });

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: [BooksQueryKey.ListBooks] }),
    );
  });

  it("does not invalidate book queries for non-scan job completion", () => {
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    renderHook(() => useSSE(), {
      wrapper: createWrapper(true, queryClient),
    });

    act(() => {
      MockEventSource.instances[0].simulateEvent(
        "job.status_changed",
        '{"job_id":1,"status":"completed","type":"export"}',
      );
    });

    expect(invalidateSpy).not.toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: [BooksQueryKey.ListBooks] }),
    );
  });

  it("registers event listeners for both event types", () => {
    renderHook(() => useSSE(), {
      wrapper: createWrapper(true, queryClient),
    });

    const es = MockEventSource.instances[0];
    expect(es.listeners["job.created"]).toHaveLength(1);
    expect(es.listeners["job.status_changed"]).toHaveLength(1);
  });
});
